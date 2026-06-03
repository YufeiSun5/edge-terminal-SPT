package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"spindle-main-server/backend/internal/auth"
	"spindle-main-server/backend/internal/config"
	"spindle-main-server/backend/internal/edgecontrol"
	"spindle-main-server/backend/internal/query"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(cfg *config.Config, db *gorm.DB) http.Handler {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors())
	stationViewQuery := query.NewStationViewQuery(db)
	authService := auth.NewService(
		auth.NewUserStore(db),
		auth.NewJWTManager(cfg.Auth.JWTSecret, time.Duration(cfg.Auth.AccessTokenTTLSeconds)*time.Second),
		auth.Options{
			EdgeBaseURL:         cfg.Edge.BaseURL,
			EdgeServiceTokenRef: cfg.Edge.ServiceTokenRef,
			Getenv:              os.Getenv,
		},
	)
	edgeControlClient := edgecontrol.NewClient(edgecontrol.Options{
		BaseURL:         cfg.Edge.BaseURL,
		ServiceTokenRef: cfg.Edge.ServiceTokenRef,
		Enabled:         cfg.Edge.IsEnabled(),
		Timeout:         10 * time.Second,
	})

	router.GET("/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		dbOK := err == nil && sqlDB.Ping() == nil
		c.JSON(http.StatusOK, gin.H{
			"status":           statusText(dbOK),
			"role":             "main_server",
			"database_ok":      dbOK,
			"edge_base_url":    cfg.Edge.BaseURL,
			"edge_instance_id": cfg.Edge.EdgeInstanceID,
			"time":             time.Now().Format(time.RFC3339Nano),
		})
	})

	router.POST("/api/v1/auth/login", authService.Login)
	router.POST("/api/v1/auth/sso-ticket/verify", authService.VerifySSOTicket)
	protected := router.Group("/api/v1")
	protected.Use(authService.RequireUser())
	protected.GET("/auth/me", authService.Me)
	protected.POST("/auth/logout", authService.Logout)
	protected.POST("/auth/sso-ticket", authService.RequirePermission(auth.PermSSOHandoff), authService.CreateSSOTicket)
	protected.GET("/users", authService.RequirePermission(auth.PermManageUsers), authService.ListUsers)
	protected.GET("/gateways", authService.RequirePermission(auth.PermViewRealtime), mainServerRuntimeDiagnosticUnsupported)
	protected.GET("/gateway-configs", authService.RequirePermission(auth.PermManageGateways), func(c *gin.Context) {
		gateways, err := stationViewQuery.ListGatewayConfigs()
		if err != nil {
			writeSyncedReadError(c, err, "gateway configs query failed")
			return
		}
		c.JSON(http.StatusOK, gateways)
	})
	protected.GET("/gateway-configs/:gateway_id", authService.RequirePermission(auth.PermManageGateways), func(c *gin.Context) {
		gatewayID, err := parseUintParam(c, "gateway_id")
		if err != nil || gatewayID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway id", "code": "invalid_gateway_id"})
			return
		}
		gateway, err := stationViewQuery.GetGatewayConfig(int(gatewayID))
		if err != nil {
			writeSyncedReadError(c, err, "gateway config query failed")
			return
		}
		c.JSON(http.StatusOK, gateway)
	})
	protected.GET("/projects/:id/members", authService.RequirePermission(auth.PermManageUsers), func(c *gin.Context) {
		projectID, err := parseUintParam(c, "id")
		if err != nil || projectID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id", "code": "invalid_project_id"})
			return
		}
		members, err := stationViewQuery.ListProjectMembers(uint(projectID), edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "project members query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": members, "count": len(members)})
	})
	protected.GET("/system/database-config", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		c.JSON(http.StatusOK, mainServerDatabaseConfigView(cfg.Database))
	})
	protected.PATCH("/system/database-config", authService.RequirePermission(auth.PermSystemSettings), mainServerDatabaseConfigUnsupported)
	protected.POST("/system/database-config/test", authService.RequirePermission(auth.PermSystemSettings), mainServerDatabaseConfigUnsupported)
	protected.GET("/runtime/channels", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edgeControlClient, "api/v1/edge-control/runtime/channels")
	})
	protected.GET("/runtime/channels/detail", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edgeControlClient, "api/v1/edge-control/runtime/channels/detail")
	})
	protected.GET("/runtime/notifications", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edgeControlClient, "api/v1/edge-control/runtime/notifications")
	})
	protected.GET("/runtime/workers", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edgeControlClient, "api/v1/edge-control/runtime/workers")
	})
	protected.GET("/task-modules", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeMetadataRead(c, edgeControlClient, "api/v1/edge-control/task-modules")
	})
	protected.GET("/task-flow-templates", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeMetadataRead(c, edgeControlClient, "api/v1/edge-control/task-flow-templates")
	})
	protected.GET("/ws", authService.RequirePermission(auth.PermViewRealtime), mainServerRealtimeWSUnsupported)
	protected.GET("/main-server/sync-diagnostics", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		diagnostics := stationViewQuery.SyncDiagnostics()
		c.JSON(http.StatusOK, gin.H{
			"role":             "main_server",
			"edge_instance_id": cfg.Edge.EdgeInstanceID,
			"sync_database":    cfg.Database.Name,
			"diagnostics":      diagnostics,
		})
	})
	protected.GET("/main-server/report-readiness", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		taskID, err := parseUintQuery(c, "task_id")
		if err != nil || taskID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "task_id is required",
				"code":  "invalid_task_id",
			})
			return
		}
		readiness, err := stationViewQuery.ReportReadiness(uint(taskID), edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "report readiness query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"role":             "main_server",
			"edge_instance_id": cfg.Edge.EdgeInstanceID,
			"sync_database":    cfg.Database.Name,
			"readiness":        readiness,
		})
	})
	protected.GET("/realtime/variables", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		resp, err := edgeControlClient.ForwardRead(c.Request.Context(), "api/v1/edge-control/realtime/variables", c.Request.URL.RawQuery)
		if err != nil {
			writeEdgeRealtimeForwardError(c, err, edgeControlClient.ServiceTokenRef())
			return
		}
		contentType := resp.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		c.Data(resp.StatusCode, contentType, resp.Body)
	})
	protected.GET("/report-templates", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseReportTemplateFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		templates, err := stationViewQuery.ListReportTemplates(filter)
		if err != nil {
			writeSyncedReadError(c, err, "report templates query failed")
			return
		}
		c.JSON(http.StatusOK, templates)
	})
	protected.GET("/storage-routes", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseStorageRouteFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		routes, err := stationViewQuery.ListStorageRoutes(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "storage routes query failed")
			return
		}
		c.JSON(http.StatusOK, routes)
	})
	protected.GET("/task-flows", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		filter, err := parseTaskFlowFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		flows, err := stationViewQuery.ListTaskFlows(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "task flows query failed")
			return
		}
		c.JSON(http.StatusOK, flows)
	})
	protected.GET("/task-flows/runtime", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edgeControlClient, "api/v1/edge-control/task-flows/runtime")
	})
	protected.GET("/task-flows/:id", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		flowID, err := parseUintParam(c, "id")
		if err != nil || flowID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task flow id", "code": "invalid_task_flow_id"})
			return
		}
		flow, err := stationViewQuery.GetTaskFlow(flowID, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "task flow query failed")
			return
		}
		c.JSON(http.StatusOK, flow)
	})
	protected.GET("/detection-standards", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseDetectionStandardFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		standards, err := stationViewQuery.ListDetectionStandards(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "detection standards query failed")
			return
		}
		c.JSON(http.StatusOK, standards)
	})
	protected.GET("/detection-standards/favorites", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok || principal.AuthType != "user" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required", "code": "unauthorized"})
			return
		}
		standards, err := stationViewQuery.ListFavoriteDetectionStandards(principal.UserID, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "favorite detection standards query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": standards, "count": len(standards)})
	})
	protected.GET("/detection-standards/recent", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok || principal.AuthType != "user" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required", "code": "unauthorized"})
			return
		}
		projectID, err := parseOptionalUintPointer(c, "project_id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		limit, err := parseOptionalPositiveInt(c, "limit", 20)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "invalid_limit"})
			return
		}
		standards, normalizedLimit, err := stationViewQuery.ListRecentDetectionStandards(principal.UserID, projectID, limit, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "recent detection standards query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": standards, "count": len(standards), "limit": normalizedLimit})
	})
	protected.GET("/detection-standards/:id", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		standardID, err := parseUintParam(c, "id")
		if err != nil || standardID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid standard id", "code": "invalid_standard_id"})
			return
		}
		standard, err := stationViewQuery.GetDetectionStandard(uint(standardID), edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "detection standard query failed")
			return
		}
		c.JSON(http.StatusOK, standard)
	})
	protected.GET("/audit-logs", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		filter, err := parseAuditLogFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		items, total, limit, offset, err := stationViewQuery.ListAuditLogs(filter)
		if err != nil {
			writeSyncedReadError(c, err, "audit logs query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
	})
	protected.GET("/notifications", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok || principal.AuthType != "user" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required", "code": "unauthorized"})
			return
		}
		filter, err := parseNotificationFilter(c, principal.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		items, total, limit, offset, err := stationViewQuery.ListUserNotifications(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "notifications query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
	})
	protected.GET("/notifications/unread-count", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok || principal.AuthType != "user" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required", "code": "unauthorized"})
			return
		}
		filter, err := parseNotificationFilter(c, principal.UserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		count, err := stationViewQuery.CountUnreadNotifications(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "notification unread count query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"unread": count})
	})
	protected.POST("/notifications/:id/read", authService.RequirePermission(auth.PermViewRealtime), mainServerNotificationReadUnsupported)
	protected.POST("/notifications/read-all", authService.RequirePermission(auth.PermViewRealtime), mainServerNotificationReadUnsupported)

	router.GET("/api/v1/main-server/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"role":                "main_server",
			"query_source":        "local_mysql",
			"edge_control_target": cfg.Edge.BaseURL,
			"query_proxy_enabled": false,
			"edge_nodes": []gin.H{{
				"edge_instance_id":  cfg.Edge.EdgeInstanceID,
				"base_url":          cfg.Edge.BaseURL,
				"service_token_ref": cfg.Edge.ServiceTokenRef,
				"enabled":           cfg.Edge.IsEnabled(),
				"sync_database":     cfg.Database.Name,
			}},
		})
	})

	protected.GET("/projects", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		edgeInstanceID := edgeContext(c, cfg)
		projects, err := stationViewQuery.ListProjects(edgeInstanceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "projects query failed",
				"code":  "internal_error",
			})
			return
		}
		c.JSON(http.StatusOK, projects)
	})

	protected.GET("/variables", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseVariableFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		tags, err := stationViewQuery.ListVariables(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "variables query failed")
			return
		}
		c.JSON(http.StatusOK, tags)
	})

	protected.GET("/history/data", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		filter, err := parseHistoryFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		rows, limit, err := stationViewQuery.QueryHistoryData(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "history query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": rows, "count": len(rows), "limit": limit})
	})

	protected.GET("/limit-alarms", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseLimitAlarmFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		alarms, total, limit, offset, err := stationViewQuery.ListLimitAlarms(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "limit alarms query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": alarms, "total": total, "limit": limit, "offset": offset})
	})

	protected.GET("/task-flow-runs", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		filter, err := parseTaskFlowRunFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		runs, total, limit, offset, err := stationViewQuery.ListTaskFlowRuns(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "task flow runs query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": runs, "total": total, "limit": limit, "offset": offset})
	})

	protected.GET("/task-flow-runs/:id", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		runID, err := parseUintParam(c, "id")
		if err != nil || runID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id", "code": "invalid_id"})
			return
		}
		run, err := stationViewQuery.GetTaskFlowRun(runID, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "task flow run query failed")
			return
		}
		c.JSON(http.StatusOK, run)
	})

	protected.GET("/task-flow-runs/:id/sql-logs", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		runID, err := parseUintParam(c, "id")
		if err != nil || runID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id", "code": "invalid_id"})
			return
		}
		limit, err := parseOptionalPositiveInt(c, "limit", 100)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "invalid_limit"})
			return
		}
		logs, _, err := stationViewQuery.ListTaskFlowSQLLogs(runID, limit, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "task flow sql logs query failed")
			return
		}
		c.JSON(http.StatusOK, logs)
	})

	protected.GET("/detection-runs/active", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		tasks, err := stationViewQuery.ActiveDetectionRuns(edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "active detection runs query failed")
			return
		}
		c.JSON(http.StatusOK, tasks)
	})

	protected.GET("/detection-runs/current", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		projectID, err := parseUintQuery(c, "project_id")
		if err != nil || projectID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "project_id is required",
				"code":  "invalid_project_id",
			})
			return
		}
		task, err := stationViewQuery.CurrentDetectionRun(uint(projectID), edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "current detection run query failed")
			return
		}
		c.JSON(http.StatusOK, task)
	})

	protected.GET("/detection-runs", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseDetectionRunFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
				"code":  "invalid_query",
			})
			return
		}
		tasks, limit, err := stationViewQuery.ListDetectionRuns(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "detection runs query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": tasks, "count": len(tasks), "limit": limit})
	})

	protected.GET("/detection-runs/:id", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		task, err := stationViewQuery.GetDetectionRun(uint(taskID))
		if err != nil {
			writeSyncedReadError(c, err, "detection run query failed")
			return
		}
		c.JSON(http.StatusOK, task)
	})

	protected.GET("/detection-runs/:id/summary", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		summary, err := stationViewQuery.DetectionRunSummary(uint(taskID))
		if err != nil {
			writeSyncedReadError(c, err, "detection run summary query failed")
			return
		}
		c.JSON(http.StatusOK, summary)
	})

	protected.GET("/detection-runs/:id/features", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		features, err := stationViewQuery.DetectionRunFeatures(uint(taskID))
		if err != nil {
			writeSyncedReadError(c, err, "detection run features query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": features, "count": len(features)})
	})

	protected.GET("/detection-runs/:id/events", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		limit, err := parseOptionalPositiveInt(c, "limit", 200)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "invalid_limit"})
			return
		}
		events, normalizedLimit, err := stationViewQuery.DetectionRunEvents(uint(taskID), limit)
		if err != nil {
			writeSyncedReadError(c, err, "detection run events query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": events, "count": len(events), "limit": normalizedLimit})
	})

	protected.GET("/detection-runs/:id/storage-routes", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		routes, err := stationViewQuery.DetectionRunStorageRoutes(uint(taskID))
		if err != nil {
			writeSyncedReadError(c, err, "detection run storage routes query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": routes, "count": len(routes)})
	})

	protected.GET("/detection-runs/:id/report-requests", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		requests, err := stationViewQuery.DetectionRunReportRequests(uint(taskID))
		if err != nil {
			writeSyncedReadError(c, err, "detection run report requests query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": requests, "count": len(requests)})
	})

	protected.GET("/detection-runs/:id/notes", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		taskID, err := parseUintParam(c, "id")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id", "code": "invalid_task_id"})
			return
		}
		limit, err := parseOptionalPositiveInt(c, "limit", 200)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "invalid_limit"})
			return
		}
		notes, normalizedLimit, err := stationViewQuery.DetectionRunNotes(uint(taskID), limit, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "detection run notes query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": notes, "count": len(notes), "limit": normalizedLimit})
	})
	protected.POST("/detection-runs", authService.RequirePermission(auth.PermStartDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edgeControlClient, "api/v1/edge-control/detection/start", "")
	})
	protected.POST("/detection-runs/:id/stop", authService.RequirePermission(auth.PermStopDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edgeControlClient, "api/v1/edge-control/detection/stop", "id")
	})
	protected.POST("/detection-runs/:id/abnormal-stop", authService.RequirePermission(auth.PermStopDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edgeControlClient, "api/v1/edge-control/detection/abnormal-stop", "id")
	})
	protected.POST("/detection-runs/:id/pause", authService.RequirePermission(auth.PermStopDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edgeControlClient, "api/v1/edge-control/detection/pause", "id")
	})
	protected.POST("/detection-runs/:id/resume", authService.RequirePermission(auth.PermStartDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edgeControlClient, "api/v1/edge-control/detection/resume", "id")
	})
	protected.POST("/detection-runs/:id/notes", authService.RequirePermission(auth.PermStartDetection), mainServerDetectionNoteWriteUnsupported)

	protected.GET("/station-view/effective", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		projectID, err := parseUintQuery(c, "project_id")
		if err != nil || projectID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "project_id is required",
				"code":  "invalid_project_id",
			})
			return
		}
		response, err := stationViewQuery.Effective(uint(projectID), edgeContext(c, cfg))
		if err != nil {
			writeStationViewError(c, err)
			return
		}
		c.JSON(http.StatusOK, response)
	})

	registerEdgeControlRoutes(router, edgeControlClient)

	router.Any("/api/v1/edge-proxy/*path", func(c *gin.Context) {
		if isWriteMethod(c.Request.Method) {
			writeRawWriteDiagnostic(c)
			return
		}
		writeQueryProxyDisabledDiagnostic(c)
	})

	router.NoRoute(func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if isWriteMethod(c.Request.Method) {
			writeRawWriteDiagnostic(c)
			return
		}
		writeQueryProxyDisabledDiagnostic(c)
	})

	return router
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Command-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func registerEdgeControlRoutes(router *gin.Engine, client *edgecontrol.Client) {
	for _, route := range []string{
		"/api/v1/edge-control/detection/start",
		"/api/v1/edge-control/detection/stop",
		"/api/v1/edge-control/detection/abnormal-stop",
		"/api/v1/edge-control/detection/pause",
		"/api/v1/edge-control/detection/resume",
		"/api/v1/edge-control/detection/mute-alarms",
		"/api/v1/edge-control/detection/update-limits",
		"/api/v1/edge-control/detection/refresh-features",
		"/api/v1/edge-control/detection/report-requests",
		"/api/v1/edge-control/variables/write",
	} {
		path := route
		router.POST(path, func(c *gin.Context) {
			forwardEdgeControl(c, client, strings.TrimPrefix(path, "/"))
		})
	}
}

