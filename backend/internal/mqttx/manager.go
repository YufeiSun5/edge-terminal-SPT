package mqttx

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/protocol/kio"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type Manager struct {
	mu             sync.RWMutex
	channels       *pipeline.Channels
	gateways       map[int]*Gateway
	pendingKIOAcks map[int64]chan kio.WriteAck
	ackMu          sync.Mutex
}

type Gateway struct {
	config           models.GatewayConfig
	client           MQTT.Client
	active           bool
	connectedOnce    bool
	reconnects       int
	lastConnected    time.Time
	lastDisconnected time.Time
	lastFullSync     time.Time
	lastError        string
	subscribedTopics map[string]byte
	ackHandler       func([]byte)
	mu               sync.RWMutex
}

type GatewayStatus struct {
	Active           bool      `json:"active"`
	ClientID         string    `json:"client_id"`
	Broker           string    `json:"broker"`
	MainTopic        string    `json:"main_topic"`
	WriteResultTopic string    `json:"write_result_topic,omitempty"`
	QueryAllTopic    string    `json:"query_all_topic,omitempty"`
	SubscribedTopics []string  `json:"subscribed_topics"`
	Reconnects       int       `json:"reconnects"`
	LastConnected    time.Time `json:"last_connected,omitempty"`
	LastDisconnected time.Time `json:"last_disconnected,omitempty"`
	LastFullSync     time.Time `json:"last_full_sync,omitempty"`
	LastError        string    `json:"last_error,omitempty"`
}

func NewManager(channels *pipeline.Channels) *Manager {
	return &Manager{
		channels:       channels,
		gateways:       make(map[int]*Gateway),
		pendingKIOAcks: make(map[int64]chan kio.WriteAck),
	}
}

func (m *Manager) StartAll(configs []models.GatewayConfig) {
	for _, cfg := range configs {
		if err := m.Start(cfg); err != nil {
			log.Printf("start mqtt gateway failed id=%d err=%v", cfg.ID, err)
		}
	}
}

func (m *Manager) Start(cfg models.GatewayConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.gateways[cfg.ID]; exists {
		return fmt.Errorf("gateway already exists: %d", cfg.ID)
	}

	gw := &Gateway{
		config:           cfg,
		ackHandler:       m.handleKIOAck,
		subscribedTopics: make(map[string]byte),
	}
	m.gateways[cfg.ID] = gw
	go gw.run(m.channels)
	return nil
}

func (m *Manager) ApplyConfig(cfg models.GatewayConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if gw, exists := m.gateways[cfg.ID]; exists {
		gw.stop()
		delete(m.gateways, cfg.ID)
	}
	if !cfg.Enabled {
		return nil
	}

	gw := &Gateway{
		config:           cfg,
		ackHandler:       m.handleKIOAck,
		subscribedTopics: make(map[string]byte),
	}
	m.gateways[cfg.ID] = gw
	go gw.run(m.channels)
	return nil
}

func (m *Manager) Stop(gatewayID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if gw, exists := m.gateways[gatewayID]; exists {
		gw.stop()
		delete(m.gateways, gatewayID)
	}
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, gw := range m.gateways {
		gw.stop()
		delete(m.gateways, id)
	}
}

func (m *Manager) Status() map[int]GatewayStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[int]GatewayStatus, len(m.gateways))
	for id, gw := range m.gateways {
		status[id] = gw.status()
	}
	return status
}

func (m *Manager) Config(gatewayID int) (models.GatewayConfig, bool) {
	m.mu.RLock()
	gw, ok := m.gateways[gatewayID]
	m.mu.RUnlock()
	if !ok {
		return models.GatewayConfig{}, false
	}

	gw.mu.RLock()
	defer gw.mu.RUnlock()
	return gw.config, true
}

func (m *Manager) Publish(ctx context.Context, gatewayID int, topic string, payload []byte, qos byte, retain bool) error {
	m.mu.RLock()
	gw, ok := m.gateways[gatewayID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("gateway not found: %d", gatewayID)
	}
	return gw.publish(ctx, topic, payload, qos, retain)
}

func (m *Manager) Subscribe(ctx context.Context, gatewayID int, topic string, qos byte) error {
	m.mu.RLock()
	gw, ok := m.gateways[gatewayID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("gateway not found: %d", gatewayID)
	}
	return gw.subscribe(ctx, topic, qos, m.channels)
}

