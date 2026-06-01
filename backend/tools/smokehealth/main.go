//go:build smoke_tools

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type client struct {
	base  string
	token string
	http  *http.Client
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
}

type healthResponse struct {
	Status   string         `json:"status"`
	Tags     int            `json:"tags"`
	Channels map[string]int `json:"channels"`
	Gateways map[string]any `json:"gateways"`
}

type projectSummary struct {
	ID          uint   `json:"id"`
	ProjectCode string `json:"project_code"`
}

type namedCheck struct {
	Name string
	Path string
}

type workerDiagnostic struct {
	Name       string  `json:"name"`
	Active     *int    `json:"active"`
	Starts     *uint64 `json:"starts"`
	Exits      *uint64 `json:"exits"`
	Health     string  `json:"health"`
	Panics     *uint64 `json:"panics"`
	Impact     string  `json:"impact"`
	NextAction string  `json:"next_action"`
}

type channelDiagnostic struct {
	Name       string  `json:"name"`
	Dropped    *uint64 `json:"dropped"`
	Impact     string  `json:"impact"`
	NextAction string  `json:"next_action"`
}

type notificationDiagnostic struct {
	PressureThreshold float64 `json:"pressure_threshold"`
	Impact            string  `json:"impact"`
	NextAction        string  `json:"next_action"`
	Published         *uint64 `json:"published"`
	Delivered         *uint64 `json:"delivered"`
	Dropped           *uint64 `json:"dropped"`
}

var expectedCoreChannels = []string{"logic", "discovery", "store", "alarm", "notify"}
var expectedCoreWorkers = []string{
	"alarm-worker",
	"channel-pressure-logger",
	"cycle-scanner",
	"discovery",
	"logic",
	"notification-dispatcher",
	"storage-bus",
	"task-flow-schedule-scanner",
	"task-flow-worker",
}

func main() {
	base := flag.String("base", "http://127.0.0.1:18080", "backend base URL")
	user := flag.String("user", "admin", "login username")
	pass := flag.String("pass", "Admin@12345", "login password")
	flag.Parse()

	if err := run(*base, *user, *pass); err != nil {
		fmt.Fprintln(os.Stderr, "health smoke failed:", err)
		os.Exit(1)
	}
}

