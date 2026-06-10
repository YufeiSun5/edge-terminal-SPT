package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"spindle-main-server/backend/internal/config"
	"spindle-main-server/backend/internal/edgecontrol"
	"spindle-main-server/backend/internal/query"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type edgeRegistry struct {
	defaultEdgeInstanceID string
	nodes                 map[string]*edgecontrol.Client
	configs               map[string]config.EdgeConfig
	order                 []string
}

func newEdgeRegistry(cfg *config.Config) *edgeRegistry {
	registry := &edgeRegistry{
		defaultEdgeInstanceID: strings.TrimSpace(cfg.Edge.EdgeInstanceID),
		nodes:                 map[string]*edgecontrol.Client{},
		configs:               map[string]config.EdgeConfig{},
	}
	for _, edge := range cfg.Edges {
		edgeID := strings.TrimSpace(edge.EdgeInstanceID)
		if edgeID == "" {
			continue
		}
		registry.order = append(registry.order, edgeID)
		registry.configs[edgeID] = edge
		registry.nodes[edgeID] = edgecontrol.NewClient(edgecontrol.Options{
			BaseURL:         edge.BaseURL,
			ServiceTokenRef: edge.ServiceTokenRef,
			Enabled:         edge.IsEnabled(),
			Timeout:         10 * time.Second,
		})
	}
	if registry.defaultEdgeInstanceID == "" && len(registry.order) > 0 {
		registry.defaultEdgeInstanceID = registry.order[0]
	}
	return registry
}

func (r *edgeRegistry) Client(edgeInstanceID string) (*edgecontrol.Client, bool) {
	client, ok := r.nodes[strings.TrimSpace(edgeInstanceID)]
	return client, ok
}

func (r *edgeRegistry) StatusNodes(syncDatabase string) []gin.H {
	nodes := make([]gin.H, 0, len(r.order))
	for _, edgeID := range r.order {
		cfg := r.configs[edgeID]
		nodes = append(nodes, gin.H{
			"edge_instance_id":  edgeID,
			"base_url":          cfg.BaseURL,
			"service_token_ref": cfg.ServiceTokenRef,
			"enabled":           cfg.IsEnabled(),
			"sync_database":     syncDatabase,
		})
	}
	return nodes
}

func resolveRealtimeEdgeClient(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery) (*edgecontrol.Client, string, bool) {
	edgeInstanceID, ok := resolveRealtimeEdgeInstanceID(c, registry, stationViewQuery)
	if !ok {
		return nil, "", false
	}
	client, exists := registry.Client(edgeInstanceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error":            "edge instance is not available on this main-server bridge",
			"code":             "edge_instance_not_found",
			"edge_instance_id": edgeInstanceID,
		})
		return nil, "", false
	}
	return client, edgeInstanceID, true
}

func resolveRealtimeEdgeInstanceID(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery) (string, bool) {
	requestedEdgeID := strings.TrimSpace(c.Query("edge_instance_id"))
	projectID, projectIDSet, err := optionalProjectID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id", "code": "invalid_project_id"})
		return "", false
	}
	if projectIDSet {
		projectEdgeID, err := stationViewQuery.ProjectEdgeInstanceID(uint(projectID), requestedEdgeID)
		if err != nil {
			if errors.Is(err, query.ErrEdgeInstanceMismatch) {
				c.JSON(http.StatusNotFound, gin.H{
					"error":            "project does not belong to requested edge instance",
					"code":             "project_edge_instance_mismatch",
					"project_id":       projectID,
					"edge_instance_id": requestedEdgeID,
				})
				return "", false
			}
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fallbackEdgeInstanceID(registry, requestedEdgeID, c)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "project edge lookup failed", "code": "internal_error"})
			return "", false
		}
		if projectEdgeID != "" {
			return projectEdgeID, true
		}
	}
	if requestedEdgeID != "" {
		return requestedEdgeID, true
	}
	if registry.defaultEdgeInstanceID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "edge instance id is not configured",
			"code":        "edge_instance_id_missing",
			"next_action": "configure main-server edge.edge_instance_id or edges[] before opening realtime bridge",
		})
		return "", false
	}
	return registry.defaultEdgeInstanceID, true
}