func forwardEdgeControl(c *gin.Context, client *edgecontrol.Client, path string) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "edge control request body is invalid",
			"code":  "invalid_payload",
		})
		return
	}
	commandID := firstNonEmpty(c.GetHeader("X-Command-ID"), commandIDFromBody(body))
	resp, err := client.Forward(c.Request.Context(), path, c.Request.URL.RawQuery, body, commandID)
	if err != nil {
		writeEdgeControlForwardError(c, err, client.ServiceTokenRef())
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, resp.Body)
}

func forwardUserDetectionControl(c *gin.Context, client *edgecontrol.Client, edgePath string, taskIDParam string) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "detection control request body is invalid",
			"code":  "invalid_payload",
		})
		return
	}
	payload, err := userDetectionControlPayload(body, c.Param(taskIDParam))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "detection control request body is invalid",
			"code":  "invalid_payload",
		})
		return
	}
	principal, ok := auth.PrincipalFromContext(c)
	if !ok || strings.TrimSpace(principal.Username) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user principal is missing", "code": "unauthorized"})
		return
	}
	commandID := firstNonEmpty(c.GetHeader("X-Command-ID"), commandIDFromBody(body), newMainCommandID())
	envelope, err := json.Marshal(gin.H{
		"command_id":        commandID,
		"operator_id":       strconv.FormatUint(uint64(principal.UserID), 10),
		"operator_name":     principal.Username,
		"operator_username": principal.Username,
		"payload":           json.RawMessage(payload),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "edge control envelope build failed", "code": "internal_error"})
		return
	}
	resp, err := client.Forward(c.Request.Context(), edgePath, c.Request.URL.RawQuery, envelope, commandID)
	if err != nil {
		writeEdgeControlForwardError(c, err, client.ServiceTokenRef())
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, resp.Body)
}

