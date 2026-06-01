//go:build smoke_tools

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	base  string
	token string
	http  *http.Client
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
}

type project struct {
	ID          uint   `json:"id"`
	ProjectCode string `json:"project_code"`
}

type tag struct {
	VarID     int64  `json:"var_id"`
	VarIDText string `json:"var_id_text"`
}

type taskFlow struct {
	ID       uint64 `json:"id"`
	FlowCode string `json:"flow_code"`
}

type taskFlowRun struct {
	ID         uint64 `json:"id"`
	FlowCode   string `json:"flow_code"`
	Status     string `json:"status"`
	ResultJSON string `json:"result_json"`
}

type listResponse[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type wsMessage struct {
	Type      string          `json:"type"`
	CommandID string          `json:"command_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *wsError        `json:"error,omitempty"`
}

type wsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type latencySample struct {
	ID       string
	Latency  time.Duration
	RunID    uint64
	WriteErr error
	RunErr   error
}

func main() {
	base := flag.String("base", "http://127.0.0.1:18080", "backend base URL")
	user := flag.String("user", "admin", "login username")
	pass := flag.String("pass", "Admin@12345", "login password")
	total := flag.Int("n", 30, "total virtual task requests")
	concurrency := flag.Int("c", 10, "concurrent request workers")
	timeout := flag.Duration("timeout", 15*time.Second, "max wait for task-flow runs")
	p95Max := flag.Duration("p95-max", 0, "optional p95 latency threshold; 0 disables this gate")
	maxMax := flag.Duration("max-max", 0, "optional max latency threshold; 0 disables this gate")
	flag.Parse()

	if err := run(*base, *user, *pass, *total, *concurrency, *timeout, *p95Max, *maxMax); err != nil {
		fmt.Fprintln(os.Stderr, "latency smoke failed:", err)
		os.Exit(1)
	}
}

func run(base, username, password string, total int, concurrency int, timeout time.Duration, p95Max time.Duration, maxMax time.Duration) error {
	if total <= 0 {
		return fmt.Errorf("n must be positive")
	}
	if concurrency <= 0 {
		return fmt.Errorf("c must be positive")
	}
	if concurrency > total {
		concurrency = total
	}
	c := &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 15 * time.Second}}
	if err := c.login(username, password); err != nil {
		return err
	}
	suffix := time.Now().Format("20060102150405")
	p, err := c.createProject(suffix)
	if err != nil {
		return err
	}
	requestVar, err := c.createVirtualStringVariable(p, "smoke_latency_request_"+suffix)
	if err != nil {
		return err
	}
	flow, err := c.createLatencyTaskFlow(p, requestVar, suffix)
	if err != nil {
		return err
	}
	fmt.Printf("latency setup ok: project=%s/%d request_var=%s flow=%s\n", p.ProjectCode, p.ID, requestVar.VarIDText, flow.FlowCode)

	jobs := make(chan int, total)
	results := make([]latencySample, total)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				latencyID := fmt.Sprintf("lat-%s-%03d", suffix, index)
				started := time.Now()
				payload := map[string]any{
					"command":             "latency_probe",
					"latency_id":          latencyID,
					"sent_at_unix_nano":   started.UnixNano(),
					"project_id":          p.ID,
					"ignore_io_kio_mqtt":  true,
					"measurement_version": 1,
				}
				sample := latencySample{ID: latencyID}
				if err := c.writeVariableWS(requestVar.VarID, payload, "latency-"+latencyID); err != nil {
					sample.WriteErr = err
					results[index] = sample
					continue
				}
				run, err := c.waitLatencyRun(flow.FlowCode, latencyID, timeout)
				if err != nil {
					sample.RunErr = err
					results[index] = sample
					continue
				}
				sample.RunID = run.ID
				sample.Latency = time.Since(started)
				results[index] = sample
			}
		}()
	}
	for i := 0; i < total; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return printSummary(results, total, concurrency, p95Max, maxMax)
}

func printSummary(samples []latencySample, total int, concurrency int, p95Max time.Duration, maxMax time.Duration) error {
	latencies := make([]time.Duration, 0, len(samples))
	var failed []latencySample
	for _, sample := range samples {
		if sample.WriteErr != nil || sample.RunErr != nil || sample.Latency <= 0 {
			failed = append(failed, sample)
			continue
		}
		latencies = append(latencies, sample.Latency)
	}
	if len(failed) > 0 {
		for _, sample := range failed {
			fmt.Printf("latency failed: id=%s write_err=%v run_err=%v\n", sample.ID, sample.WriteErr, sample.RunErr)
		}
		return fmt.Errorf("%d/%d latency samples failed", len(failed), total)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := percentile(latencies, 0.50)
	p90 := percentile(latencies, 0.90)
	p95 := percentile(latencies, 0.95)
	maxLatency := latencies[len(latencies)-1]
	avg := average(latencies)
	fmt.Printf("latency smoke passed: total=%d concurrency=%d\n", total, concurrency)
	fmt.Printf("latency ms: min=%.1f p50=%.1f p90=%.1f p95=%.1f max=%.1f avg=%.1f\n",
		ms(latencies[0]),
		ms(p50),
		ms(p90),
		ms(p95),
		ms(maxLatency),
		ms(avg),
	)
	if p95Max > 0 || maxMax > 0 {
		fmt.Printf("latency gates: p95_max=%s max_max=%s\n", formatGate(p95Max), formatGate(maxMax))
	}
	if p95Max > 0 && p95 > p95Max {
		return fmt.Errorf("p95 latency %s exceeds threshold %s", p95.Round(time.Millisecond), p95Max)
	}
	if maxMax > 0 && maxLatency > maxMax {
		return fmt.Errorf("max latency %s exceeds threshold %s", maxLatency.Round(time.Millisecond), maxMax)
	}
	return nil
}

func percentile(items []time.Duration, p float64) time.Duration {
	if len(items) == 0 {
		return 0
	}
	index := int(float64(len(items)-1) * p)
	return items[index]
}

func average(items []time.Duration) time.Duration {
	var total time.Duration
	for _, item := range items {
		total += item
	}
	return total / time.Duration(len(items))
}

func ms(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}

func formatGate(value time.Duration) string {
	if value <= 0 {
		return "disabled"
	}
	return value.String()
}

func (c *client) login(username, password string) error {
	var out loginResponse
	if err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password}, &out); err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("login response missing access_token")
	}
	c.token = out.AccessToken
	return nil
}

func (c *client) createProject(suffix string) (project, error) {
	var p project
	err := c.do(http.MethodPost, "/api/v1/projects", map[string]any{
		"project_code":    "SMOKE-LAT-" + suffix,
		"name":            "Latency Smoke",
		"display_name":    "Latency Smoke",
		"display_name_en": "Latency Smoke",
		"display_name_ja": "Latency Smoke",
	}, &p)
	return p, err
}

func (c *client) createVirtualStringVariable(p project, name string) (tag, error) {
	var out tag
	err := c.do(http.MethodPost, "/api/v1/variables", map[string]any{
		"var_id":          safeVarID(),
		"source_type":     "virtual",
		"project_id":      p.ID,
		"var_group":       "latency smoke",
		"var_name":        name,
		"display_name":    name,
		"display_name_en": name,
		"display_name_ja": name,
		"data_type":       "STRING",
		"enabled":         true,
	}, &out)
	return out, err
}

func (c *client) createLatencyTaskFlow(p project, requestVar tag, suffix string) (taskFlow, error) {
	var out taskFlow
	steps, _ := json.Marshal([]map[string]any{{
		"code":   "mark",
		"module": "builtin.context_set",
		"params": map[string]any{
			"latency_id":        map[string]any{"source": "trigger_param", "key": "latency_id"},
			"sent_at_unix_nano": map[string]any{"source": "trigger_param", "key": "sent_at_unix_nano"},
		},
	}})
	err := c.do(http.MethodPost, "/api/v1/task-flows", map[string]any{
		"project_id":   p.ID,
		"flow_code":    "smoke_latency_" + suffix,
		"name":         "Latency Smoke",
		"enabled":      true,
		"trigger_type": "data_change",
		"action_type":  "builtin.context_set",
		"steps_json":   string(steps),
		"timeout_ms":   3000,
		"cooldown_ms":  0,
		"hold_ms":      0,
		"priority":     100,
		"vars": []map[string]any{{
			"var_id":   requestVar.VarIDText,
			"var_name": "latency_request",
			"role":     "watch",
		}},
	}, &out)
	return out, err
}

func (c *client) writeVariableWS(varID int64, payload map[string]any, commandID string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	wsURL, err := c.wsURL("realtime.variables")
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.Type == "connection.ready" {
			break
		}
	}
	if err := conn.WriteJSON(map[string]any{
		"type":       "command.write_variable",
		"request_id": "req-" + commandID,
		"command_id": commandID,
		"payload": map[string]any{
			"var_id":  fmt.Sprintf("%d", varID),
			"value":   string(raw),
			"trigger": true,
		},
	}); err != nil {
		return err
	}
	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.CommandID != commandID {
			continue
		}
		if msg.Error != nil {
			return fmt.Errorf("ws write failed: %s %s", msg.Error.Code, msg.Error.Message)
		}
		if msg.Type != "command.ack" {
			return fmt.Errorf("unexpected ws response type %s", msg.Type)
		}
		return nil
	}
}

func (c *client) waitLatencyRun(flowCode string, latencyID string, timeout time.Duration) (taskFlowRun, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var out listResponse[taskFlowRun]
		path := fmt.Sprintf("/api/v1/task-flow-runs?flow_code=%s&status=success&limit=100", url.QueryEscape(flowCode))
		if err := c.do(http.MethodGet, path, nil, &out); err != nil {
			return taskFlowRun{}, err
		}
		for _, run := range out.Items {
			if run.FlowCode == flowCode && strings.Contains(run.ResultJSON, latencyID) {
				return run, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return taskFlowRun{}, fmt.Errorf("timeout waiting latency_id=%s", latencyID)
}

func (c *client) wsURL(topic string) (string, error) {
	u, err := url.Parse(c.base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/ws"
	q := u.Query()
	q.Set("access_token", c.token)
	q.Set("topic", topic)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *client) do(method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s status=%d body=%s", method, path, resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s: %w body=%s", method, path, err, string(raw))
	}
	return nil
}

func safeVarID() int64 {
	return 700_000_000 + rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(199_999_999)
}
