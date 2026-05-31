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
	"strings"
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
	VarID int64 `json:"var_id"`
}

type storageRoute struct {
	ID        uint64 `json:"id"`
	Enabled   bool   `json:"enabled"`
	CycleMS   int    `json:"cycle_ms"`
	Column    string `json:"column_name"`
	TableName string `json:"table_name"`
}

type detectionTask struct {
	ID        uint   `json:"id"`
	ProjectID uint   `json:"project_id"`
	TestNo    string `json:"test_no"`
	Status    string `json:"status"`
	EndType   string `json:"end_type"`
}

type listResponse[T any] struct {
	Items []T `json:"items"`
	Count int `json:"count"`
}

type historyRow struct {
	TaskID    uint     `json:"task_id"`
	ProjectID uint     `json:"project_id"`
	VarID     int64    `json:"var_id"`
	Value     *float64 `json:"value"`
}

type wsMessage struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	CommandID string          `json:"command_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *wsError        `json:"error,omitempty"`
}

type wsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	base := flag.String("base", "http://127.0.0.1:18080", "backend base URL")
	user := flag.String("user", "admin", "login username")
	pass := flag.String("pass", "Admin@12345", "login password")
	wait := flag.Duration("wait", 3*time.Second, "time to wait for cycle storage")
	flag.Parse()

	if err := run(*base, *user, *pass, *wait); err != nil {
		fmt.Fprintln(os.Stderr, "EB-017 smoke failed:", err)
		os.Exit(1)
	}
}

func run(base, username, password string, wait time.Duration) error {
	c := &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 15 * time.Second}}
	if err := c.login(username, password); err != nil {
		return err
	}
	fmt.Println("login ok")

	suffix := time.Now().Format("20060102150405")
	projects, err := c.createProjects(suffix)
	if err != nil {
		return err
	}
	fmt.Printf("projects ok: %s=%d %s=%d\n", projects[0].ProjectCode, projects[0].ID, projects[1].ProjectCode, projects[1].ID)

	varA, err := c.createVirtualVariable(projects[0], "INT", "smoke_eb017_int_"+suffix)
	if err != nil {
		return err
	}
	varAString, err := c.createVirtualVariable(projects[0], "STRING", "smoke_eb017_string_"+suffix)
	if err != nil {
		return err
	}
	varB, err := c.createVirtualVariable(projects[1], "INT", "smoke_eb017_int_b_"+suffix)
	if err != nil {
		return err
	}
	fmt.Printf("variables ok: intA=%d stringA=%d intB=%d\n", varA.VarID, varAString.VarID, varB.VarID)

	if err := c.enableCycleRoute(projects[0].ID, varA.VarID); err != nil {
		return err
	}
	if err := c.enableCycleRoute(projects[1].ID, varB.VarID); err != nil {
		return err
	}
	fmt.Println("storage routes enabled")

	runA, err := c.startRun(projects[0].ID, "SMOKE-EB017-A-"+suffix)
	if err != nil {
		return err
	}
	runB, err := c.startRun(projects[1].ID, "SMOKE-EB017-B-"+suffix)
	if err != nil {
		_, _ = c.stopRun(runA.ID, "cleanup after start B failure")
		return err
	}
	defer func() {
		_, _ = c.stopRun(runA.ID, "smoke cleanup")
		_, _ = c.stopRun(runB.ID, "smoke cleanup")
	}()
	if _, err := c.startRun(projects[0].ID, "SMOKE-EB017-DUP-"+suffix); err == nil {
		return fmt.Errorf("expected same-project duplicate start to return conflict")
	} else if !strings.Contains(err.Error(), "409") {
		return fmt.Errorf("expected duplicate start conflict, got %v", err)
	}
	fmt.Printf("parallel runs ok: taskA=%d taskB=%d duplicate=409\n", runA.ID, runB.ID)

	if err := c.writeVariablesWS(map[int64]any{
		varA.VarID:       101,
		varAString.VarID: "string-ok-" + suffix,
		varB.VarID:       202,
	}); err != nil {
		return err
	}
	fmt.Println("websocket virtual writes ok")

	time.Sleep(wait)
	if err := c.expectHistory(runA.ID, projects[0].ID, varA.VarID); err != nil {
		return err
	}
	if err := c.expectHistory(runB.ID, projects[1].ID, varB.VarID); err != nil {
		return err
	}
	fmt.Println("history rows ok")

	stoppedA, err := c.stopRun(runA.ID, "smoke stop A")
	if err != nil {
		return err
	}
	if stoppedA.Status != "stopped" || stoppedA.EndType == "abnormal_stop" {
		return fmt.Errorf("unexpected stopped A state: status=%s end_type=%s", stoppedA.Status, stoppedA.EndType)
	}
	currentB, err := c.currentRun(projects[1].ID)
	if err != nil {
		return err
	}
	if currentB.ID != runB.ID || currentB.Status != "running" {
		return fmt.Errorf("stopping project A affected project B: current=%+v", currentB)
	}
	stoppedB, err := c.stopRun(runB.ID, "smoke stop B")
	if err != nil {
		return err
	}
	if stoppedB.Status != "stopped" {
		return fmt.Errorf("unexpected stopped B state: status=%s", stoppedB.Status)
	}
	fmt.Println("stop isolation ok")
	fmt.Printf("EB-017 smoke passed: projects=%d,%d tasks=%d,%d vars=%d,%d,%d\n", projects[0].ID, projects[1].ID, runA.ID, runB.ID, varA.VarID, varAString.VarID, varB.VarID)
	return nil
}

func (c *client) login(username, password string) error {
	var out loginResponse
	if err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password}, &out); err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("login response has empty access_token")
	}
	c.token = out.AccessToken
	return nil
}

func (c *client) createProjects(suffix string) ([]project, error) {
	out := make([]project, 0, 2)
	for _, codeSuffix := range []string{"A", "B"} {
		var p project
		code := "SMOKE-EB017-" + suffix + "-" + codeSuffix
		err := c.do(http.MethodPost, "/api/v1/projects", map[string]any{
			"project_code":    code,
			"name":            "EB017 Smoke " + codeSuffix,
			"display_name":    "EB017 Smoke " + codeSuffix,
			"display_name_en": "EB017 Smoke " + codeSuffix,
			"display_name_ja": "EB017 Smoke " + codeSuffix,
		}, &p)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (c *client) createVirtualVariable(p project, dataType string, name string) (tag, error) {
	var out tag
	payload := map[string]any{
		"var_id":          safeVarID(),
		"source_type":     "virtual",
		"project_id":      p.ID,
		"var_group":       "EB017 smoke",
		"var_name":        name,
		"display_name":    name,
		"display_name_en": name,
		"display_name_ja": name,
		"data_type":       dataType,
		"enabled":         true,
	}
	if err := c.do(http.MethodPost, "/api/v1/variables", payload, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *client) enableCycleRoute(projectID uint, varID int64) error {
	var routes []storageRoute
	path := fmt.Sprintf("/api/v1/storage-routes?project_id=%d&var_id=%d", projectID, varID)
	if err := c.do(http.MethodGet, path, nil, &routes); err != nil {
		return err
	}
	if len(routes) == 0 {
		return fmt.Errorf("no default storage route for var_id=%d", varID)
	}
	patch := map[string]any{
		"enabled":        true,
		"trigger_mode":   "on_cycle",
		"cycle_ms":       500,
		"store_on_start": true,
	}
	var updated storageRoute
	if err := c.do(http.MethodPatch, fmt.Sprintf("/api/v1/storage-routes/%d", routes[0].ID), patch, &updated); err != nil {
		return err
	}
	if !updated.Enabled || updated.CycleMS != 500 {
		return fmt.Errorf("storage route update did not apply: %+v", updated)
	}
	return nil
}

func (c *client) startRun(projectID uint, testNo string) (detectionTask, error) {
	var out detectionTask
	err := c.do(http.MethodPost, "/api/v1/detection-runs", map[string]any{
		"project_id": projectID,
		"test_no":    testNo,
		"mode":       "smoke",
	}, &out)
	return out, err
}

func (c *client) stopRun(taskID uint, reason string) (detectionTask, error) {
	var out detectionTask
	err := c.do(http.MethodPost, fmt.Sprintf("/api/v1/detection-runs/%d/stop", taskID), map[string]any{"reason": reason}, &out)
	return out, err
}

func (c *client) currentRun(projectID uint) (detectionTask, error) {
	var out detectionTask
	err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/detection-runs/current?project_id=%d", projectID), nil, &out)
	return out, err
}

func (c *client) expectHistory(taskID uint, projectID uint, varID int64) error {
	path := fmt.Sprintf("/api/v1/history/data?task_id=%d&project_id=%d&limit=20", taskID, projectID)
	var out listResponse[historyRow]
	if err := c.do(http.MethodGet, path, nil, &out); err != nil {
		return err
	}
	for _, row := range out.Items {
		if row.TaskID == taskID && row.ProjectID == projectID && row.VarID == varID {
			return nil
		}
	}
	return fmt.Errorf("history row not found for task_id=%d project_id=%d var_id=%d rows=%d", taskID, projectID, varID, len(out.Items))
}

func (c *client) writeVariablesWS(values map[int64]any) error {
	wsURL, err := c.wsURL()
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for len(values) > 0 {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.Type == "connection.ready" {
			break
		}
	}
	for varID, value := range values {
		commandID := fmt.Sprintf("cmd-eb017-%d", varID)
		payload := map[string]any{"var_id": varID, "value": value, "trigger": false}
		if err := conn.WriteJSON(map[string]any{
			"type":       "command.write_variable",
			"request_id": "req-" + commandID,
			"command_id": commandID,
			"payload":    payload,
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
				return fmt.Errorf("ws write %d failed: %s %s", varID, msg.Error.Code, msg.Error.Message)
			}
			if msg.Type != "command.ack" {
				return fmt.Errorf("ws write %d returned unexpected type %s", varID, msg.Type)
			}
			break
		}
	}
	return nil
}

func (c *client) wsURL() (string, error) {
	parsed, err := url.Parse(c.base)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/ws"
	query := parsed.Query()
	query.Set("access_token", c.token)
	query.Set("topic", "realtime.variables")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
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
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s: %w body=%s", method, path, err, string(raw))
	}
	return nil
}

func safeVarID() int64 {
	return 700_000_000 + rand.New(rand.NewSource(time.Now().UnixNano())).Int63n(200_000_000)
}
