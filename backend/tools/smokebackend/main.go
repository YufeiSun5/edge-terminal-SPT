//go:build smoke_tools

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type smokeStep struct {
	Name string
	Args []string
}

type smokeClient struct {
	base  string
	token string
	http  *http.Client
}

type runtimeDropSnapshot struct {
	Channels             map[string]runtimeChannelSnapshot
	TaskFlowDropped      uint64
	TaskFlowQueue        runtimeQueueSnapshot
	NotificationDropped  uint64
	NotificationPressure bool
	Workers              map[string]runtimeWorkerSnapshot
}

type runtimeChannelSnapshot struct {
	Len      int
	Cap      int
	Dropped  uint64
	Pressure bool
}

type runtimeQueueSnapshot struct {
	Len      int
	Cap      int
	Pressure bool
}

type runtimeWorkerSnapshot struct {
	Active int
	Starts uint64
	Exits  uint64
	Panics uint64
	Health string
}

type runtimeWorkerItem struct {
	Name   string  `json:"name"`
	Active *int    `json:"active"`
	Starts *uint64 `json:"starts"`
	Exits  *uint64 `json:"exits"`
	Panics *uint64 `json:"panics"`
	Health string  `json:"health"`
}

type runtimeChannelItem struct {
	Name     string  `json:"name"`
	Len      *int    `json:"len"`
	Cap      *int    `json:"cap"`
	Dropped  *uint64 `json:"dropped"`
	Pressure *bool   `json:"pressure"`
}

type runtimeQueueItem struct {
	Name     string `json:"name"`
	Len      *int   `json:"len"`
	Cap      *int   `json:"cap"`
	Pressure *bool  `json:"pressure"`
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
	latencyN := flag.Int("latency-n", 30, "latency smoke request count")
	latencyC := flag.Int("latency-c", 10, "latency smoke concurrency")
	latencyP95Max := flag.Duration("latency-p95-max", 2*time.Second, "p95 threshold passed to smokelatency; 0 disables")
	latencyMaxMax := flag.Duration("latency-max-max", 5*time.Second, "max threshold passed to smokelatency; 0 disables")
	timeout := flag.Duration("timeout", 90*time.Second, "timeout for each smoke step")
	skipBusiness := flag.Bool("skip-business", false, "only run health and latency checks")
	flag.Parse()

	if err := run(*base, *user, *pass, *latencyN, *latencyC, *latencyP95Max, *latencyMaxMax, *timeout, *skipBusiness); err != nil {
		fmt.Fprintln(os.Stderr, "backend smoke suite failed:", err)
		os.Exit(1)
	}
}

func run(base, user, pass string, latencyN, latencyC int, latencyP95Max time.Duration, latencyMaxMax time.Duration, timeout time.Duration, skipBusiness bool) error {
	client := &smokeClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 10 * time.Second}}
	if err := client.login(user, pass); err != nil {
		return fmt.Errorf("runtime diagnostics login failed: %w", err)
	}
	beforeDrops, err := client.runtimeDropSnapshot()
	if err != nil {
		return fmt.Errorf("runtime diagnostics baseline failed: %w", err)
	}

	steps := []smokeStep{
		{Name: "health", Args: []string{"run", "-tags", "smoke_tools", "./tools/smokehealth", "-base", base, "-user", user, "-pass", pass}},
	}
	if !skipBusiness {
		steps = append(steps,
			smokeStep{Name: "eb017 multi-project detection", Args: []string{"run", "-tags", "smoke_tools", "./tools/smokeeb017", "-base", base, "-user", user, "-pass", pass}},
			smokeStep{Name: "eb045 task-flow business chain", Args: []string{"run", "-tags", "smoke_tools", "./tools/smokeeb045", "-base", base, "-user", user, "-pass", pass}},
		)
	}
	steps = append(steps, smokeStep{
		Name: "concurrent latency",
		Args: []string{"run", "-tags", "smoke_tools", "./tools/smokelatency", "-base", base, "-user", user, "-pass", pass, "-n", fmt.Sprint(latencyN), "-c", fmt.Sprint(latencyC), "-p95-max", latencyP95Max.String(), "-max-max", latencyMaxMax.String()},
	})

	started := time.Now()
	for _, step := range steps {
		if err := runStep(step, timeout); err != nil {
			return err
		}
	}
	afterDrops, err := client.runtimeDropSnapshot()
	if err != nil {
		return fmt.Errorf("runtime diagnostics final snapshot failed: %w", err)
	}
	if err := ensureNoNewRuntimeDrops(beforeDrops, afterDrops); err != nil {
		return err
	}
	fmt.Printf("runtime diagnostics ok: channels=%v task_flow_dropped=%d notification_dropped=%d workers=%d\n", afterDrops.Channels, afterDrops.TaskFlowDropped, afterDrops.NotificationDropped, len(afterDrops.Workers))
	fmt.Printf("backend smoke suite passed: steps=%d elapsed=%s\n", len(steps), time.Since(started).Round(time.Millisecond))
	return nil
}

