package services

import (
	"context"
	"fmt"
	"time"

	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/protocol/kio"
)

type KIOBroker interface {
	Config(gatewayID int) (models.GatewayConfig, bool)
	Publish(ctx context.Context, gatewayID int, topic string, payload []byte, qos byte, retain bool) error
	Subscribe(ctx context.Context, gatewayID int, topic string, qos byte) error
	PublishAndWaitKIOAck(ctx context.Context, gatewayID int, topic string, payload []byte, qos byte, retain bool, qid int64) (*kio.WriteAck, bool, error)
}

type KIOWriteService struct {
	broker KIOBroker
}

type KIOWriteInput struct {
	GatewayID     int
	ClientID      string
	Topic         string
	AckTopic      string
	Writer        string
	WriteTime     string
	Username      string
	Password      string
	QID           int64
	Values        []kio.WriteValue
	QOS           byte
	Retain        bool
	WaitAck       bool
	AckTimeoutSec int
}

type KIOWriteResult struct {
	GatewayID        int    `json:"gateway_id"`
	Topic            string `json:"topic"`
	AckTopic         string `json:"ack_topic,omitempty"`
	QID              int64  `json:"qid"`
	BrokerAccepted   bool   `json:"broker_accepted"`
	ProjectConfirmed bool   `json:"Project_confirmed"`
	ProcessStep      int    `json:"process_step,omitempty"`
	Result           string `json:"result,omitempty"`
	Message          string `json:"message,omitempty"`
	Status           string `json:"status"`
}

type KIOServiceError struct {
	Status  int
	Message string
}

func (e KIOServiceError) Error() string {
	return e.Message
}

func NewKIOWriteService(broker KIOBroker) *KIOWriteService {
	return &KIOWriteService{broker: broker}
}

func (s *KIOWriteService) Write(ctx context.Context, input KIOWriteInput) (KIOWriteResult, error) {
	if s == nil || s.broker == nil {
		return KIOWriteResult{}, KIOServiceError{Status: 502, Message: "kio broker is not available"}
	}
	gatewayCfg, ok := s.broker.Config(input.GatewayID)
	if !ok {
		return KIOWriteResult{}, KIOServiceError{Status: 400, Message: "gateway not found"}
	}
	clientID := firstNonEmpty(input.ClientID, gatewayCfg.KIOClientID)
	writer := firstNonEmpty(input.Writer, gatewayCfg.KIOWriter)
	username := firstNonEmpty(input.Username, gatewayCfg.KIOWriteUsername)
	password := firstNonEmpty(input.Password, gatewayCfg.KIOWritePassword)
	topic := firstNonEmpty(input.Topic, gatewayCfg.SetDataTopic)
	if topic == "" && clientID != "" {
		topic = kio.SetDataTopic(clientID)
	}
	if topic == "" {
		return KIOWriteResult{}, KIOServiceError{Status: 400, Message: "topic or client_id is required"}
	}
	if writer == "" {
		return KIOWriteResult{}, KIOServiceError{Status: 400, Message: "writer is required"}
	}
	payload, err := kio.BuildWritePayload(kio.WriteRequest{
		Writer:    writer,
		WriteTime: input.WriteTime,
		Username:  username,
		Password:  password,
		QID:       input.QID,
		Values:    input.Values,
	})
	if err != nil {
		return KIOWriteResult{}, KIOServiceError{Status: 400, Message: err.Error()}
	}
	qid := kio.QIDFromPayload(payload)
	qos := input.QOS
	if qos == 0 {
		qos = gatewayCfg.QOS
	}
	timeout := 10 * time.Second
	if input.AckTimeoutSec > 0 {
		timeout = time.Duration(input.AckTimeoutSec) * time.Second
	}
	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if !input.WaitAck {
		if err := s.broker.Publish(writeCtx, input.GatewayID, topic, payload, qos, input.Retain); err != nil {
			return KIOWriteResult{}, KIOServiceError{Status: 502, Message: err.Error()}
		}
		return KIOWriteResult{
			GatewayID:        input.GatewayID,
			Topic:            topic,
			QID:              qid,
			BrokerAccepted:   true,
			ProjectConfirmed: false,
			Status:           "published_unconfirmed",
		}, nil
	}

	ackTopic := firstNonEmpty(input.AckTopic, gatewayCfg.WriteResultTopic)
	if ackTopic == "" && clientID != "" {
		ackTopic = kio.SetDataResultTopic(clientID, writer)
	}
	if ackTopic != "" {
		if err := s.broker.Subscribe(writeCtx, input.GatewayID, ackTopic, qos); err != nil {
			return KIOWriteResult{}, KIOServiceError{Status: 502, Message: fmt.Sprintf("ack subscribe failed: %v", err)}
		}
	}
	ack, brokerAccepted, err := s.broker.PublishAndWaitKIOAck(writeCtx, input.GatewayID, topic, payload, qos, input.Retain, qid)
	if err != nil {
		status := 504
		if !brokerAccepted {
			status = 502
		}
		return KIOWriteResult{
			GatewayID:        input.GatewayID,
			Topic:            topic,
			AckTopic:         ackTopic,
			QID:              qid,
			BrokerAccepted:   brokerAccepted,
			ProjectConfirmed: false,
			Status:           "ack_timeout_or_unmatched",
		}, KIOServiceError{Status: status, Message: err.Error()}
	}
	resultStatus := "rejected"
	if ack.Success {
		resultStatus = "confirmed"
	}
	return KIOWriteResult{
		GatewayID:        input.GatewayID,
		Topic:            topic,
		AckTopic:         ackTopic,
		QID:              ack.QID,
		BrokerAccepted:   true,
		ProjectConfirmed: ack.Success,
		ProcessStep:      ack.ProcessStep,
		Result:           ack.Result,
		Message:          ack.Message,
		Status:           resultStatus,
	}, nil
}

func HTTPStatusForKIOError(err error) int {
	if typed, ok := err.(KIOServiceError); ok && typed.Status > 0 {
		return typed.Status
	}
	return 502
}