func (m *Manager) PublishAndWaitKIOAck(ctx context.Context, gatewayID int, topic string, payload []byte, qos byte, retain bool, qid int64) (*kio.WriteAck, bool, error) {
	if qid == 0 {
		return nil, false, fmt.Errorf("qid is required")
	}
	ackCh, cancel := m.registerKIOAck(qid)
	defer cancel()

	if err := m.Publish(ctx, gatewayID, topic, payload, qos, retain); err != nil {
		return nil, false, err
	}

	select {
	case ack := <-ackCh:
		return &ack, true, nil
	case <-ctx.Done():
		return nil, true, ctx.Err()
	}
}

func (m *Manager) registerKIOAck(qid int64) (<-chan kio.WriteAck, func()) {
	ch := make(chan kio.WriteAck, 1)
	m.ackMu.Lock()
	m.pendingKIOAcks[qid] = ch
	m.ackMu.Unlock()
	cancel := func() {
		m.ackMu.Lock()
		delete(m.pendingKIOAcks, qid)
		m.ackMu.Unlock()
	}
	return ch, cancel
}

func (m *Manager) handleKIOAck(payload []byte) {
	ack, ok, err := kio.ParseWriteAck(payload)
	if err != nil || !ok || ack.QID == 0 {
		return
	}

	m.ackMu.Lock()
	ch := m.pendingKIOAcks[ack.QID]
	if ch != nil {
		delete(m.pendingKIOAcks, ack.QID)
	}
	m.ackMu.Unlock()

	if ch != nil {
		select {
		case ch <- ack:
		default:
		}
	}
}

func (gw *Gateway) run(channels *pipeline.Channels) {
	opts := MQTT.NewClientOptions()
	opts.AddBroker(gw.config.Broker)
	opts.SetClientID(fmt.Sprintf("%s_%d_%d", gw.config.ClientID, time.Now().UnixNano(), rand.Intn(10000)))
	opts.SetUsername(gw.config.Username)
	opts.SetPassword(gw.config.Password)
	opts.SetCleanSession(false)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(20 * time.Second)
	opts.SetConnectTimeout(30 * time.Second)
	opts.SetMaxReconnectInterval(30 * time.Second)

	opts.OnConnect = func(client MQTT.Client) {
		gw.markConnected()

		if err := gw.subscribeConfigured(client, channels); err != nil {
			gw.markError(err)
			log.Printf("[mqtt-%d] subscribe failed: %v", gw.config.ID, err)
			return
		}
		log.Printf("[mqtt-%d] connected and subscribed: %v", gw.config.ID, gw.subscribedTopicNames())
		go gw.publishQueryAll(client)
	}

	opts.OnConnectionLost = func(_ MQTT.Client, err error) {
		gw.markDisconnected(err)
		log.Printf("[mqtt-%d] connection lost: %v", gw.config.ID, err)
	}

	gw.client = MQTT.NewClient(opts)
	if token := gw.client.Connect(); token.Wait() && token.Error() != nil {
		gw.markError(token.Error())
		log.Printf("[mqtt-%d] connect failed: %v", gw.config.ID, token.Error())
		return
	}
}

func (gw *Gateway) stop() {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	if gw.client != nil && gw.client.IsConnected() {
		gw.client.Disconnect(250)
	}
	gw.active = false
	gw.lastDisconnected = time.Now()
}

func (gw *Gateway) publish(ctx context.Context, topic string, payload []byte, qos byte, retain bool) error {
	gw.mu.RLock()
	client := gw.client
	active := gw.active
	gw.mu.RUnlock()

	if !active || client == nil || !client.IsConnected() {
		return fmt.Errorf("gateway is not connected")
	}
	if topic == "" {
		return fmt.Errorf("topic is required")
	}

	token := client.Publish(topic, qos, retain, payload)
	if deadline, ok := ctx.Deadline(); ok {
		if !token.WaitTimeout(time.Until(deadline)) {
			return fmt.Errorf("publish timeout")
		}
	} else {
		done := make(chan struct{})
		go func() {
			token.Wait()
			close(done)
		}()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
		}
	}
	if token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (gw *Gateway) subscribe(ctx context.Context, topic string, qos byte, channels *pipeline.Channels) error {
	gw.mu.RLock()
	client := gw.client
	active := gw.active
	gw.mu.RUnlock()

	if !active || client == nil || !client.IsConnected() {
		return fmt.Errorf("gateway is not connected")
	}
	if topic == "" {
		return fmt.Errorf("topic is required")
	}

	token := client.Subscribe(topic, qos, gw.messageHandler(channels))
	if deadline, ok := ctx.Deadline(); ok {
		if !token.WaitTimeout(time.Until(deadline)) {
			return fmt.Errorf("subscribe timeout")
		}
	} else {
		done := make(chan struct{})
		go func() {
			token.Wait()
			close(done)
		}()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
		}
	}
	if token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (gw *Gateway) messageHandler(channels *pipeline.Channels) MQTT.MessageHandler {
	return func(_ MQTT.Client, msg MQTT.Message) {
		mqttMsg := &models.MQTTMessage{
			GatewayID: gw.config.ID,
			Topic:     msg.Topic(),
			Payload:   append([]byte(nil), msg.Payload()...),
			Timestamp: time.Now(),
		}
		if gw.ackHandler != nil {
			gw.ackHandler(mqttMsg.Payload)
		}
		if msg.Topic() != gw.config.Topic {
			return
		}
		select {
		case channels.Logic <- mqttMsg:
		default:
			log.Printf("[mqtt-%d] logic queue full, drop topic=%s", gw.config.ID, msg.Topic())
		}
		select {
		case channels.Discovery <- mqttMsg:
		default:
		}
	}
}