func run(base, username, password string) error {
	c := &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 15 * time.Second}}

	health, err := c.health()
	if err != nil {
		return err
	}
	fmt.Printf("health ok: status=%s tags=%d channels=%d gateways=%d\n", health.Status, health.Tags, len(health.Channels), len(health.Gateways))

	if err := c.login(username, password); err != nil {
		return err
	}
	fmt.Println("login ok")

	projects, err := c.projects()
	if err != nil {
		return fmt.Errorf("projects check failed: %w", err)
	}
	fmt.Printf("projects ok: count=%d\n", len(projects))
	if len(projects) > 0 {
		path := fmt.Sprintf("/api/v1/projects/%d/members", projects[0].ID)
		if err := c.expectJSON(http.MethodGet, path, http.StatusOK); err != nil {
			return fmt.Errorf("project members check failed: %w", err)
		}
		fmt.Printf("project members ok: project_id=%d\n", projects[0].ID)

		path = fmt.Sprintf("/api/v1/storage-routes?project_id=%d&enabled=true", projects[0].ID)
		if err := c.expectJSON(http.MethodGet, path, http.StatusOK); err != nil {
			return fmt.Errorf("storage routes project filter check failed: %w", err)
		}
		fmt.Printf("storage routes project filter ok: project_id=%d\n", projects[0].ID)

		path = fmt.Sprintf("/api/v1/detection-standards?project_id=%d&enabled=true", projects[0].ID)
		if err := c.expectJSON(http.MethodGet, path, http.StatusOK); err != nil {
			return fmt.Errorf("detection standards project filter check failed: %w", err)
		}
		fmt.Printf("detection standards project filter ok: project_id=%d\n", projects[0].ID)

		path = fmt.Sprintf("/api/v1/realtime/variables?project_id=%d", projects[0].ID)
		if err := c.expectJSON(http.MethodGet, path, http.StatusOK); err != nil {
			return fmt.Errorf("realtime variables project filter check failed: %w", err)
		}
		fmt.Printf("realtime variables project filter ok: project_id=%d\n", projects[0].ID)

		path = fmt.Sprintf("/api/v1/history/data?project_id=%d&limit=1", projects[0].ID)
		if err := c.expectJSON(http.MethodGet, path, http.StatusOK); err != nil {
			return fmt.Errorf("history data project filter check failed: %w", err)
		}
		fmt.Printf("history data project filter ok: project_id=%d\n", projects[0].ID)

		path = fmt.Sprintf("/api/v1/limit-alarms?project_id=%d&limit=1", projects[0].ID)
		if err := c.expectJSON(http.MethodGet, path, http.StatusOK); err != nil {
			return fmt.Errorf("limit alarms project filter check failed: %w", err)
		}
		fmt.Printf("limit alarms project filter ok: project_id=%d\n", projects[0].ID)

		path = fmt.Sprintf("/api/v1/detection-runs/current?project_id=%d", projects[0].ID)
		if status, err := c.expectJSONAnyStatus(http.MethodGet, path, http.StatusOK, http.StatusNotFound); err != nil {
			return fmt.Errorf("current detection run check failed: %w", err)
		} else {
			fmt.Printf("current detection run lookup ok: project_id=%d status=%d\n", projects[0].ID, status)
		}
	}

	checks := []namedCheck{
		{Name: "auth/me", Path: "/api/v1/auth/me"},
		{Name: "variables", Path: "/api/v1/variables"},
		{Name: "runtime/channels", Path: "/api/v1/runtime/channels"},
		{Name: "runtime/channels/detail", Path: "/api/v1/runtime/channels/detail"},
		{Name: "runtime/notifications", Path: "/api/v1/runtime/notifications"},
		{Name: "runtime/workers", Path: "/api/v1/runtime/workers"},
		{Name: "gateway-configs", Path: "/api/v1/gateway-configs"},
		{Name: "audit-logs filtered", Path: "/api/v1/audit-logs?action=auth.login&result=success&limit=1"},
		{Name: "report-templates", Path: "/api/v1/report-templates?enabled=true"},
		{Name: "task-modules", Path: "/api/v1/task-modules"},
		{Name: "task-flow-templates", Path: "/api/v1/task-flow-templates"},
		{Name: "task-flows/runtime", Path: "/api/v1/task-flows/runtime"},
		{Name: "task-flow-runs", Path: "/api/v1/task-flow-runs?limit=1"},
		{Name: "notifications/unread-count", Path: "/api/v1/notifications/unread-count"},
		{Name: "notifications filtered list", Path: "/api/v1/notifications?unread=true&type=detection.run_started&level=info&project_id=1&from=2026-01-01T00%3A00%3A00Z&to=2026-12-31T23%3A59%3A59Z&keyword=smoke&limit=1"},
		{Name: "notifications filtered unread-count", Path: "/api/v1/notifications/unread-count?type=detection.run_started&level=info&project_id=1&from=2026-01-01T00%3A00%3A00Z&to=2026-12-31T23%3A59%3A59Z&keyword=smoke"},
		{Name: "detection-runs/active", Path: "/api/v1/detection-runs/active"},
	}
	for _, check := range checks {
		if err := c.expectJSON(http.MethodGet, check.Path, http.StatusOK); err != nil {
			return fmt.Errorf("%s check failed: %w", check.Name, err)
		}
		fmt.Printf("%s ok\n", check.Name)
	}
	if err := c.expectRuntimeDiagnosticFields(); err != nil {
		return fmt.Errorf("runtime actionable diagnostics check failed: %w", err)
	}
	fmt.Println("runtime actionable diagnostics ok")
	if err := c.expectJSON(http.MethodPost, "/api/v1/notifications/read-all?type=detection.run_started&level=info&project_id=1&from=2026-01-01T00%3A00%3A00Z&to=2026-12-31T23%3A59%3A59Z&keyword=smoke", http.StatusOK); err != nil {
		return fmt.Errorf("notifications filtered read-all check failed: %w", err)
	}
	fmt.Println("notifications filtered read-all ok")

	if err := c.expectStatus(http.MethodGet, "/api/v1/devices", http.StatusNotFound); err != nil {
		return fmt.Errorf("legacy devices endpoint check failed: %w", err)
	}
	fmt.Println("legacy devices endpoint ok: 404")

	fmt.Println("health smoke passed")
	return nil
}

