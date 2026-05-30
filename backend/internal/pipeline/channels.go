package pipeline

import "spindle-edge/backend/internal/models"

type Channels struct {
	Logic     chan *models.MQTTMessage
	Discovery chan *models.MQTTMessage
	Store     chan *models.StoreTask
	Alarm     chan *models.DetectionLimitAlarmEvent
	Notify    chan *models.RuntimeNotification
}

func NewChannels() *Channels {
	return &Channels{
		Logic:     make(chan *models.MQTTMessage, 2000),
		Discovery: make(chan *models.MQTTMessage, 200),
		Store:     make(chan *models.StoreTask, 1000),
		Alarm:     make(chan *models.DetectionLimitAlarmEvent, 5000),
		Notify:    make(chan *models.RuntimeNotification, 2000),
	}
}

func (c *Channels) Stats() map[string]int {
	return map[string]int{
		"logic":     len(c.Logic),
		"discovery": len(c.Discovery),
		"store":     len(c.Store),
		"alarm":     len(c.Alarm),
		"notify":    len(c.Notify),
	}
}