func (gw *Gateway) subscribeConfigured(client MQTT.Client, channels *pipeline.Channels) error {
	topics := []string{gw.config.Topic}
	if gw.config.WriteResultTopic != "" && gw.config.WriteResultTopic != gw.config.Topic {
		topics = append(topics, gw.config.WriteResultTopic)
	}

	for _, topic := range topics {
		if topic == "" {
			continue
		}
		token := client.Subscribe(topic, gw.config.QOS, gw.messageHandler(channels))
		if !token.WaitTimeout(10 * time.Second) {
			return fmt.Errorf("subscribe timeout: %s", topic)
		}
		if token.Error() != nil {
			return fmt.Errorf("subscribe %s: %w", topic, token.Error())
		}
		gw.recordSubscription(topic, gw.config.QOS)
	}
	return nil
}

func (gw *Gateway) publishQueryAll(client MQTT.Client) {
	topic := gw.config.QueryAllTopic
	if topic == "" && gw.config.KIOClientID != "" {
		topic = kio.QueryAllTagsTopic(gw.config.KIOClientID)
	}
	if topic == "" {
		return
	}

	token := client.Publish(topic, gw.config.QOS, false, []byte{})
	if !token.WaitTimeout(10 * time.Second) {
		err := fmt.Errorf("query-all publish timeout: %s", topic)
		gw.markError(err)
		log.Printf("[mqtt-%d] %v", gw.config.ID, err)
		return
	}
	if token.Error() != nil {
		gw.markError(token.Error())
		log.Printf("[mqtt-%d] query-all publish failed topic=%s err=%v", gw.config.ID, topic, token.Error())
		return
	}

	gw.mu.Lock()
	gw.lastFullSync = time.Now()
	gw.lastError = ""
	gw.mu.Unlock()
	log.Printf("[mqtt-%d] query-all published: %s", gw.config.ID, topic)
}

func (gw *Gateway) markConnected() {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	if gw.connectedOnce {
		gw.reconnects++
	}
	gw.connectedOnce = true
	gw.active = true
	gw.lastConnected = time.Now()
	gw.lastError = ""
}

func (gw *Gateway) markDisconnected(err error) {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	gw.active = false
	gw.lastDisconnected = time.Now()
	if err != nil {
		gw.lastError = err.Error()
	}
}

func (gw *Gateway) markError(err error) {
	if err == nil {
		return
	}
	gw.mu.Lock()
	defer gw.mu.Unlock()
	gw.lastError = err.Error()
}

func (gw *Gateway) recordSubscription(topic string, qos byte) {
	gw.mu.Lock()
	defer gw.mu.Unlock()
	if gw.subscribedTopics == nil {
		gw.subscribedTopics = make(map[string]byte)
	}
	gw.subscribedTopics[topic] = qos
}

func (gw *Gateway) subscribedTopicNames() []string {
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	return subscribedTopicNames(gw.subscribedTopics)
}

func (gw *Gateway) status() GatewayStatus {
	gw.mu.RLock()
	defer gw.mu.RUnlock()

	return GatewayStatus{
		Active:           gw.active,
		ClientID:         gw.config.ClientID,
		Broker:           gw.config.Broker,
		MainTopic:        gw.config.Topic,
		WriteResultTopic: gw.config.WriteResultTopic,
		QueryAllTopic:    gw.config.QueryAllTopic,
		SubscribedTopics: subscribedTopicNames(gw.subscribedTopics),
		Reconnects:       gw.reconnects,
		LastConnected:    gw.lastConnected,
		LastDisconnected: gw.lastDisconnected,
		LastFullSync:     gw.lastFullSync,
		LastError:        gw.lastError,
	}
}

func subscribedTopicNames(topics map[string]byte) []string {
	names := make([]string, 0, len(topics))
	for topic := range topics {
		names = append(names, topic)
	}
	sort.Strings(names)
	return names
}