func (c *client) projects() ([]projectSummary, error) {
	var out []projectSummary
	if err := c.do(http.MethodGet, "/api/v1/projects", nil, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) health() (healthResponse, error) {
	var out healthResponse
	if err := c.do(http.MethodGet, "/health", nil, &out, http.StatusOK); err != nil {
		return healthResponse{}, err
	}
	if err := validateHealth(out); err != nil {
		return healthResponse{}, err
	}
	return out, nil
}

func validateHealth(out healthResponse) error {
	if strings.ToLower(out.Status) != "ok" {
		return fmt.Errorf("unexpected health status %q", out.Status)
	}
	return validateChannelMap("health response", out.Channels)
}

func (c *client) login(username, password string) error {
	var out loginResponse
	if err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password}, &out, http.StatusOK); err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("login response has empty access_token")
	}
	c.token = out.AccessToken
	return nil
}

func (c *client) expectJSON(method, path string, expectedStatus int) error {
	var raw json.RawMessage
	if _, err := c.doAny(method, path, nil, &raw, expectedStatus); err != nil {
		return err
	}
	if !json.Valid(raw) {
		return fmt.Errorf("response is not valid json")
	}
	return nil
}

func (c *client) expectJSONAnyStatus(method, path string, expectedStatuses ...int) (int, error) {
	var raw json.RawMessage
	status, err := c.doAny(method, path, nil, &raw, expectedStatuses...)
	if err != nil {
		return status, err
	}
	if !json.Valid(raw) {
		return status, fmt.Errorf("response is not valid json")
	}
	return status, nil
}

func (c *client) expectStatus(method, path string, expectedStatus int) error {
	_, err := c.doAny(method, path, nil, nil, expectedStatus)
	return err
}

func (c *client) expectRuntimeDiagnosticFields() error {
	var compactChannels map[string]int
	if err := c.do(http.MethodGet, "/api/v1/runtime/channels", nil, &compactChannels, http.StatusOK); err != nil {
		return err
	}
	if err := validateChannelMap("runtime/channels", compactChannels); err != nil {
		return err
	}

	var channels struct {
		Items []channelDiagnostic `json:"items"`
	}
	if err := c.do(http.MethodGet, "/api/v1/runtime/channels/detail", nil, &channels, http.StatusOK); err != nil {
		return err
	}
	if err := validateChannelDiagnostics(channels.Items); err != nil {
		return err
	}

	var flows struct {
		Queue struct {
			Name       string `json:"name"`
			Impact     string `json:"impact"`
			NextAction string `json:"next_action"`
		} `json:"queue"`
		Submitted *uint64 `json:"submitted"`
		Enqueued  *uint64 `json:"enqueued"`
		Dropped   *uint64 `json:"dropped"`
	}
	if err := c.do(http.MethodGet, "/api/v1/task-flows/runtime", nil, &flows, http.StatusOK); err != nil {
		return err
	}
	if flows.Queue.Name == "" || flows.Queue.Impact == "" || flows.Queue.NextAction == "" {
		return fmt.Errorf("task-flows/runtime missing actionable queue fields: %+v", flows.Queue)
	}
	if flows.Submitted == nil || flows.Enqueued == nil || flows.Dropped == nil {
		return fmt.Errorf("task-flows/runtime missing submit/drop counters: submitted=%v enqueued=%v dropped=%v", flows.Submitted, flows.Enqueued, flows.Dropped)
	}

	var notifications notificationDiagnostic
	if err := c.do(http.MethodGet, "/api/v1/runtime/notifications", nil, &notifications, http.StatusOK); err != nil {
		return err
	}
	if err := validateNotificationDiagnostics(notifications); err != nil {
		return err
	}

	var workers struct {
		Items []workerDiagnostic `json:"items"`
	}
	if err := c.do(http.MethodGet, "/api/v1/runtime/workers", nil, &workers, http.StatusOK); err != nil {
		return err
	}
	return validateWorkerDiagnostics(workers.Items)
}

