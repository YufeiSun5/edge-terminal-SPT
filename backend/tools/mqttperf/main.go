package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"spindle-edge/backend/internal/protocol/kio"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/tidwall/gjson"
)

type varStat struct {
	Name        string    `json:"name"`
	Count       int64     `json:"count"`
	LastValue   string    `json:"last_value"`
	LastAt      time.Time `json:"last_at"`
	LastSource  time.Time `json:"last_source"`
	MinLatency  float64   `json:"min_latency_ms"`
	MaxLatency  float64   `json:"max_latency_ms"`
	SumLatency  float64   `json:"-"`
	LatencyN    int64     `json:"latency_n"`
	BadQuality  int64     `json:"bad_quality"`
	Regressions int64     `json:"regressions"`
	Gaps        int64     `json:"gaps"`
	lastNumber  float64
	hasNumber   bool
}

type summary struct {
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSec     float64   `json:"duration_sec"`
	Messages        int64     `json:"messages"`
	InvalidMessages int64     `json:"invalid_messages"`
	Updates         int64     `json:"updates"`
	UniqueVariables int       `json:"unique_variables"`
	BadQuality      int64     `json:"bad_quality"`
	LatencySamples  int64     `json:"latency_samples"`
	MinLatencyMs    float64   `json:"min_latency_ms"`
	AvgLatencyMs    float64   `json:"avg_latency_ms"`
	P95LatencyMs    float64   `json:"p95_latency_ms"`
	MaxLatencyMs    float64   `json:"max_latency_ms"`
	StaleOver2s     int       `json:"stale_over_2s"`
	StaleOver5s     int       `json:"stale_over_5s"`
	StaleOver10s    int       `json:"stale_over_10s"`
	ThirtyEight     []varStat `json:"thirty_eight_variables"`
	WorstStale      []varStat `json:"worst_stale_variables"`
}

func main() {
	broker := flag.String("broker", "tcp://127.0.0.1:1883", "MQTT broker URL")
	clientID := flag.String("client-id", "edge-perf", "MQTT client id")
	username := flag.String("username", "Admin", "MQTT username")
	password := flag.String("password", "admin", "MQTT password")
	topic := flag.String("topic", "datachange_S_KIO_Project", "MQTT topic")
	qos := flag.Int("qos", 2, "MQTT QoS")
	duration := flag.Duration("duration", 30*time.Minute, "test duration")
	reportEvery := flag.Duration("report-every", 30*time.Second, "progress interval")
	out := flag.String("out", "mqttperf_result.json", "summary output path")
	flag.Parse()

	stats := newCollector()
	opts := MQTT.NewClientOptions()
	opts.AddBroker(*broker)
	opts.SetClientID(fmt.Sprintf("%s_%d", *clientID, time.Now().UnixNano()))
	opts.SetUsername(*username)
	opts.SetPassword(*password)
	opts.SetCleanSession(true)
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(false)
	opts.SetDefaultPublishHandler(func(_ MQTT.Client, msg MQTT.Message) {
		stats.observe(msg.Payload(), time.Now())
	})

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("connect failed: %v", token.Error())
	}
	defer client.Disconnect(250)

	if token := client.Subscribe(*topic, byte(*qos), nil); token.Wait() && token.Error() != nil {
		log.Fatalf("subscribe failed: %v", token.Error())
	}

	started := time.Now()
	log.Printf("mqttperf started broker=%s topic=%s duration=%s", *broker, *topic, duration.String())
	ticker := time.NewTicker(*reportEvery)
	defer ticker.Stop()
	deadline := time.NewTimer(*duration)
	defer deadline.Stop()

	for {
		select {
		case <-ticker.C:
			printProgress(stats.snapshot(started, time.Now()))
		case <-deadline.C:
			ended := time.Now()
			result := stats.snapshot(started, ended)
			printProgress(result)
			if err := writeJSON(*out, result); err != nil {
				log.Fatalf("write result: %v", err)
			}
			log.Printf("mqttperf finished result=%s", *out)
			return
		}
	}
}

type collector struct {
	mu              sync.Mutex
	messages        int64
	invalidMessages int64
	updates         int64
	badQuality      int64
	vars            map[string]*varStat
	latencies       []float64
}

func newCollector() *collector {
	return &collector{vars: make(map[string]*varStat), latencies: make([]float64, 0, 4096)}
}

