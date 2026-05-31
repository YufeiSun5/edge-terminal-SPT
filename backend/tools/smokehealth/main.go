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
	}

	checks := []namedCheck{
		{Name: "auth/me", Path: "/api/v1/auth/me"},
		{Name: "variables", Path: "/api/v1/variables"},
		{Name: "runtime/channels", Path: "/api/v1/runtime/channels"},
		{Name: "gateway-configs", Path: "/api/v1/gateway-configs"},
		{Name: "task-modules", Path: "/api/v1/task-modules"},
		{Name: "task-flow-templates", Path: "/api/v1/task-flow-templates"},
		{Name: "task-flow-runs", Path: "/api/v1/task-flow-runs?limit=1"},
		{Name: "notifications/unread-count", Path: "/api/v1/notifications/unread-count"},
		{Name: "detection-runs/active", Path: "/api/v1/detection-runs/active"},
	}
	for _, check := range checks {
		if err := c.expectJSON(http.MethodGet, check.Path, http.StatusOK); err != nil {
			return fmt.Errorf("%s check failed: %w", check.Name, err)
		}
		fmt.Printf("%s ok\n", check.Name)
	}

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
	if strings.ToLower(out.Status) != "ok" {
		return healthResponse{}, fmt.Errorf("unexpected health status %q", out.Status)
	}
	return out, nil
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
	if err := c.do(method, path, nil, &raw, expectedStatus); err != nil {
		return err
	}
	if !json.Valid(raw) {
		return fmt.Errorf("response is not valid json")
	}
	return nil
}

func (c *client) expectStatus(method, path string, expectedStatus int) error {
	return c.do(method, path, nil, nil, expectedStatus)
}

func (c *client) do(method, path string, body any, out any, expectedStatus int) error {
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
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("%s %s returned %d, want %d: %s", method, path, resp.StatusCode, expectedStatus, strings.TrimSpace(string(raw)))
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