func validateChannelDiagnostics(items []channelDiagnostic) error {
	if len(items) == 0 {
		return fmt.Errorf("runtime/channels/detail returned no items")
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Name == "" || item.Impact == "" || item.NextAction == "" || item.Dropped == nil {
			return fmt.Errorf("runtime/channels/detail missing actionable fields: %+v", item)
		}
		seen[item.Name] = true
	}
	for _, name := range expectedCoreChannels {
		if !seen[name] {
			return fmt.Errorf("runtime/channels/detail missing core channel %s", name)
		}
	}
	return nil
}

func validateChannelMap(source string, channels map[string]int) error {
	if len(channels) == 0 {
		return fmt.Errorf("%s has no channels", source)
	}
	for _, name := range expectedCoreChannels {
		if _, ok := channels[name]; !ok {
			return fmt.Errorf("%s missing core channel %s", source, name)
		}
	}
	return nil
}

func validateNotificationDiagnostics(item notificationDiagnostic) error {
	if item.PressureThreshold == 0 || item.Impact == "" || item.NextAction == "" || item.Published == nil || item.Delivered == nil || item.Dropped == nil {
		return fmt.Errorf("runtime/notifications missing actionable fields: %+v", item)
	}
	return nil
}

func validateWorkerDiagnostics(items []workerDiagnostic) error {
	if len(items) == 0 {
		return fmt.Errorf("runtime/workers returned no items")
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Name == "" || item.Health == "" || item.Impact == "" || item.NextAction == "" || item.Active == nil || item.Starts == nil || item.Exits == nil || item.Panics == nil {
			return fmt.Errorf("runtime/workers missing actionable fields: %+v", item)
		}
		seen[item.Name] = true
		if item.Health != "ok" {
			return fmt.Errorf("runtime worker %s unhealthy: health=%s panics=%d", item.Name, item.Health, *item.Panics)
		}
	}
	for _, name := range expectedCoreWorkers {
		if !seen[name] {
			return fmt.Errorf("runtime/workers missing core worker %s", name)
		}
	}
	for _, item := range items {
		if isExpectedCoreWorker(item.Name) && *item.Active <= 0 {
			return fmt.Errorf("runtime core worker %s not active: active=%d health=%s panics=%d", item.Name, *item.Active, item.Health, *item.Panics)
		}
	}
	return nil
}

func isExpectedCoreWorker(name string) bool {
	for _, expected := range expectedCoreWorkers {
		if name == expected {
			return true
		}
	}
	return false
}

func (c *client) do(method, path string, body any, out any, expectedStatus int) error {
	_, err := c.doAny(method, path, body, out, expectedStatus)
	return err
}

func (c *client) doAny(method, path string, body any, out any, expectedStatuses ...int) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if !statusAllowed(resp.StatusCode, expectedStatuses) {
		return resp.StatusCode, fmt.Errorf("%s %s returned %d, want %s: %s", method, path, resp.StatusCode, expectedStatusText(expectedStatuses), strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	if len(raw) == 0 {
		return resp.StatusCode, fmt.Errorf("%s %s returned empty body", method, path)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return resp.StatusCode, fmt.Errorf("decode %s %s: %w body=%s", method, path, err, strings.TrimSpace(string(raw)))
	}
	return resp.StatusCode, nil
}

func statusAllowed(status int, allowed []int) bool {
	for _, value := range allowed {
		if status == value {
			return true
		}
	}
	return false
}

func expectedStatusText(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return strings.Join(parts, " or ")
}