func userDetectionControlPayload(body []byte, taskIDParam string) ([]byte, error) {
	payload := map[string]any{}
	if strings.TrimSpace(string(body)) != "" {
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
	}
	delete(payload, "command_id")
	if taskIDParam != "" {
		taskID, err := strconv.ParseUint(strings.TrimSpace(taskIDParam), 10, 64)
		if err != nil || taskID == 0 {
			return nil, err
		}
		payload["task_id"] = taskID
	}
	return json.Marshal(payload)
}

func commandIDFromBody(body []byte) string {
	var envelope struct {
		CommandID string `json:"command_id"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.CommandID)
}

func newMainCommandID() string {
	return "main-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func writeEdgeControlForwardError(c *gin.Context, err error, serviceTokenRef string) {
	switch {
	case errors.Is(err, edgecontrol.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "edge control is disabled",
			"code":        "edge_control_disabled",
			"next_action": "enable edge control in main-server edge configuration before sending control commands",
		})
	case errors.Is(err, edgecontrol.ErrMissingToken):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":             "edge control service token is missing",
			"code":              "edge_control_token_missing",
			"service_token_ref": serviceTokenRef,
		})
	default:
		c.JSON(http.StatusBadGateway, gin.H{
			"error":       "edge backend unavailable",
			"code":        "edge_backend_unavailable",
			"next_action": "check edge base_url, service token, and edge backend health",
		})
	}
}

func writeEdgeRealtimeForwardError(c *gin.Context, err error, serviceTokenRef string) {
	switch {
	case errors.Is(err, edgecontrol.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "edge realtime mirror is disabled",
			"code":        "edge_realtime_disabled",
			"next_action": "enable edge realtime access in main-server edge configuration before requesting live snapshots",
		})
	case errors.Is(err, edgecontrol.ErrMissingToken):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":             "edge realtime service token is missing",
			"code":              "edge_realtime_token_missing",
			"service_token_ref": serviceTokenRef,
		})
	default:
		c.JSON(http.StatusBadGateway, gin.H{
			"error":       "edge realtime backend unavailable",
			"code":        "edge_realtime_unavailable",
			"next_action": "check edge base_url, service token, and edge backend health",
		})
	}
}

func forwardEdgeMetadataRead(c *gin.Context, client *edgecontrol.Client, path string) {
	resp, err := client.ForwardRead(c.Request.Context(), path, c.Request.URL.RawQuery)
	if err != nil {
		writeEdgeMetadataForwardError(c, err, client.ServiceTokenRef())
		return
	}
	writeForwardResponse(c, resp)
}

func forwardEdgeRuntimeRead(c *gin.Context, client *edgecontrol.Client, path string) {
	resp, err := client.ForwardRead(c.Request.Context(), path, c.Request.URL.RawQuery)
	if err != nil {
		writeEdgeRuntimeForwardError(c, err, client.ServiceTokenRef())
		return
	}
	writeForwardResponse(c, resp)
}

func writeForwardResponse(c *gin.Context, resp edgecontrol.Response) {
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, resp.Body)
}

func writeEdgeMetadataForwardError(c *gin.Context, err error, serviceTokenRef string) {
	switch {
	case errors.Is(err, edgecontrol.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "edge metadata mirror is disabled",
			"code":        "edge_metadata_disabled",
			"next_action": "enable edge metadata access in main-server edge configuration before opening task editors",
		})
	case errors.Is(err, edgecontrol.ErrMissingToken):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":             "edge metadata service token is missing",
			"code":              "edge_metadata_token_missing",
			"service_token_ref": serviceTokenRef,
		})
	default:
		c.JSON(http.StatusBadGateway, gin.H{
			"error":       "edge metadata backend unavailable",
			"code":        "edge_metadata_unavailable",
			"next_action": "check edge base_url, service token, and edge backend health",
		})
	}
}

func writeEdgeRuntimeForwardError(c *gin.Context, err error, serviceTokenRef string) {
	switch {
	case errors.Is(err, edgecontrol.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "edge runtime diagnostics mirror is disabled",
			"code":        "edge_runtime_disabled",
			"next_action": "enable edge runtime diagnostics access in main-server edge configuration before showing live runtime diagnostics",
		})
	case errors.Is(err, edgecontrol.ErrMissingToken):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":             "edge runtime diagnostics service token is missing",
			"code":              "edge_runtime_token_missing",
			"service_token_ref": serviceTokenRef,
		})
	default:
		c.JSON(http.StatusBadGateway, gin.H{
			"error":       "edge runtime diagnostics backend unavailable",
			"code":        "edge_runtime_unavailable",
			"next_action": "check edge base_url, service token, and edge backend health",
		})
	}
}

func writeRawWriteDiagnostic(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server raw write proxy is disabled",
		"code":        "edge_control_required",
		"path":        c.Request.URL.Path,
		"next_action": "use /api/v1/edge-control/* so the edge service token, command_id idempotency, operator mapping, and edge audit log are preserved",
	})
}

func writeQueryProxyDisabledDiagnostic(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server query proxy is disabled",
		"code":        "main_server_query_route_not_implemented",
		"path":        c.Request.URL.Path,
		"next_action": "port the matching edge read handler to main-server/backend/internal/query or add an explicit service-token mirror endpoint",
	})
}

func mainServerNotificationReadUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server notification read state is read-only",
		"code":        "main_server_notification_read_unsupported",
		"path":        c.Request.URL.Path,
		"next_action": "mark notifications read on the edge backend, or add a main-server-owned notification read-state table before enabling this write",
	})
}

func mainServerDetectionNoteWriteUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server detection notes are read-only",
		"code":        "main_server_detection_note_write_unsupported",
		"path":        c.Request.URL.Path,
		"next_action": "append detection notes on the edge backend, or add a controlled edge command before enabling main-server note writes",
	})
}

func mainServerRuntimeDiagnosticUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server does not own edge runtime diagnostics",
		"code":        "main_server_runtime_diagnostic_unsupported",
		"path":        c.Request.URL.Path,
		"next_action": "query the edge backend runtime diagnostics through an explicit service-token endpoint before showing live queue state on the main server",
	})
}

func mainServerRealtimeWSUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server realtime websocket bridge is not available in this stage",
		"code":        "main_server_realtime_ws_unsupported",
		"path":        c.Request.URL.Path,
		"next_action": "use GET /api/v1/realtime/variables for the current main-server realtime mirror, or add a service-token websocket bridge before enabling live websocket subscriptions",
	})
}

func mainServerDatabaseConfigView(cfg config.DatabaseConfig) gin.H {
	return gin.H{
		"host":             cfg.Host,
		"port":             cfg.Port,
		"user":             cfg.User,
		"name":             cfg.Name,
		"auto_migrate":     false,
		"password_set":     cfg.Password != "",
		"restart_required": false,
		"role":             "main_server",
		"source":           "main_server_config",
		"read_only":        true,
	}
}

func mainServerDatabaseConfigUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server database configuration is read-only in this stage",
		"code":        "main_server_database_config_read_only",
		"path":        c.Request.URL.Path,
		"next_action": "edit the main-server backend config file and restart the main-server process; do not write synchronized edge database settings from the main-server UI",
	})
}

func isWriteMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func statusText(ok bool) string {
	if ok {
		return "ok"
	}
	return "degraded"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseUintQuery(c *gin.Context, key string) (uint64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseUint(raw, 10, 64)
}

func parseUintParam(c *gin.Context, key string) (uint64, error) {
	raw := strings.TrimSpace(c.Param(key))
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseUint(raw, 10, 64)
}

func parseOptionalPositiveInt(c *gin.Context, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}

func parseDetectionRunFilter(c *gin.Context) (query.DetectionRunFilter, error) {
	var filter query.DetectionRunFilter
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return filter, errors.New("invalid project_id")
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	filter.Status = strings.TrimSpace(c.Query("status"))
	filter.TestNo = strings.TrimSpace(c.Query("test_no"))
	if raw := strings.TrimSpace(c.Query("start")); raw != "" {
		value, err := query.ParseDetectionRunTime(raw)
		if err != nil {
			return filter, errors.New("invalid start")
		}
		filter.Start = &value
	}
	if raw := strings.TrimSpace(c.Query("end")); raw != "" {
		value, err := query.ParseDetectionRunTime(raw)
		if err != nil {
			return filter, errors.New("invalid end")
		}
		filter.End = &value
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return filter, errors.New("invalid limit")
		}
		filter.Limit = value
	}
	return filter, nil
}

func parseVariableFilter(c *gin.Context) (query.VariableFilter, error) {
	var filter query.VariableFilter
	if raw := strings.TrimSpace(c.Query("gateway_id")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filter, errors.New("invalid gateway_id")
		}
		filter.GatewayID = &value
	}
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return filter, errors.New("invalid project_id")
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("invalid enabled")
		}
		filter.Enabled = &value
	}
	if raw := strings.TrimSpace(c.Query("discovered")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("invalid discovered")
		}
		filter.Discovered = &value
	}
	filter.SourceType = strings.TrimSpace(c.Query("source_type"))
	filter.Keyword = strings.TrimSpace(c.Query("keyword"))
	return filter, nil
}

func parseHistoryFilter(c *gin.Context) (query.HistoryFilter, error) {
	var filter query.HistoryFilter
	if raw := strings.TrimSpace(c.Query("project_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return filter, errors.New("invalid project_id")
		}
		projectID := uint(value)
		filter.ProjectID = &projectID
	}
	if raw := strings.TrimSpace(c.Query("task_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return filter, errors.New("invalid task_id")
		}
		taskID := uint(value)
		filter.TaskID = &taskID
	}
	filter.ProjectCode = strings.TrimSpace(c.Query("project_code"))
	filter.TestNo = strings.TrimSpace(c.Query("test_no"))
	if raw := strings.TrimSpace(c.Query("start")); raw != "" {
		value, err := query.ParseHistoryTime(raw)
		if err != nil {
			return filter, errors.New("invalid start")
		}
		filter.Start = &value
	}
	if raw := strings.TrimSpace(c.Query("end")); raw != "" {
		value, err := query.ParseHistoryTime(raw)
		if err != nil {
			return filter, errors.New("invalid end")
		}
		filter.End = &value
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return filter, errors.New("invalid limit")
		}
		filter.Limit = value
	}
	return filter, nil
}

func parseLimitAlarmFilter(c *gin.Context) (query.LimitAlarmFilter, error) {
	var filter query.LimitAlarmFilter
	scope := strings.TrimSpace(c.Query("scope"))
	if scope != "" && scope != query.AlarmScopeDefault && scope != query.AlarmScopeDetection {
		return filter, errors.New("scope must be default or detection")
	}
	filter.Scope = scope
	projectID, err := parseOptionalUintPointer(c, "project_id")
	if err != nil {
		return filter, err
	}
	filter.ProjectID = projectID
	taskID, err := parseOptionalUintPointer(c, "task_id")
	if err != nil {
		return filter, err
	}
	filter.TaskID = taskID
	varID, err := parseOptionalInt64Pointer(c, "var_id")
	if err != nil {
		return filter, err
	}
	filter.VarID = varID
	filter.TestNo = strings.TrimSpace(c.Query("test_no"))
	status, err := normalizeLimitAlarmStatus(c.Query("status"))
	if err != nil {
		return filter, err
	}
	filter.Status = status
	filter.AlarmType = strings.TrimSpace(c.Query("alarm_type"))
	filter.AlarmLevel = strings.TrimSpace(c.Query("alarm_level"))
	if filter.AlarmLevel == "" {
		filter.AlarmLevel = strings.TrimSpace(c.Query("level"))
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		value, err := query.ParseHistoryTime(raw)
		if err != nil {
			return filter, errors.New("invalid from")
		}
		filter.From = &value
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		value, err := query.ParseHistoryTime(raw)
		if err != nil {
			return filter, errors.New("invalid to")
		}
		filter.To = &value
	}
	limit, err := parseOptionalInt(c, "limit", 100)
	if err != nil {
		return filter, err
	}
	if limit <= 0 {
		return filter, errors.New("limit must be positive")
	}
	offset, err := parseOptionalInt(c, "offset", 0)
	if err != nil {
		return filter, err
	}
	if offset < 0 {
		return filter, errors.New("offset must be non-negative")
	}
	filter.Limit = limit
	filter.Offset = offset
	return filter, nil
}

func normalizeLimitAlarmStatus(raw string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(raw))
	switch status {
	case "", query.DetectionAlarmStatusOpen, query.DetectionAlarmStatusClose:
		return status, nil
	case "closed":
		return query.DetectionAlarmStatusClose, nil
	default:
		return "", errors.New("status must be active, recovered, or closed")
	}
}

func parseTaskFlowRunFilter(c *gin.Context) (query.TaskFlowRunFilter, error) {
	limit, err := parseOptionalInt(c, "limit", 50)
	if err != nil {
		return query.TaskFlowRunFilter{}, err
	}
	if limit <= 0 {
		return query.TaskFlowRunFilter{}, errors.New("limit must be positive")
	}
	offset, err := parseOptionalInt(c, "offset", 0)
	if err != nil {
		return query.TaskFlowRunFilter{}, err
	}
	if offset < 0 {
		return query.TaskFlowRunFilter{}, errors.New("offset must be non-negative")
	}
	filter := query.TaskFlowRunFilter{
		FlowCode:    strings.TrimSpace(c.Query("flow_code")),
		Status:      strings.ToLower(strings.TrimSpace(c.Query("status"))),
		TriggerType: strings.ToLower(strings.TrimSpace(c.Query("trigger_type"))),
		Limit:       limit,
		Offset:      offset,
	}
	if err := validateTaskFlowRunStatus(filter.Status); err != nil {
		return query.TaskFlowRunFilter{}, err
	}
	if err := validateTaskFlowTriggerType(filter.TriggerType); err != nil {
		return query.TaskFlowRunFilter{}, err
	}
	projectID, err := parseOptionalUintPointer(c, "project_id")
	if err != nil {
		return query.TaskFlowRunFilter{}, err
	}
	filter.ProjectID = projectID
	if raw := strings.TrimSpace(c.Query("flow_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return query.TaskFlowRunFilter{}, errors.New("flow_id must be positive")
		}
		filter.FlowID = &value
	}
	triggerVarID, err := parseOptionalInt64Pointer(c, "trigger_var_id")
	if err != nil {
		return query.TaskFlowRunFilter{}, err
	}
	filter.TriggerVarID = triggerVarID
	if raw := strings.TrimSpace(c.Query("origin_flow_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return query.TaskFlowRunFilter{}, errors.New("origin_flow_id must be positive")
		}
		filter.OriginFlowID = &value
	}
	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		value, err := query.ParseHistoryTime(raw)
		if err != nil {
			return query.TaskFlowRunFilter{}, errors.New("invalid from")
		}
		filter.From = &value
	}
	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		value, err := query.ParseHistoryTime(raw)
		if err != nil {
			return query.TaskFlowRunFilter{}, errors.New("invalid to")
		}
		filter.To = &value
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return query.TaskFlowRunFilter{}, errors.New("from must be before or equal to to")
	}
	return filter, nil
}

func parseAuditLogFilter(c *gin.Context) (query.AuditLogFilter, error) {
	limit, err := parseOptionalInt(c, "limit", 50)
	if err != nil {
		return query.AuditLogFilter{}, err
	}
	if limit <= 0 {
		return query.AuditLogFilter{}, errors.New("limit must be positive")
	}
	offset, err := parseOptionalInt(c, "offset", 0)
	if err != nil {
		return query.AuditLogFilter{}, err
	}
	if offset < 0 {
		return query.AuditLogFilter{}, errors.New("offset must be non-negative")
	}
	from, err := parseFlexibleQueryTime(firstNonEmpty(c.Query("created_from"), c.Query("from")))
	if err != nil {
		return query.AuditLogFilter{}, errors.New("invalid from")
	}
	to, err := parseFlexibleQueryTime(firstNonEmpty(c.Query("created_to"), c.Query("to")))
	if err != nil {
		return query.AuditLogFilter{}, errors.New("invalid to")
	}
	if from != nil && to != nil && from.After(*to) {
		return query.AuditLogFilter{}, errors.New("from must be before or equal to to")
	}
	return query.AuditLogFilter{
		ActorType:  strings.TrimSpace(c.Query("actor_type")),
		ActorID:    strings.TrimSpace(c.Query("actor_id")),
		Action:     strings.TrimSpace(c.Query("action")),
		TargetType: strings.TrimSpace(c.Query("target_type")),
		TargetID:   strings.TrimSpace(c.Query("target_id")),
		Result:     strings.TrimSpace(c.Query("result")),
		From:       from,
		To:         to,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func parseNotificationFilter(c *gin.Context, userID uint) (query.NotificationFilter, error) {
	limit, err := parseOptionalInt(c, "limit", 50)
	if err != nil {
		return query.NotificationFilter{}, err
	}
	if limit <= 0 {
		return query.NotificationFilter{}, errors.New("limit must be positive")
	}
	offset, err := parseOptionalInt(c, "offset", 0)
	if err != nil {
		return query.NotificationFilter{}, err
	}
	if offset < 0 {
		return query.NotificationFilter{}, errors.New("offset must be non-negative")
	}
	var unread *bool
	if raw := strings.TrimSpace(c.Query("unread")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return query.NotificationFilter{}, errors.New("unread must be a boolean")
		}
		unread = &parsed
	}
	projectID, err := parseOptionalUintPointer(c, "project_id")
	if err != nil {
		return query.NotificationFilter{}, err
	}
	from, err := parseFlexibleQueryTime(firstNonEmpty(c.Query("occurred_from"), c.Query("from")))
	if err != nil {
		return query.NotificationFilter{}, errors.New("invalid from")
	}
	to, err := parseFlexibleQueryTime(firstNonEmpty(c.Query("occurred_to"), c.Query("to")))
	if err != nil {
		return query.NotificationFilter{}, errors.New("invalid to")
	}
	if from != nil && to != nil && from.After(*to) {
		return query.NotificationFilter{}, errors.New("from must be before or equal to to")
	}
	return query.NotificationFilter{
		UserID:    userID,
		Unread:    unread,
		Type:      strings.TrimSpace(c.Query("type")),
		Level:     strings.TrimSpace(c.Query("level")),
		ProjectID: projectID,
		From:      from,
		To:        to,
		Keyword:   strings.TrimSpace(c.Query("keyword")),
		Limit:     limit,
		Offset:    offset,
	}, nil
}

func parseReportTemplateFilter(c *gin.Context) (query.ReportTemplateFilter, error) {
	var filter query.ReportTemplateFilter
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("enabled must be a boolean")
		}
		filter.Enabled = &value
	}
	filter.Keyword = strings.TrimSpace(c.Query("keyword"))
	return filter, nil
}

func parseStorageRouteFilter(c *gin.Context) (query.StorageRouteFilter, error) {
	var filter query.StorageRouteFilter
	projectID, err := parseOptionalUintPointer(c, "project_id")
	if err != nil {
		return filter, err
	}
	filter.ProjectID = projectID
	varID, err := parseOptionalInt64Pointer(c, "var_id")
	if err != nil {
		return filter, err
	}
	filter.VarID = varID
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("enabled must be a boolean")
		}
		filter.Enabled = &value
	}
	return filter, nil
}

func parseTaskFlowFilter(c *gin.Context) (query.TaskFlowFilter, error) {
	var filter query.TaskFlowFilter
	projectID, err := parseOptionalUintPointer(c, "project_id")
	if err != nil {
		return filter, err
	}
	filter.ProjectID = projectID
	triggerType := strings.ToLower(strings.TrimSpace(c.Query("trigger_type")))
	if err := validateTaskFlowTriggerType(triggerType); err != nil {
		return filter, err
	}
	filter.TriggerType = triggerType
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("enabled must be a boolean")
		}
		filter.Enabled = &value
	}
	return filter, nil
}

func parseDetectionStandardFilter(c *gin.Context) (query.DetectionStandardFilter, error) {
	var filter query.DetectionStandardFilter
	projectID, err := parseOptionalUintPointer(c, "project_id")
	if err != nil {
		return filter, err
	}
	filter.ProjectID = projectID
	filter.ProjectCode = strings.TrimSpace(c.Query("project_code"))
	filter.Mode = strings.TrimSpace(c.Query("mode"))
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("enabled must be a boolean")
		}
		filter.Enabled = &value
	}
	filter.Keyword = strings.TrimSpace(c.Query("keyword"))
	return filter, nil
}

func parseFlexibleQueryTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		value, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return &value, nil
		}
	}
	return nil, errors.New("invalid time")
}

func validateTaskFlowRunStatus(status string) error {
	if status == "" {
		return nil
	}
	switch status {
	case query.TaskFlowStatusPending,
		query.TaskFlowStatusRunning,
		query.TaskFlowStatusSuccess,
		query.TaskFlowStatusFailed,
		query.TaskFlowStatusTimeout,
		query.TaskFlowStatusSkipped:
		return nil
	default:
		return errors.New("invalid task flow run status: " + status)
	}
}

func validateTaskFlowTriggerType(triggerType string) error {
	if triggerType == "" {
		return nil
	}
	switch triggerType {
	case query.TaskFlowTriggerManual,
		query.TaskFlowTriggerDataChange,
		query.TaskFlowTriggerSchedule,
		query.TaskFlowTriggerProjectStart,
		query.TaskFlowTriggerProjectEnd:
		return nil
	default:
		return errors.New("invalid task flow trigger_type: " + triggerType)
	}
}

func parseOptionalUintPointer(c *gin.Context, key string) (*uint, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return nil, errors.New(key + " must be positive")
	}
	parsed := uint(value)
	return &parsed, nil
}

func parseOptionalInt64Pointer(c *gin.Context, key string) (*int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, errors.New(key + " must be positive")
	}
	return &value, nil
}

func parseOptionalInt(c *gin.Context, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(key + " must be an integer")
	}
	return value, nil
}

func edgeContext(c *gin.Context, cfg *config.Config) string {
	edgeInstanceID := strings.TrimSpace(c.Query("edge_instance_id"))
	if edgeInstanceID == "" {
		edgeInstanceID = cfg.Edge.EdgeInstanceID
	}
	return edgeInstanceID
}

func writeSyncedReadError(c *gin.Context, err error, message string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not found",
			"code":  "not_found",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": message,
			"code":  "internal_error",
		})
	}
}

func writeStationViewError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "station view project or edge context not found",
			"code":  "not_found",
		})
	case errors.Is(err, query.ErrStationViewSyncNotReady):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":       "station view synced configuration is not ready",
			"code":        "sync_not_ready",
			"next_action": "verify sys_station_view_templates, regions, items, assignments have been synchronized from edge/main configuration",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "station view query failed",
			"code":  "internal_error",
		})
	}
}