func resolveStationViewEdgeInstanceID(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, projectID uint) (string, bool) {
	requestedEdgeID := strings.TrimSpace(c.Query("edge_instance_id"))
	projectEdgeID, err := stationViewQuery.ProjectEdgeInstanceID(projectID, requestedEdgeID)
	if err != nil {
		if errors.Is(err, query.ErrEdgeInstanceMismatch) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":            "station view project does not belong to requested edge instance",
				"code":             "station_view_edge_instance_mismatch",
				"project_id":       projectID,
				"edge_instance_id": requestedEdgeID,
			})
			return "", false
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":      "station view project not found",
				"code":       "station_view_project_not_found",
				"project_id": projectID,
			})
			return "", false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "station view edge lookup failed", "code": "internal_error"})
		return "", false
	}
	edgeInstanceID := strings.TrimSpace(firstNonEmpty(projectEdgeID, requestedEdgeID))
	if edgeInstanceID == "" {
		code := "station_view_edge_instance_unresolved"
		message := "station view project edge_instance_id is not set"
		if len(registry.order) > 1 {
			code = "station_view_edge_instance_ambiguous"
			message = "station view project edge_instance_id is not set and multiple edge nodes are configured"
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":       message,
			"code":        code,
			"project_id":  projectID,
			"next_action": "set sys_projects.edge_instance_id for this project or pass edge_instance_id explicitly",
		})
		return "", false
	}
	if _, exists := registry.Client(edgeInstanceID); !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error":            "edge instance is not available on this main-server bridge",
			"code":             "edge_instance_not_found",
			"edge_instance_id": edgeInstanceID,
		})
		return "", false
	}
	return edgeInstanceID, true
}

func fallbackEdgeInstanceID(registry *edgeRegistry, requestedEdgeID string, c *gin.Context) (string, bool) {
	if requestedEdgeID != "" {
		return requestedEdgeID, true
	}
	if registry.defaultEdgeInstanceID != "" {
		return registry.defaultEdgeInstanceID, true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "edge instance id is not configured", "code": "edge_instance_id_missing"})
	return "", false
}

func optionalProjectID(c *gin.Context) (uint64, bool, error) {
	raw := strings.TrimSpace(c.Query("project_id"))
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, true, strconv.ErrSyntax
	}
	return value, true, nil
}

func edgeForwardRawQuery(values url.Values) string {
	cleaned := url.Values{}
	for key, list := range values {
		if key == "access_token" || key == "edge_instance_id" {
			continue
		}
		for _, value := range list {
			cleaned.Add(key, value)
		}
	}
	return cleaned.Encode()
}

func resolveControlEdgeClient(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, body []byte) (*edgecontrol.Client, string, bool) {
	edgeInstanceID, ok := resolveControlEdgeInstanceID(c, registry, stationViewQuery, body)
	if !ok {
		return nil, "", false
	}
	client, exists := registry.Client(edgeInstanceID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error":            "edge instance is not available on this main-server bridge",
			"code":             "edge_instance_not_found",
			"edge_instance_id": edgeInstanceID,
		})
		return nil, "", false
	}
	return client, edgeInstanceID, true
}

func resolveControlEdgeInstanceID(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, body []byte) (string, bool) {
	requestedEdgeID := controlRequestedEdgeID(c)
	target := controlTargetFromJSON(body)
	edgeInstanceID, err := resolveControlEdgeInstanceIDFromTarget(registry, stationViewQuery, requestedEdgeID, target)
	if err != nil {
		writeControlEdgeResolveError(c, err)
		return "", false
	}
	return edgeInstanceID, true
}

func controlRequestedEdgeID(c *gin.Context) string {
	return strings.TrimSpace(firstNonEmpty(c.Query("edge_instance_id"), c.GetHeader("X-Edge-Instance-ID")))
}

type controlTarget struct {
	ProjectID   uint64
	TaskID      uint64
	VarID       int64
	ProjectCode string
	VarName     string
}

func controlTargetFromJSON(body []byte) controlTarget {
	root := jsonObject(body)
	target := controlTargetFromMap(root)
	if nested, ok := root["payload"]; ok {
		if nestedTarget := controlTargetFromMap(objectFromAny(nested)); nestedTarget.ProjectID > 0 || nestedTarget.TaskID > 0 || nestedTarget.VarID != 0 || nestedTarget.ProjectCode != "" || nestedTarget.VarName != "" {
			target = mergeControlTargets(target, nestedTarget)
		}
	}
	return target
}

func controlTargetFromMap(payload map[string]any) controlTarget {
	return controlTarget{
		ProjectID:   uint64FromAny(payload["project_id"]),
		TaskID:      uint64FromAny(payload["task_id"]),
		VarID:       int64FromAny(payload["var_id"]),
		ProjectCode: stringFromAny(payload["project_code"]),
		VarName:     stringFromAny(payload["var_name"]),
	}
}

func mergeControlTargets(base controlTarget, override controlTarget) controlTarget {
	if override.ProjectID > 0 {
		base.ProjectID = override.ProjectID
	}
	if override.TaskID > 0 {
		base.TaskID = override.TaskID
	}
	if override.VarID != 0 {
		base.VarID = override.VarID
	}
	if override.ProjectCode != "" {
		base.ProjectCode = override.ProjectCode
	}
	if override.VarName != "" {
		base.VarName = override.VarName
	}
	return base
}

func jsonObject(body []byte) map[string]any {
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]any{}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func objectFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case json.RawMessage:
		return jsonObject(typed)
	}
	return map[string]any{}
}

type controlEdgeResolveError struct {
	Status         int
	Code           string
	Message        string
	EdgeInstanceID string
	TargetID       string
}

func (e controlEdgeResolveError) Error() string {
	return e.Message
}

