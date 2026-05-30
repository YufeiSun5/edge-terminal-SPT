//go:build perf_tools

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type payload struct {
	PVs  map[string]any   `json:"PVs"`
	Objs []map[string]any `json:"Objs"`
}

func main() {
	broker := flag.String("broker", "tcp://127.0.0.1:1883", "MQTT broker URL")
	clientID := flag.String("client-id", "edge-pubperf", "MQTT client id")
	username := flag.String("username", "Admin", "MQTT username")
	password := flag.String("password", "admin", "MQTT password")
	topic := flag.String("topic", "datachange_S_KIO_Project", "MQTT topic")
	qos := flag.Int("qos", 2, "MQTT QoS")
	duration := flag.Duration("duration", 10*time.Minute, "publish duration")
	interval := flag.Duration("interval", time.Second, "publish interval")
	vars := flag.Int("vars", 520, "variables per message")
	pattern := flag.String("pattern", "ramp", "value pattern: ramp or alarm")
	alarmPeriod := flag.Int("alarm-period", 20, "alarm pattern cycle length in messages")
	flag.Parse()

	opts := MQTT.NewClientOptions()
	opts.AddBroker(*broker)
	opts.SetClientID(fmt.Sprintf("%s_%d", *clientID, time.Now().UnixNano()))
	opts.SetUsername(*username)
	opts.SetPassword(*password)
	opts.SetCleanSession(true)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(false)

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("connect failed: %v", token.Error())
	}
	defer client.Disconnect(250)

	started := time.Now()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	deadline := time.NewTimer(*duration)
	defer deadline.Stop()

	var messages int64
	var updates int64
	log.Printf("mqttpubperf started broker=%s topic=%s vars=%d interval=%s duration=%s pattern=%s", *broker, *topic, *vars, interval.String(), duration.String(), *pattern)
	for {
		select {
		case now := <-ticker.C:
			raw, count, err := makePayload(*vars, now, messages, *pattern, *alarmPeriod)
			if err != nil {
				log.Fatalf("make payload: %v", err)
			}
			token := client.Publish(*topic, byte(*qos), false, raw)
			token.Wait()
			if token.Error() != nil {
				log.Fatalf("publish failed: %v", token.Error())
			}
			messages++
			updates += int64(count)
		case <-deadline.C:
			log.Printf("mqttpubperf finished duration=%.1fs messages=%d updates=%d", time.Since(started).Seconds(), messages, updates)
			return
		}
	}
}

func makePayload(vars int, sourceTime time.Time, seq int64, pattern string, alarmPeriod int) ([]byte, int, error) {
	if vars <= 0 {
		vars = 1
	}
	if alarmPeriod < 2 {
		alarmPeriod = 2
	}
	objects := make([]map[string]any, 0, vars)
	for i := 0; i < vars; i++ {
		value := perfValue(seq, i, pattern, alarmPeriod)
		objects = append(objects, map[string]any{
			"N": fmt.Sprintf("perf_%04d", i+1),
			"1": value,
			"3": 192,
		})
	}
	msg := payload{
		PVs: map[string]any{
			"2": sourceTime.Format("2006-01-02 15:04:05.000 -0700"),
			"3": 192,
		},
		Objs: objects,
	}
	raw, err := json.Marshal(msg)
	return raw, len(objects), err
}

func perfValue(seq int64, idx int, pattern string, alarmPeriod int) float64 {
	switch pattern {
	case "alarm":
		pos := int(seq) % alarmPeriod
		quarter := alarmPeriod / 4
		if quarter <= 0 {
			quarter = 1
		}
		if pos < quarter {
			return 60 + math.Mod(float64(idx), 1000)/1000
		}
		if pos >= quarter*2 && pos < quarter*3 {
			return -60 - math.Mod(float64(idx), 1000)/1000
		}
		return 0
	default:
		return float64(seq) + math.Mod(float64(idx), 1000)/1000
	}
}