func runStep(step smokeStep, timeout time.Duration) error {
	fmt.Printf("==> %s\n", step.Name)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", step.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.Dir = "."
	started := time.Now()
	err := cmd.Run()
	elapsed := time.Since(started).Round(time.Millisecond)
	if ctx.Err() != nil {
		return fmt.Errorf("%s timed out after %s", step.Name, timeout)
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("%s failed after %s: %s", step.Name, elapsed, strings.TrimSpace(exitErr.Error()))
		}
		return fmt.Errorf("%s failed after %s: %w", step.Name, elapsed, err)
	}
	fmt.Printf("<== %s ok elapsed=%s\n", step.Name, elapsed)
	return nil
}

func (c *smokeClient) login(username, password string) error {
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]any{"username": username, "password": password}, &out); err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("login response has empty access_token")
	}
	c.token = out.AccessToken
	return nil
}

func (c *smokeClient) runtimeDropSnapshot() (runtimeDropSnapshot, error) {
	var channels struct {
		Items []runtimeChannelItem `json:"items"`
	}
	if err := c.do(http.MethodGet, "/api/v1/runtime/channels/detail", nil, &channels); err != nil {
		return runtimeDropSnapshot{}, err
	}
	channelDrops, err := validateRuntimeChannelItems(channels.Items)
	if err != nil {
		return runtimeDropSnapshot{}, err
	}
	out := runtimeDropSnapshot{Channels: channelDrops}

	var flows struct {
		Queue   runtimeQueueItem `json:"queue"`
		Dropped *uint64          `json:"dropped"`
	}
	if err := c.do(http.MethodGet, "/api/v1/task-flows/runtime", nil, &flows); err != nil {
		return runtimeDropSnapshot{}, err
	}
	taskFlowQueue, err := validateRuntimeQueueItem("task-flows/runtime queue", flows.Queue, "task_flow")
	if err != nil {
		return runtimeDropSnapshot{}, err
	}
	out.TaskFlowQueue = taskFlowQueue
	taskFlowDropped, err := requireDroppedCounter("task-flows/runtime", flows.Dropped)
	if err != nil {
		return runtimeDropSnapshot{}, err
	}
	out.TaskFlowDropped = taskFlowDropped

	var notifications struct {
		Dropped  *uint64 `json:"dropped"`
		Pressure *bool   `json:"pressure"`
	}
	if err := c.do(http.MethodGet, "/api/v1/runtime/notifications", nil, &notifications); err != nil {
		return runtimeDropSnapshot{}, err
	}
	if notifications.Pressure == nil {
		return runtimeDropSnapshot{}, fmt.Errorf("runtime/notifications missing pressure field")
	}
	if *notifications.Pressure {
		return runtimeDropSnapshot{}, fmt.Errorf("runtime/notifications under pressure at snapshot")
	}
	out.NotificationPressure = *notifications.Pressure
	notificationDropped, err := requireDroppedCounter("runtime/notifications", notifications.Dropped)
	if err != nil {
		return runtimeDropSnapshot{}, err
	}
	out.NotificationDropped = notificationDropped

	var workers struct {
		Items []runtimeWorkerItem `json:"items"`
	}
	if err := c.do(http.MethodGet, "/api/v1/runtime/workers", nil, &workers); err != nil {
		return runtimeDropSnapshot{}, err
	}
	workerSnapshots, err := validateRuntimeWorkerItems(workers.Items)
	if err != nil {
		return runtimeDropSnapshot{}, err
	}
	out.Workers = workerSnapshots
	return out, nil
}