func (c *collector) observe(payload []byte, received time.Time) {
	updates, err := kio.ParseUpdates(payload)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages++
	if err != nil {
		c.invalidMessages++
		return
	}
	for _, update := range updates {
		stat := c.vars[update.Name]
		if stat == nil {
			stat = &varStat{Name: update.Name, MinLatency: math.Inf(1), MaxLatency: math.Inf(-1)}
			c.vars[update.Name] = stat
		}
		c.updates++
		stat.Count++
		stat.LastAt = received
		stat.LastSource = update.SourceTime
		stat.LastValue = update.Value.String()
		if update.Quality != 1 {
			c.badQuality++
			stat.BadQuality++
		}
		if !update.SourceTime.IsZero() {
			latency := received.Sub(update.SourceTime).Seconds() * 1000
			if latency > -60_000 && latency < 60_000 {
				c.latencies = append(c.latencies, latency)
				stat.SumLatency += latency
				stat.LatencyN++
				if latency < stat.MinLatency {
					stat.MinLatency = latency
				}
				if latency > stat.MaxLatency {
					stat.MaxLatency = latency
				}
			}
		}
		if strings.HasSuffix(update.Name, "_38") && update.Value.Type == gjson.Number {
			value := update.Value.Float()
			if stat.hasNumber {
				if value < stat.lastNumber {
					stat.Regressions++
				}
				if value-stat.lastNumber > 1 {
					stat.Gaps++
				}
			}
			stat.lastNumber = value
			stat.hasNumber = true
		}
	}
}

func (c *collector) snapshot(started time.Time, ended time.Time) summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	latencies := append([]float64(nil), c.latencies...)
	sort.Float64s(latencies)
	result := summary{
		StartedAt:       started,
		EndedAt:         ended,
		DurationSec:     ended.Sub(started).Seconds(),
		Messages:        c.messages,
		InvalidMessages: c.invalidMessages,
		Updates:         c.updates,
		UniqueVariables: len(c.vars),
		BadQuality:      c.badQuality,
		LatencySamples:  int64(len(latencies)),
	}
	if len(latencies) > 0 {
		result.MinLatencyMs = latencies[0]
		result.MaxLatencyMs = latencies[len(latencies)-1]
		result.P95LatencyMs = percentile(latencies, 0.95)
		var sum float64
		for _, latency := range latencies {
			sum += latency
		}
		result.AvgLatencyMs = sum / float64(len(latencies))
	}

	all := make([]varStat, 0, len(c.vars))
	for _, stat := range c.vars {
		item := *stat
		if item.LatencyN > 0 {
			item.SumLatency = item.SumLatency / float64(item.LatencyN)
		}
		if math.IsInf(item.MinLatency, 1) {
			item.MinLatency = 0
		}
		if math.IsInf(item.MaxLatency, -1) {
			item.MaxLatency = 0
		}
		all = append(all, item)
		age := ended.Sub(item.LastAt)
		if age > 2*time.Second {
			result.StaleOver2s++
		}
		if age > 5*time.Second {
			result.StaleOver5s++
		}
		if age > 10*time.Second {
			result.StaleOver10s++
		}
		if strings.HasSuffix(item.Name, "_38") {
			result.ThirtyEight = append(result.ThirtyEight, item)
		}
	}
	sort.Slice(result.ThirtyEight, func(i, j int) bool {
		return result.ThirtyEight[i].Name < result.ThirtyEight[j].Name
	})
	sort.Slice(all, func(i, j int) bool {
		return all[i].LastAt.Before(all[j].LastAt)
	})
	if len(all) > 10 {
		result.WorstStale = all[:10]
	} else {
		result.WorstStale = all
	}
	return result
}

func printProgress(result summary) {
	log.Printf(
		"progress %.0fs messages=%d updates=%d unique=%d latency_avg=%.1fms p95=%.1fms max=%.1fms stale>5s=%d bad_quality=%d",
		result.DurationSec,
		result.Messages,
		result.Updates,
		result.UniqueVariables,
		result.AvgLatencyMs,
		result.P95LatencyMs,
		result.MaxLatencyMs,
		result.StaleOver5s,
		result.BadQuality,
	)
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(values))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(values) {
		idx = len(values) - 1
	}
	return values[idx]
}

func writeJSON(path string, result summary) error {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}
