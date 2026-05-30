package mqttx

import (
	"context"
	"errors"
	"testing"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/pipeline"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

func TestManagerStatusConfigAndStop(t *testing.T) {
	manager := NewManager(pipeline.NewChannels())
	manager.gateways[1] = &Gateway{
		config: models.GatewayConfig{
			ID:               1,
			ClientID:         "client",
			Broker:           "tcp://127.0.0.1:1883",
			Topic:            "topic",
			WriteResultTopic: "ack",
			QueryAllTopic:    "query-all",
		},
		active:           true,
		subscribedTopics: map[string]byte{"b": 1, "a": 1},
	}
	if cfg, ok := manager.Config(1); !ok || cfg.ClientID != "client" {
		t.Fatalf("unexpected config: %+v ok=%v", cfg, ok)
	}
	status := manager.Status()[1]
	if !status.Active || len(status.SubscribedTopics) != 2 || status.SubscribedTopics[0] != "a" {
		t.Fatalf("unexpected status: %+v", status)
	}
	manager.Stop(1)
	if len(manager.gateways) != 0 {
		t.Fatal("expected gateway stopped")
	}
}

func TestManagerApplyConfigDisabledAndStopAll(t *testing.T) {
	manager := NewManager(pipeline.NewChannels())
	manager.gateways[1] = &Gateway{config: models.GatewayConfig{ID: 1}, subscribedTopics: map[string]byte{}}
	if err := manager.ApplyConfig(models.GatewayConfig{ID: 1, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if len(manager.gateways) != 0 {
		t.Fatal("expected disabled config to remove gateway")
	}
	manager.gateways[2] = &Gateway{config: models.GatewayConfig{ID: 2}, active: true, subscribedTopics: map[string]byte{}}
	manager.gateways[3] = &Gateway{config: models.GatewayConfig{ID: 3}, active: true, subscribedTopics: map[string]byte{}}
	manager.StopAll()
	if len(manager.gateways) != 0 {
		t.Fatal("expected all gateways stopped")
	}
}

func TestManagerAckRegistrationAndErrors(t *testing.T) {
	manager := NewManager(pipeline.NewChannels())
	ch, cancel := manager.registerKIOAck(123)
	manager.handleKIOAck([]byte(`{"Qid":123,"ProcessStep":100,"Result":"ok"}`))
	select {
	case ack := <-ch:
		if !ack.Success {
			t.Fatalf("unexpected ack: %+v", ack)
		}
	case <-time.After(time.Second):
		t.Fatal("ack not delivered")
	}
	cancel()
	manager.handleKIOAck([]byte(`bad`))

	ctx, cancelCtx := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelCtx()
	if err := manager.Publish(ctx, 99, "topic", []byte("{}"), 1, false); err == nil {
		t.Fatal("expected missing gateway publish error")
	}
	if err := manager.Subscribe(ctx, 99, "topic", 1); err == nil {
		t.Fatal("expected missing gateway subscribe error")
	}
	if _, _, err := manager.PublishAndWaitKIOAck(ctx, 99, "topic", []byte(`{"Qid":1}`), 1, false, 0); err == nil {
		t.Fatal("expected missing qid error")
	}
}

func TestManagerPublishAndWaitKIOAckSuccessAndTimeout(t *testing.T) {
	manager := NewManager(pipeline.NewChannels())
	manager.gateways[1] = &Gateway{
		config:           models.GatewayConfig{ID: 1, Topic: "main"},
		client:           &fakeClient{connected: true, token: fakeToken{wait: true}},
		active:           true,
		subscribedTopics: map[string]byte{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		time.Sleep(10 * time.Millisecond)
		manager.handleKIOAck([]byte(`{"Qid":5,"ProcessStep":100,"Result":"ok"}`))
	}()
	ack, brokerAccepted, err := manager.PublishAndWaitKIOAck(ctx, 1, "topic", []byte(`{"Qid":5}`), 1, false, 5)
	if err != nil || !brokerAccepted || ack == nil || !ack.Success {
		t.Fatalf("unexpected ack result ack=%+v broker=%v err=%v", ack, brokerAccepted, err)
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer timeoutCancel()
	if _, brokerAccepted, err := manager.PublishAndWaitKIOAck(timeoutCtx, 1, "topic", []byte(`{"Qid":6}`), 1, false, 6); err == nil || !brokerAccepted {
		t.Fatalf("expected timeout with broker accepted, broker=%v err=%v", brokerAccepted, err)
	}
}

func TestGatewayStateHelpers(t *testing.T) {
	gateway := &Gateway{config: models.GatewayConfig{ClientID: "client", Topic: "topic"}, subscribedTopics: map[string]byte{}}
	gateway.markConnected()
	gateway.markConnected()
	gateway.recordSubscription("b", 1)
	gateway.recordSubscription("a", 1)
	gateway.markError(errors.New("boom"))
	status := gateway.status()
	if !status.Active || status.Reconnects != 1 || status.LastError != "boom" || status.SubscribedTopics[0] != "a" {
		t.Fatalf("unexpected status: %+v", status)
	}
	gateway.markDisconnected(errors.New("lost"))
	if gateway.status().Active || gateway.status().LastError != "lost" {
		t.Fatalf("unexpected disconnected status: %+v", gateway.status())
	}
	gateway.markError(nil)
	if names := subscribedTopicNames(map[string]byte{"z": 1, "a": 1}); names[0] != "a" || names[1] != "z" {
		t.Fatalf("unexpected subscribed names: %+v", names)
	}
}

func TestGatewayMessageHandlerAndPublishSubscribeValidation(t *testing.T) {
	channels := pipeline.NewChannels()
	ackCount := 0
	gateway := &Gateway{
		config: models.GatewayConfig{ID: 1, Topic: "main"},
		ackHandler: func(_ []byte) {
			ackCount++
		},
		subscribedTopics: map[string]byte{},
	}
	handler := gateway.messageHandler(channels)
	handler(nil, fakeMessage{topic: "other", payload: []byte(`{"ack":true}`)})
	if ackCount != 1 || len(channels.Logic) != 0 || len(channels.Discovery) != 0 {
		t.Fatalf("unexpected non-main handling ack=%d logic=%d discovery=%d", ackCount, len(channels.Logic), len(channels.Discovery))
	}
	handler(nil, fakeMessage{topic: "main", payload: []byte(`{"value":1}`)})
	if ackCount != 2 || len(channels.Logic) != 1 || len(channels.Discovery) != 1 {
		t.Fatalf("unexpected main handling ack=%d logic=%d discovery=%d", ackCount, len(channels.Logic), len(channels.Discovery))
	}
	if err := gateway.publish(context.Background(), "", []byte("{}"), 1, false); err == nil {
		t.Fatal("expected inactive publish error")
	}
	if err := gateway.subscribe(context.Background(), "", 1, channels); err == nil {
		t.Fatal("expected inactive subscribe error")
	}
	gateway.publishQueryAll(nil)
}

func TestGatewayPublishSubscribeConfiguredAndQueryAllWithFakeClient(t *testing.T) {
	channels := pipeline.NewChannels()
	client := &fakeClient{connected: true, token: fakeToken{wait: true}}
	gateway := &Gateway{
		config: models.GatewayConfig{
			ID:               1,
			Topic:            "main",
			QOS:              2,
			QueryAllTopic:    "query-all",
			WriteResultTopic: "ack",
		},
		client:           client,
		active:           true,
		ackHandler:       func(_ []byte) {},
		subscribedTopics: map[string]byte{},
	}
	if err := gateway.publish(context.Background(), "topic", []byte("{}"), 1, false); err != nil {
		t.Fatal(err)
	}
	if client.publishedTopic != "topic" {
		t.Fatalf("unexpected published topic: %s", client.publishedTopic)
	}
	if err := gateway.subscribe(context.Background(), "extra", 1, channels); err != nil {
		t.Fatal(err)
	}
	if err := gateway.publish(context.Background(), "", []byte("{}"), 1, false); err == nil {
		t.Fatal("expected empty topic publish error")
	}
	if err := gateway.subscribe(context.Background(), "", 1, channels); err == nil {
		t.Fatal("expected empty topic subscribe error")
	}
	if err := gateway.subscribeConfigured(client, channels); err != nil {
		t.Fatal(err)
	}
	if len(gateway.subscribedTopics) != 2 {
		t.Fatalf("unexpected subscriptions: %+v", gateway.subscribedTopics)
	}
	gateway.publishQueryAll(client)
	if gateway.status().LastFullSync.IsZero() || gateway.status().LastError != "" {
		t.Fatalf("unexpected query-all status: %+v", gateway.status())
	}

	client.token = fakeToken{wait: false}
	timeoutCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := gateway.publish(timeoutCtx, "topic", []byte("{}"), 1, false); err == nil {
		t.Fatal("expected publish timeout")
	}
	if err := gateway.subscribe(timeoutCtx, "topic", 1, channels); err == nil {
		t.Fatal("expected subscribe timeout")
	}
	client.token = fakeToken{wait: true, err: errors.New("mqtt error")}
	if err := gateway.publish(context.Background(), "topic", []byte("{}"), 1, false); err == nil {
		t.Fatal("expected publish token error")
	}
	if err := gateway.subscribeConfigured(client, channels); err == nil {
		t.Fatal("expected subscribe configured token error")
	}
	gateway.publishQueryAll(client)
	if gateway.status().LastError == "" {
		t.Fatal("expected query-all error")
	}
}

type fakeMessage struct {
	topic   string
	payload []byte
}

func (m fakeMessage) Duplicate() bool {
	return false
}

func (m fakeMessage) Qos() byte {
	return 0
}

func (m fakeMessage) Retained() bool {
	return false
}

func (m fakeMessage) Topic() string {
	return m.topic
}

func (m fakeMessage) MessageID() uint16 {
	return 0
}

func (m fakeMessage) Payload() []byte {
	return m.payload
}

func (m fakeMessage) Ack() {}

type fakeToken struct {
	wait bool
	err  error
}

func (t fakeToken) Wait() bool {
	return t.wait
}

func (t fakeToken) WaitTimeout(time.Duration) bool {
	return t.wait
}

func (t fakeToken) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (t fakeToken) Error() error {
	return t.err
}

type fakeClient struct {
	connected      bool
	token          fakeToken
	publishedTopic string
}

func (c *fakeClient) IsConnected() bool {
	return c.connected
}

func (c *fakeClient) IsConnectionOpen() bool {
	return c.connected
}

func (c *fakeClient) Connect() MQTT.Token {
	return c.token
}

func (c *fakeClient) Disconnect(uint) {}

func (c *fakeClient) Publish(topic string, qos byte, retained bool, payload interface{}) MQTT.Token {
	c.publishedTopic = topic
	return c.token
}

func (c *fakeClient) Subscribe(topic string, qos byte, callback MQTT.MessageHandler) MQTT.Token {
	return c.token
}

func (c *fakeClient) SubscribeMultiple(filters map[string]byte, callback MQTT.MessageHandler) MQTT.Token {
	return c.token
}

func (c *fakeClient) Unsubscribe(topics ...string) MQTT.Token {
	return c.token
}

func (c *fakeClient) AddRoute(topic string, callback MQTT.MessageHandler) {}

func (c *fakeClient) OptionsReader() MQTT.ClientOptionsReader {
	return MQTT.ClientOptionsReader{}
}