func validateRuntimeChannelItems(items []runtimeChannelItem) (map[string]runtimeChannelSnapshot, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("runtime/channels/detail returned no items")
	}
	out := make(map[string]runtimeChannelSnapshot, len(items))
	for _, item := range items {
		if item.Name == "" {
			return nil, fmt.Errorf("runtime/channels/detail returned empty channel name")
		}
		if _, exists := out[item.Name]; exists {
			return nil, fmt.Errorf("runtime/channels/detail returned duplicate channel name %s", item.Name)
		}
		dropped, err := requireDroppedCounter("runtime/channels/detail "+item.Name, item.Dropped)
		if err != nil {
			return nil, err
		}
		queue, err := validateRuntimeQueueItem("runtime/channels/detail "+item.Name, runtimeQueueItem{
			Name:     item.Name,
			Len:      item.Len,
			Cap:      item.Cap,
			Pressure: item.Pressure,
		}, item.Name)
		if err != nil {
			return nil, err
		}
		out[item.Name] = runtimeChannelSnapshot{Len: queue.Len, Cap: queue.Cap, Dropped: dropped, Pressure: queue.Pressure}
	}
	if err := requireExpectedNames("runtime/channels/detail", out, expectedCoreChannels); err != nil {
		return nil, err
	}
	return out, nil
}

func validateRuntimeQueueItem(source string, item runtimeQueueItem, expectedName string) (runtimeQueueSnapshot, error) {
	if expectedName != "" && item.Name != expectedName {
		return runtimeQueueSnapshot{}, fmt.Errorf("%s name=%q want %q", source, item.Name, expectedName)
	}
	if item.Len == nil || item.Cap == nil || item.Pressure == nil {
		return runtimeQueueSnapshot{}, fmt.Errorf("%s missing queue pressure diagnostics", source)
	}
	if *item.Len < 0 || *item.Cap < 0 {
		return runtimeQueueSnapshot{}, fmt.Errorf("%s has invalid queue size len=%d cap=%d", source, *item.Len, *item.Cap)
	}
	if *item.Cap > 0 && *item.Len > *item.Cap {
		return runtimeQueueSnapshot{}, fmt.Errorf("%s has len greater than cap: len=%d cap=%d", source, *item.Len, *item.Cap)
	}
	if *item.Pressure {
		return runtimeQueueSnapshot{}, fmt.Errorf("%s under pressure: len=%d cap=%d", source, *item.Len, *item.Cap)
	}
	return runtimeQueueSnapshot{Len: *item.Len, Cap: *item.Cap, Pressure: *item.Pressure}, nil
}