func resolveControlEdgeInstanceIDFromTarget(registry *edgeRegistry, stationViewQuery *query.StationViewQuery, requestedEdgeID string, target controlTarget) (string, error) {
	requestedEdgeID = strings.TrimSpace(requestedEdgeID)
	if target.ProjectID > 0 {
		return resolveControlProjectEdge(registry, stationViewQuery, uint(target.ProjectID), requestedEdgeID, strconv.FormatUint(target.ProjectID, 10))
	}
	if target.TaskID > 0 {
		edgeID, err := stationViewQuery.DetectionRunEdgeInstanceID(uint(target.TaskID), requestedEdgeID)
		return edgeIDFromLookup(registry, edgeID, err, "task_id", strconv.FormatUint(target.TaskID, 10), requestedEdgeID)
	}
	if target.VarID != 0 {
		edgeID, err := stationViewQuery.VariableEdgeInstanceID(target.VarID, requestedEdgeID)
		return edgeIDFromLookup(registry, edgeID, err, "var_id", strconv.FormatInt(target.VarID, 10), requestedEdgeID)
	}
	if target.ProjectCode != "" {
		edgeID, err := stationViewQuery.ProjectCodeEdgeInstanceID(target.ProjectCode, requestedEdgeID)
		return edgeIDFromLookup(registry, edgeID, err, "project_code", target.ProjectCode, requestedEdgeID)
	}
	if requestedEdgeID != "" {
		return requestedEdgeID, nil
	}
	if edgeID, ok := singleConfiguredEdgeID(registry); ok {
		return edgeID, nil
	}
	return "", controlEdgeResolveError{
		Status:  http.StatusConflict,
		Code:    "control_edge_instance_unresolved",
		Message: "control target edge_instance_id cannot be resolved from payload",
	}
}

func resolveControlProjectEdge(registry *edgeRegistry, stationViewQuery *query.StationViewQuery, projectID uint, requestedEdgeID string, targetID string) (string, error) {
	edgeID, err := stationViewQuery.ProjectEdgeInstanceID(projectID, requestedEdgeID)
	return edgeIDFromLookup(registry, edgeID, err, "project_id", targetID, requestedEdgeID)
}

func edgeIDFromLookup(registry *edgeRegistry, edgeID string, err error, targetType string, targetID string, requestedEdgeID string) (string, error) {
	if err == nil && strings.TrimSpace(edgeID) != "" {
		return strings.TrimSpace(edgeID), nil
	}
	if err == nil && strings.TrimSpace(requestedEdgeID) != "" {
		return strings.TrimSpace(requestedEdgeID), nil
	}
	if errors.Is(err, query.ErrEdgeInstanceMismatch) {
		return "", controlEdgeResolveError{
			Status:         http.StatusNotFound,
			Code:           "control_edge_instance_mismatch",
			Message:        "control target does not belong to requested edge instance",
			EdgeInstanceID: requestedEdgeID,
			TargetID:       targetID,
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && strings.TrimSpace(edgeID) == "") {
		if requestedEdgeID == "" {
			if edgeID, ok := singleConfiguredEdgeID(registry); ok {
				return edgeID, nil
			}
		}
		return "", controlEdgeResolveError{
			Status:   http.StatusNotFound,
			Code:     "control_target_not_found",
			Message:  "control target cannot be resolved",
			TargetID: targetType + ":" + targetID,
		}
	}
	return "", controlEdgeResolveError{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "control edge lookup failed",
	}
}

func singleConfiguredEdgeID(registry *edgeRegistry) (string, bool) {
	if len(registry.order) == 1 && registry.defaultEdgeInstanceID != "" {
		return registry.defaultEdgeInstanceID, true
	}
	if registry.defaultEdgeInstanceID != "" && len(registry.order) <= 1 {
		return registry.defaultEdgeInstanceID, true
	}
	return "", false
}

func writeControlEdgeResolveError(c *gin.Context, err error) {
	var typed controlEdgeResolveError
	if errors.As(err, &typed) {
		body := gin.H{
			"error": typed.Message,
			"code":  typed.Code,
		}
		if typed.TargetID != "" {
			body["target_id"] = typed.TargetID
		}
		if typed.EdgeInstanceID != "" {
			body["edge_instance_id"] = typed.EdgeInstanceID
		}
		if typed.Code == "control_edge_instance_unresolved" {
			body["next_action"] = "include project_id, task_id, var_id, project_code, or edge_instance_id in the control payload"
		}
		c.JSON(typed.Status, body)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "control edge lookup failed", "code": "internal_error"})
}

func uint64FromAny(value any) uint64 {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return uint64(typed)
		}
	case json.Number:
		parsed, err := strconv.ParseUint(strings.TrimSpace(string(typed)), 10, 64)
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case float64:
		if typed != 0 {
			return int64(typed)
		}
	case json.Number:
		parsed, err := strconv.ParseInt(strings.TrimSpace(string(typed)), 10, 64)
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func stringFromAny(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}