func validateRuntimeWorkerItems(items []runtimeWorkerItem) (map[string]runtimeWorkerSnapshot, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("runtime/workers returned no items")
	}
	out := make(map[string]runtimeWorkerSnapshot, len(items))
	for _, item := range items {
		if item.Name == "" {
			return nil, fmt.Errorf("runtime/workers returned empty worker name")
		}
		if _, exists := out[item.Name]; exists {
			return nil, fmt.Errorf("runtime/workers returned duplicate worker name %s", item.Name)
		}
		if item.Active == nil || item.Starts == nil || item.Exits == nil || item.Panics == nil || item.Health == "" {
			return nil, fmt.Errorf("runtime/workers missing lifecycle diagnostics for worker %s", item.Name)
		}
		if item.Health != "ok" {
			return nil, fmt.Errorf("runtime worker %s unhealthy at snapshot: health=%s panics=%d", item.Name, item.Health, *item.Panics)
		}
		if isExpectedCoreWorker(item.Name) && *item.Active <= 0 {
			return nil, fmt.Errorf("runtime core worker %s not active at snapshot: active=%d health=%s panics=%d", item.Name, *item.Active, item.Health, *item.Panics)
		}
		out[item.Name] = runtimeWorkerSnapshot{Active: *item.Active, Starts: *item.Starts, Exits: *item.Exits, Panics: *item.Panics, Health: item.Health}
	}
	if err := requireExpectedNames("runtime/workers", out, expectedCoreWorkers); err != nil {
		return nil, err
	}
	return out, nil
}

func requireDroppedCounter(source string, counter *uint64) (uint64, error) {
	if counter == nil {
		return 0, fmt.Errorf("%s missing dropped counter", source)
	}
	return *counter, nil
}

func requireExpectedNames[T any](source string, values map[string]T, expected []string) error {
	for _, name := range expected {
		if _, ok := values[name]; !ok {
			return fmt.Errorf("%s missing core item %s", source, name)
		}
	}
	return nil
}

func ensureNoNewRuntimeDrops(before runtimeDropSnapshot, after runtimeDropSnapshot) error {
	for name, afterChannel := range after.Channels {
		beforeChannel := before.Channels[name]
		if afterChannel.Pressure {
			return fmt.Errorf("runtime channel %s under pressure after smoke: len=%d cap=%d", name, afterChannel.Len, afterChannel.Cap)
		}
		if afterChannel.Dropped > beforeChannel.Dropped {
			return fmt.Errorf("runtime channel %s dropped messages during smoke: before=%d after=%d", name, beforeChannel.Dropped, afterChannel.Dropped)
		}
	}
	if after.TaskFlowQueue.Pressure {
		return fmt.Errorf("task-flow queue under pressure after smoke: len=%d cap=%d", after.TaskFlowQueue.Len, after.TaskFlowQueue.Cap)
	}
	if after.TaskFlowDropped > before.TaskFlowDropped {
		return fmt.Errorf("task-flow queue dropped jobs during smoke: before=%d after=%d", before.TaskFlowDropped, after.TaskFlowDropped)
	}
	if after.NotificationPressure {
		return fmt.Errorf("notification hub under pressure after smoke")
	}
	if after.NotificationDropped > before.NotificationDropped {
		return fmt.Errorf("notification hub dropped online deliveries during smoke: before=%d after=%d", before.NotificationDropped, after.NotificationDropped)
	}
	for name, worker := range after.Workers {
		if isExpectedCoreWorker(name) && worker.Active <= 0 {
			return fmt.Errorf("runtime core worker %s not active after smoke: active=%d health=%s panics=%d", name, worker.Active, worker.Health, worker.Panics)
		}
		if worker.Panics > before.Workers[name].Panics {
			return fmt.Errorf("runtime worker %s panicked during smoke: before=%d after=%d", name, before.Workers[name].Panics, worker.Panics)
		}
		if worker.Health != "ok" {
			return fmt.Errorf("runtime worker %s unhealthy after smoke: health=%s panics=%d", name, worker.Health, worker.Panics)
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

func (c *smokeClient) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
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
	if out == nil {
		return nil
	}
	if len(raw) == 0 {
		return fmt.Errorf("%s %s returned empty body", method, path)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s: %w body=%s", method, path, err, strings.TrimSpace(string(raw)))
	}
	return nil
}
