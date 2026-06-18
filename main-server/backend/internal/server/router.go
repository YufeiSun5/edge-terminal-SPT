package server

import (
	"bytes"
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
	"spindle-main-server/backend/internal/reports"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(cfg *config.Config, db *gorm.DB) http.Handler {
	cfg.EnsureEdges()
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors())
	stationViewQuery := query.NewStationViewQuery(db)
	reportService := reports.NewService(db, stationViewQuery, reports.Options{
		ArtifactDir: cfg.Reports.ArtifactDir,
		MaxAttempts: cfg.Reports.MaxAttempts,
	})
	authService := auth.NewService(
		auth.NewUserStore(db),
		auth.NewJWTManager(cfg.Auth.JWTSecret, time.Duration(cfg.Auth.AccessTokenTTLSeconds)*time.Second),
		auth.Options{
			EdgeBaseURL:         cfg.Edge.BaseURL,
			EdgeServiceTokenRef: cfg.Edge.ServiceTokenRef,
			Getenv:              os.Getenv,
		},
	)
	edges := newEdgeRegistry(cfg)

	router.GET("/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		dbOK := err == nil && sqlDB.Ping() == nil
		c.JSON(http.StatusOK, gin.H{
			"status":           statusText(dbOK),
			"role":             "main_server",
			"database_ok":      dbOK,
			"edge_base_url":    cfg.Edge.BaseURL,
			"edge_instance_id": cfg.Edge.EdgeInstanceID,
			"edge_nodes":       edges.StatusNodes(cfg.Database.Name),
			"time":             time.Now().Format(time.RFC3339Nano),
		})
	})

	router.POST("/api/v1/auth/login", authService.Login)
	router.POST("/api/v1/auth/sso-ticket/verify", authService.VerifySSOTicket)
	router.GET("/api/v1/ws", authService.RequireUserFromBearerOrQuery(), authService.RequirePermission(auth.PermViewRealtime), mainServerRealtimeWSBridge(edges, stationViewQuery))
	protected := router.Group("/api/v1")
	protected.Use(authService.RequireUser())
	protected.GET("/auth/me", authService.Me)
	protected.POST("/auth/logout", authService.Logout)
	protected.POST("/auth/sso-ticket", authService.RequirePermission(auth.PermSSOHandoff), authService.CreateSSOTicket)
	protected.GET("/users", authService.RequirePermission(auth.PermManageUsers), authService.ListUsers)
	protected.GET("/gateways", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edges, stationViewQuery, "api/v1/edge-control/gateways")
	})
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
		forwardEdgeRuntimeRead(c, edges, stationViewQuery, "api/v1/edge-control/runtime/channels")
	})
	protected.GET("/runtime/channels/detail", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edges, stationViewQuery, "api/v1/edge-control/runtime/channels/detail")
	})
	protected.GET("/runtime/notifications", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edges, stationViewQuery, "api/v1/edge-control/runtime/notifications")
	})
	protected.GET("/runtime/workers", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edges, stationViewQuery, "api/v1/edge-control/runtime/workers")
	})
	protected.GET("/task-modules", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeMetadataRead(c, edges, stationViewQuery, "api/v1/edge-control/task-modules")
	})
	protected.GET("/task-flow-templates", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeMetadataRead(c, edges, stationViewQuery, "api/v1/edge-control/task-flow-templates")
	})
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
	protected.GET("/main-server/report-templates", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		filter, err := parseReportTemplateFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
			return
		}
		templates, err := stationViewQuery.ListReportTemplates(filter)
		if err != nil {
			writeSyncedReadError(c, err, "report templates query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": templates, "count": len(templates)})
	})
	protected.POST("/main-server/report-templates/upload", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is required", "code": "invalid_report_template_file"})
			return
		}
		opened, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file cannot be opened", "code": "invalid_report_template_file"})
			return
		}
		defer func() { _ = opened.Close() }()
		const maxTemplateUploadBytes = 50 << 20
		raw, err := io.ReadAll(io.LimitReader(opened, maxTemplateUploadBytes+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file cannot be read", "code": "invalid_report_template_file"})
			return
		}
		if len(raw) > maxTemplateUploadBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is too large", "code": "report_template_file_too_large"})
			return
		}
		enabled := true
		if rawEnabled := strings.TrimSpace(c.PostForm("enabled")); rawEnabled != "" {
			parsed, err := strconv.ParseBool(rawEnabled)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean", "code": "invalid_enabled"})
				return
			}
			enabled = parsed
		}
		version := 0
		if rawVersion := strings.TrimSpace(c.PostForm("version")); rawVersion != "" {
			parsed, err := strconv.Atoi(rawVersion)
			if err != nil || parsed <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "version must be a positive integer", "code": "invalid_version"})
				return
			}
			version = parsed
		}
		paramsSchema := firstNonEmpty(c.PostForm("params_schema_json"), c.PostForm("mapping_json"))
		template, meta, err := reportService.UploadTemplate(c.Request.Context(), reports.TemplateUploadInput{
			TemplateCode:     c.PostForm("template_code"),
			Name:             c.PostForm("name"),
			DisplayName:      c.PostForm("display_name"),
			Version:          version,
			ParamsSchemaJSON: paramsSchema,
			Remark:           c.PostForm("remark"),
			Enabled:          enabled,
		}, raw, file.Filename)
		if err != nil {
			writeReportTemplateError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"template": template, "artifact": meta})
	})
	protected.PATCH("/main-server/report-templates/:id/mapping", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		templateID, err := parseUintParam(c, "id")
		if err != nil || templateID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report template id", "code": "invalid_report_template_id"})
			return
		}
		var req struct {
			ParamsSchemaJSON string          `json:"params_schema_json"`
			Mapping          json.RawMessage `json:"mapping"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body", "code": "invalid_body"})
			return
		}
		paramsSchema := strings.TrimSpace(req.ParamsSchemaJSON)
		if paramsSchema == "" && len(req.Mapping) > 0 {
			paramsSchema = string(req.Mapping)
		}
		template, err := reportService.UpdateTemplateMapping(uint(templateID), paramsSchema)
		if err != nil {
			writeReportTemplateError(c, err)
			return
		}
		c.JSON(http.StatusOK, template)
	})
	protected.GET("/main-server/report-templates/:id/artifact", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		templateID, err := parseUintParam(c, "id")
		if err != nil || templateID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report template id", "code": "invalid_report_template_id"})
			return
		}
		path, name, contentType, err := reportService.TemplateArtifact(uint(templateID))
		if err != nil {
			writeReportTemplateError(c, err)
			return
		}
		c.Header("Content-Type", contentType)
		c.FileAttachment(path, name)
	})
	protected.POST("/main-server/report-plan-imports/parse", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is required", "code": "invalid_plan_import_file"})
			return
		}
		opened, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file cannot be opened", "code": "invalid_plan_import_file"})
			return
		}
		defer func() { _ = opened.Close() }()
		const maxPlanImportBytes = 50 << 20
		raw, err := io.ReadAll(io.LimitReader(opened, maxPlanImportBytes+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file cannot be read", "code": "invalid_plan_import_file"})
			return
		}
		if len(raw) > maxPlanImportBytes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file is too large", "code": "plan_import_file_too_large"})
			return
		}
		edgeInstanceID := firstNonEmpty(c.PostForm("edge_instance_id"), edgeContext(c, cfg))
		draft, err := reportService.ParsePlanImport(c.Request.Context(), raw, file.Filename, edgeInstanceID)
		if err != nil {
			writePlanImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, draft)
	})
	protected.POST("/main-server/report-plan-imports/confirm", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		var req reports.PlanImportConfirmInput
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body", "code": "invalid_body"})
			return
		}
		if strings.TrimSpace(req.EdgeInstanceID) == "" {
			req.EdgeInstanceID = edgeContext(c, cfg)
		}
		result, err := reportService.ConfirmPlanImport(c.Request.Context(), req, syncWriteMeta(c))
		if err != nil {
			writePlanImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	protected.POST("/main-server/report-jobs/enqueue", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		var req struct {
			TaskID         uint   `json:"task_id"`
			Force          bool   `json:"force"`
			EdgeInstanceID string `json:"edge_instance_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.TaskID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required", "code": "invalid_task_id"})
			return
		}
		edgeInstanceID := strings.TrimSpace(req.EdgeInstanceID)
		if edgeInstanceID == "" {
			edgeInstanceID = edgeContext(c, cfg)
		}
		result, err := reportService.EnqueueTask(req.TaskID, edgeInstanceID, req.Force)
		if err != nil {
			writeReportJobError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	protected.GET("/main-server/report-jobs", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		filter, err := parseReportJobFilter(c, cfg)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
			return
		}
		jobs, total, limit, offset, err := reportService.ListJobs(filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "report jobs query failed", "code": "internal_error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": jobs, "total": total, "limit": limit, "offset": offset})
	})
	protected.GET("/main-server/report-jobs/:id", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		jobID, err := parseUintParam(c, "id")
		if err != nil || jobID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report job id", "code": "invalid_report_job_id"})
			return
		}
		job, err := reportService.GetJob(jobID)
		if err != nil {
			writeReportJobError(c, err)
			return
		}
		c.JSON(http.StatusOK, job)
	})
	protected.GET("/main-server/report-jobs/:id/events", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		jobID, err := parseUintParam(c, "id")
		if err != nil || jobID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report job id", "code": "invalid_report_job_id"})
			return
		}
		limit, err := parseOptionalInt(c, "limit", 100)
		if err != nil || limit <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "invalid_limit"})
			return
		}
		events, normalizedLimit, err := reportService.ListEvents(jobID, limit)
		if err != nil {
			writeReportJobError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": events, "count": len(events), "limit": normalizedLimit})
	})
	protected.GET("/main-server/report-jobs/:id/artifact", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		jobID, err := parseUintParam(c, "id")
		if err != nil || jobID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report job id", "code": "invalid_report_job_id"})
			return
		}
		path, name, contentType, err := reportService.Artifact(jobID)
		if err != nil {
			writeReportJobError(c, err)
			return
		}
		c.Header("Content-Type", contentType)
		c.FileAttachment(path, name)
	})
	protected.POST("/main-server/report-jobs/:id/retry", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		jobID, err := parseUintParam(c, "id")
		if err != nil || jobID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report job id", "code": "invalid_report_job_id"})
			return
		}
		job, err := reportService.RetryJob(jobID)
		if err != nil {
			writeReportJobError(c, err)
			return
		}
		c.JSON(http.StatusOK, job)
	})
	protected.POST("/main-server/report-jobs/:id/regenerate", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		jobID, err := parseUintParam(c, "id")
		if err != nil || jobID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report job id", "code": "invalid_report_job_id"})
			return
		}
		var req struct {
			ParamsJSON string          `json:"params_json"`
			Params     json.RawMessage `json:"params"`
			Reason     string          `json:"reason"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body", "code": "invalid_body"})
			return
		}
		paramsJSON := strings.TrimSpace(req.ParamsJSON)
		if paramsJSON == "" && len(req.Params) > 0 {
			paramsJSON = string(req.Params)
		}
		if paramsJSON == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "params or params_json is required", "code": "invalid_report_params"})
			return
		}
		operator := ""
		if principal, ok := auth.PrincipalFromContext(c); ok {
			operator = principal.Username
		}
		job, err := reportService.RegenerateJobWithParams(jobID, reports.RegenerateReportInput{
			ParamsJSON: paramsJSON,
			Reason:     req.Reason,
			Operator:   operator,
		})
		if err != nil {
			if strings.Contains(err.Error(), "invalid report params_json") {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_report_params"})
				return
			}
			writeReportJobError(c, err)
			return
		}
		c.JSON(http.StatusOK, job)
	})
	protected.POST("/main-server/download-packages", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		var req struct {
			TaskID         uint     `json:"task_id"`
			EdgeInstanceID string   `json:"edge_instance_id"`
			Keys           []string `json:"keys"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.TaskID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required", "code": "invalid_task_id"})
			return
		}
		if strings.TrimSpace(req.EdgeInstanceID) == "" {
			req.EdgeInstanceID = edgeContext(c, cfg)
		}
		operator := ""
		if principal, ok := auth.PrincipalFromContext(c); ok {
			operator = principal.Username
		}
		pkg, err := reportService.BuildDownloadPackage(c.Request.Context(), reports.DownloadPackageInput{
			TaskID:         req.TaskID,
			EdgeInstanceID: req.EdgeInstanceID,
			Keys:           req.Keys,
			Operator:       operator,
		})
		if err != nil {
			writeReportJobError(c, err)
			return
		}
		c.Header("Content-Type", pkg.ContentType)
		c.DataFromReader(http.StatusOK, int64(len(pkg.Data)), pkg.ContentType, bytes.NewReader(pkg.Data), map[string]string{
			"Content-Disposition": `attachment; filename="` + pkg.Name + `"`,
		})
	})
	protected.GET("/main-server/report-notifications", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
			return
		}
		filter, err := parseReportNotificationFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
			return
		}
		items, total, limit, offset, err := reportService.ListNotifications(principal.UserID, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "report notifications query failed", "code": "internal_error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "limit": limit, "offset": offset})
	})
	protected.GET("/main-server/report-notifications/unread-count", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
			return
		}
		filter, err := parseReportNotificationFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
			return
		}
		count, err := reportService.UnreadNotificationCount(principal.UserID, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "report notification count failed", "code": "internal_error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"unread": count})
	})
	protected.POST("/main-server/report-notifications/:id/read", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
			return
		}
		notificationID, err := parseUintParam(c, "id")
		if err != nil || notificationID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report notification id", "code": "invalid_report_notification_id"})
			return
		}
		if err := reportService.MarkNotificationRead(principal.UserID, notificationID); err != nil {
			writeReportNotificationError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	protected.POST("/main-server/report-notifications/read-all", authService.RequirePermission(auth.PermViewHistory), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "unauthorized"})
			return
		}
		filter, err := parseReportNotificationFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
			return
		}
		updated, err := reportService.MarkNotificationsRead(principal.UserID, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "report notifications read failed", "code": "internal_error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "updated": updated})
	})
	protected.GET("/realtime/variables", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		if _, exists := c.GetQuery("device_id"); exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "unsupported_query_param: device_id",
				"code":  "unsupported_query_param",
			})
			return
		}
		edgeClient, _, ok := resolveRealtimeEdgeClient(c, edges, stationViewQuery)
		if !ok {
			return
		}
		resp, err := edgeClient.ForwardRead(c.Request.Context(), "api/v1/edge-control/realtime/variables", edgeForwardRawQuery(c.Request.URL.Query()))
		if err != nil {
			writeEdgeRealtimeForwardError(c, err, edgeClient.ServiceTokenRef())
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
	protected.POST("/task-flows", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		var req query.TaskFlow
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
			return
		}
		flow, err := stationViewQuery.CreateTaskFlow(&req, syncWriteMeta(c))
		if err != nil {
			writeSyncedReadError(c, err, "task flow write failed")
			return
		}
		c.JSON(http.StatusCreated, flow)
	})
	protected.GET("/task-flows/runtime", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		forwardEdgeRuntimeRead(c, edges, stationViewQuery, "api/v1/edge-control/task-flows/runtime")
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
	protected.PUT("/task-flows/:id", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		flowID, err := parseUintParam(c, "id")
		if err != nil || flowID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task flow id", "code": "invalid_task_flow_id"})
			return
		}
		var req query.TaskFlow
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
			return
		}
		vars := req.Vars
		flow, err := stationViewQuery.UpdateTaskFlow(flowID, taskFlowDefinitionUpdates(req), &vars, syncWriteMeta(c))
		if err != nil {
			writeSyncedReadError(c, err, "task flow write failed")
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
	protected.POST("/detection-standards", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		var req query.DetectionStandard
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
			return
		}
		items := req.Items
		req.Items = nil
		standard, err := stationViewQuery.CreateDetectionStandard(&req, items, syncWriteMeta(c))
		if err != nil {
			writeSyncedReadError(c, err, "detection standard write failed")
			return
		}
		c.JSON(http.StatusCreated, standard)
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
	protected.PUT("/detection-standards/:id", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		standardID, err := parseUintParam(c, "id")
		if err != nil || standardID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid standard id", "code": "invalid_standard_id"})
			return
		}
		var req query.DetectionStandard
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
			return
		}
		items := req.Items
		standard, err := stationViewQuery.UpdateDetectionStandard(uint(standardID), detectionStandardDefinitionUpdates(req), &items, syncWriteMeta(c))
		if err != nil {
			writeSyncedReadError(c, err, "detection standard write failed")
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
	protected.POST("/notifications/:id/read", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		principal, ok := auth.PrincipalFromContext(c)
		if !ok || principal.AuthType != "user" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required", "code": "unauthorized"})
			return
		}
		notificationID, err := parseUintParam(c, "id")
		if err != nil || notificationID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id", "code": "invalid_notification_id"})
			return
		}
		if err := stationViewQuery.MarkNotificationRead(principal.UserID, notificationID, edgeContext(c, cfg)); err != nil {
			writeSyncedReadError(c, err, "notification read update failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	protected.POST("/notifications/read-all", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
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
		updated, err := stationViewQuery.MarkUserNotificationsRead(filter, edgeContext(c, cfg))
		if err != nil {
			writeSyncedReadError(c, err, "notifications read update failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "updated": updated})
	})

	router.GET("/api/v1/main-server/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"role":                "main_server",
			"query_source":        "local_mysql",
			"edge_control_target": cfg.Edge.BaseURL,
			"query_proxy_enabled": false,
			"edge_nodes":          edges.StatusNodes(cfg.Database.Name),
		})
	})

	protected.GET("/projects", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		edgeInstanceID := strings.TrimSpace(c.Query("edge_instance_id"))
		includeLegacy := false
		if edgeInstanceID == "" {
			edgeInstanceID = edgeContext(c, cfg)
			includeLegacy = true
		}
		limit, err := parseOptionalPositiveInt(c, "limit", 0)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit", "code": "invalid_limit"})
			return
		}
		offset := 0
		if rawOffset := strings.TrimSpace(c.Query("offset")); rawOffset != "" {
			offset, err = strconv.Atoi(rawOffset)
		}
		if err != nil || offset < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset", "code": "invalid_offset"})
			return
		}
		projects, err := stationViewQuery.ListProjects(edgeInstanceID, includeLegacy, limit, offset)
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
		if _, hasLimit := c.GetQuery("limit"); hasLimit {
			tags, total, limit, offset, err := stationViewQuery.ListVariablesPage(filter, edgeContext(c, cfg))
			if err != nil {
				writeSyncedReadError(c, err, "variables query failed")
				return
			}
			c.JSON(http.StatusOK, gin.H{"items": tags, "total": total, "limit": limit, "offset": offset})
			return
		}
		if _, hasOffset := c.GetQuery("offset"); hasOffset {
			tags, total, limit, offset, err := stationViewQuery.ListVariablesPage(filter, edgeContext(c, cfg))
			if err != nil {
				writeSyncedReadError(c, err, "variables query failed")
				return
			}
			c.JSON(http.StatusOK, gin.H{"items": tags, "total": total, "limit": limit, "offset": offset})
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

	protected.GET("/detection-plans", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		filter, err := parseDetectionPlanFilter(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_query"})
			return
		}
		plans, total, limit, offset, err := stationViewQuery.ListDetectionPlans(filter)
		if err != nil {
			writeSyncedReadError(c, err, "detection plans query failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": plans, "count": len(plans), "total": total, "limit": limit, "offset": offset})
	})

	protected.GET("/detection-plans/:id", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		planID, err := parseUintParam(c, "id")
		if err != nil || planID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id", "code": "invalid_plan_id"})
			return
		}
		plan, err := stationViewQuery.GetDetectionPlan(uint(planID))
		if err != nil {
			writeSyncedReadError(c, err, "detection plan query failed")
			return
		}
		c.JSON(http.StatusOK, plan)
	})

	protected.PATCH("/detection-plans/:id", authService.RequirePermission(auth.PermStartDetection), func(c *gin.Context) {
		planID, err := parseUintParam(c, "id")
		if err != nil || planID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id", "code": "invalid_plan_id"})
			return
		}
		var req struct {
			PlanNo          string `json:"plan_no"`
			SourceSystem    string `json:"source_system"`
			ExternalPlanID  string `json:"external_plan_id"`
			ExternalOrderNo string `json:"external_order_no"`
			FactoryNo       string `json:"factory_no"`
			CustomerName    string `json:"customer_name"`
			DeviceModel     string `json:"device_model"`
			TestItemCode    string `json:"test_item_code"`
			TestItemName    string `json:"test_item_name"`
			TestSequence    int    `json:"test_sequence"`
			Mode            string `json:"mode"`
			StandardCode    string `json:"standard_code"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_payload"})
			return
		}
		principal, _ := auth.PrincipalFromContext(c)
		plan, err := stationViewQuery.UpdatePendingDetectionPlan(uint(planID), query.DetectionPlanUpdate{
			PlanNo:          req.PlanNo,
			SourceSystem:    req.SourceSystem,
			ExternalPlanID:  req.ExternalPlanID,
			ExternalOrderNo: req.ExternalOrderNo,
			FactoryNo:       req.FactoryNo,
			CustomerName:    req.CustomerName,
			DeviceModel:     req.DeviceModel,
			TestItemCode:    req.TestItemCode,
			TestItemName:    req.TestItemName,
			TestSequence:    req.TestSequence,
			Mode:            req.Mode,
			StandardCode:    req.StandardCode,
			UpdatedByUser:   principal.Username,
		})
		if err != nil {
			if errors.Is(err, query.ErrDetectionPlanNotEditable) {
				c.JSON(http.StatusConflict, gin.H{"error": "detection plan is not editable", "code": "detection_plan_not_editable"})
				return
			}
			if errors.Is(err, query.ErrDetectionPlanInvalid) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_payload"})
				return
			}
			writeSyncedReadError(c, err, "detection plan update failed")
			return
		}
		c.JSON(http.StatusOK, plan)
	})

	protected.POST("/detection-plans/:id/start", authService.RequirePermission(auth.PermStartDetection), func(c *gin.Context) {
		planID, err := parseUintParam(c, "id")
		if err != nil || planID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan id", "code": "invalid_plan_id"})
			return
		}
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection-plans/"+strconv.FormatUint(planID, 10)+"/start", "")
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
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection/start", "")
	})
	protected.POST("/detection-runs/:id/stop", authService.RequirePermission(auth.PermStopDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection/stop", "id")
	})
	protected.POST("/detection-runs/:id/abnormal-stop", authService.RequirePermission(auth.PermStopDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection/abnormal-stop", "id")
	})
	protected.POST("/detection-runs/:id/pause", authService.RequirePermission(auth.PermStopDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection/pause", "id")
	})
	protected.POST("/detection-runs/:id/resume", authService.RequirePermission(auth.PermStartDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection/resume", "id")
	})
	protected.POST("/detection-runs/:id/apply-config", authService.RequirePermission(auth.PermStartDetection), func(c *gin.Context) {
		forwardUserDetectionControl(c, edges, stationViewQuery, "api/v1/edge-control/detection/apply-config", "id")
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
		edgeInstanceID, ok := resolveStationViewEdgeInstanceID(c, edges, stationViewQuery, uint(projectID))
		if !ok {
			return
		}
		response, err := stationViewQuery.Effective(uint(projectID), edgeInstanceID)
		if err != nil {
			writeStationViewError(c, err)
			return
		}
		c.JSON(http.StatusOK, response)
	})
	protected.GET("/station-view/templates", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		templates, err := stationViewQuery.ListStationViewTemplates(query.StationViewTemplateFilter{
			Status:     c.Query("status"),
			OwnerScope: c.Query("owner_scope"),
			Keyword:    c.Query("keyword"),
		})
		if err != nil {
			writeStationViewError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":        templates,
			"count":        len(templates),
			"query_source": "synced_mysql",
		})
	})
	protected.GET("/station-view/items", authService.RequirePermission(auth.PermViewRealtime), func(c *gin.Context) {
		templateUID := strings.TrimSpace(c.Query("template_uid"))
		if templateUID == "" {
			projectID, err := parseUintQuery(c, "project_id")
			if err != nil || projectID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "template_uid or project_id is required", "code": "invalid_query"})
				return
			}
			edgeInstanceID, ok := resolveStationViewEdgeInstanceID(c, edges, stationViewQuery, uint(projectID))
			if !ok {
				return
			}
			effective, err := stationViewQuery.Effective(uint(projectID), edgeInstanceID)
			if err != nil {
				writeStationViewError(c, err)
				return
			}
			templateUID = effective.Template.TemplateUID
		}
		items, err := stationViewQuery.ListStationViewItems(templateUID)
		if err != nil {
			writeStationViewError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"template_uid": templateUID, "items": items, "count": len(items), "query_source": "synced_mysql"})
	})
	protected.PUT("/station-view/items", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		var req stationViewItemsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_body"})
			return
		}
		templateUID := strings.TrimSpace(req.TemplateUID)
		if templateUID == "" {
			templateUID = strings.TrimSpace(c.Query("template_uid"))
		}
		if templateUID == "" {
			projectID := req.ProjectID
			if projectID == 0 {
				parsed, err := parseUintQuery(c, "project_id")
				if err == nil {
					projectID = uint(parsed)
				}
			}
			if projectID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "template_uid or project_id is required", "code": "invalid_query"})
				return
			}
			edgeInstanceID, ok := resolveStationViewEdgeInstanceID(c, edges, stationViewQuery, projectID)
			if !ok {
				return
			}
			effective, err := stationViewQuery.Effective(projectID, edgeInstanceID)
			if err != nil {
				writeStationViewError(c, err)
				return
			}
			templateUID = effective.Template.TemplateUID
		}
		items, err := stationViewQuery.ReplaceStationViewItems(templateUID, req.Items, syncWriteMeta(c))
		if err != nil {
			writeStationViewError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items), "query_source": "synced_mysql"})
	})
	protected.POST("/station-view/reload", authService.RequirePermission(auth.PermSystemSettings), func(c *gin.Context) {
		diagnostics := stationViewQuery.SyncDiagnostics()
		status := http.StatusOK
		if diagnostics.OverallStatus != "ok" {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{
			"ok":           diagnostics.OverallStatus == "ok",
			"reload_mode":  "sync_diagnostics_only",
			"query_source": "synced_mysql",
			"diagnostics":  diagnostics,
			"next_action":  "wait for database synchronization to copy sys_station_view_* rows; main-server reload does not call edge-control",
		})
	})

	registerEdgeControlRoutes(router, edges, stationViewQuery)

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

func registerEdgeControlRoutes(router *gin.Engine, registry *edgeRegistry, stationViewQuery *query.StationViewQuery) {
	for _, route := range []string{
		"/api/v1/edge-control/detection/start",
		"/api/v1/edge-control/detection/stop",
		"/api/v1/edge-control/detection/abnormal-stop",
		"/api/v1/edge-control/detection/pause",
		"/api/v1/edge-control/detection/resume",
		"/api/v1/edge-control/detection/mute-alarms",
		"/api/v1/edge-control/detection/update-limits",
		"/api/v1/edge-control/detection/apply-config",
		"/api/v1/edge-control/detection/refresh-features",
		"/api/v1/edge-control/detection/report-requests",
		"/api/v1/edge-control/variables/write",
	} {
		path := route
		router.POST(path, func(c *gin.Context) {
			forwardEdgeControl(c, registry, stationViewQuery, strings.TrimPrefix(path, "/"))
		})
	}
}

func forwardEdgeControl(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, path string) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4*1024*1024))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "edge control request body is invalid",
			"code":  "invalid_payload",
		})
		return
	}
	client, _, ok := resolveControlEdgeClient(c, registry, stationViewQuery, body)
	if !ok {
		return
	}
	commandID := firstNonEmpty(c.GetHeader("X-Command-ID"), commandIDFromBody(body))
	resp, err := client.Forward(c.Request.Context(), path, edgeForwardRawQuery(c.Request.URL.Query()), body, commandID)
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

func forwardUserDetectionControl(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, edgePath string, taskIDParam string) {
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
	client, _, ok := resolveControlEdgeClient(c, registry, stationViewQuery, payload)
	if !ok {
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
	resp, err := client.Forward(c.Request.Context(), edgePath, edgeForwardRawQuery(c.Request.URL.Query()), envelope, commandID)
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

func forwardEdgeMetadataRead(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, path string) {
	client, _, ok := resolveRealtimeEdgeClient(c, registry, stationViewQuery)
	if !ok {
		return
	}
	resp, err := client.ForwardRead(c.Request.Context(), path, edgeForwardRawQuery(c.Request.URL.Query()))
	if err != nil {
		writeEdgeMetadataForwardError(c, err, client.ServiceTokenRef())
		return
	}
	writeForwardResponse(c, resp)
}

func forwardEdgeRuntimeRead(c *gin.Context, registry *edgeRegistry, stationViewQuery *query.StationViewQuery, path string) {
	client, _, ok := resolveRealtimeEdgeClient(c, registry, stationViewQuery)
	if !ok {
		return
	}
	resp, err := client.ForwardRead(c.Request.Context(), path, edgeForwardRawQuery(c.Request.URL.Query()))
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

func mainServerDetectionNoteWriteUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":       "main server detection notes are read-only",
		"code":        "main_server_detection_note_write_unsupported",
		"path":        c.Request.URL.Path,
		"next_action": "append detection notes on the edge backend, or add a controlled edge command before enabling main-server note writes",
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
	filter.FactoryNo = strings.TrimSpace(c.Query("factory_no"))
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

func parseDetectionPlanFilter(c *gin.Context) (query.DetectionPlanFilter, error) {
	var filter query.DetectionPlanFilter
	filter.Status = strings.TrimSpace(c.Query("status"))
	filter.FactoryNo = strings.TrimSpace(c.Query("factory_no"))
	filter.Keyword = strings.TrimSpace(c.Query("keyword"))
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return filter, errors.New("invalid limit")
		}
		filter.Limit = value
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return filter, errors.New("invalid offset")
		}
		filter.Offset = value
	}
	return filter, nil
}

func parseVariableFilter(c *gin.Context) (query.VariableFilter, error) {
	var filter query.VariableFilter
	if _, exists := c.GetQuery("device_id"); exists {
		return filter, errors.New("unsupported_query_param: device_id")
	}
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
	if raw := strings.TrimSpace(c.Query("writable")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("invalid writable")
		}
		filter.Writable = &value
	}
	if raw := strings.TrimSpace(c.Query("assigned")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return filter, errors.New("invalid assigned")
		}
		filter.Assigned = &value
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return filter, errors.New("invalid limit")
		}
		filter.Limit = value
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return filter, errors.New("invalid offset")
		}
		filter.Offset = value
	}
	filter.SourceType = strings.TrimSpace(c.Query("source_type"))
	filter.ProjectCode = strings.TrimSpace(c.Query("project_code"))
	filter.VarGroup = strings.TrimSpace(c.Query("var_group"))
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
	filter.FactoryNo = strings.TrimSpace(c.Query("factory_no"))
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

func parseReportJobFilter(c *gin.Context, cfg *config.Config) (reports.JobFilter, error) {
	limit, err := parseOptionalInt(c, "limit", 50)
	if err != nil {
		return reports.JobFilter{}, err
	}
	if limit <= 0 {
		return reports.JobFilter{}, errors.New("limit must be positive")
	}
	offset, err := parseOptionalInt(c, "offset", 0)
	if err != nil {
		return reports.JobFilter{}, err
	}
	if offset < 0 {
		return reports.JobFilter{}, errors.New("offset must be non-negative")
	}
	taskID, err := parseOptionalUintPointer(c, "task_id")
	if err != nil {
		return reports.JobFilter{}, err
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if err := validateReportJobStatus(status); err != nil {
		return reports.JobFilter{}, err
	}
	edgeInstanceID := strings.TrimSpace(c.Query("edge_instance_id"))
	if edgeInstanceID == "" {
		edgeInstanceID = strings.TrimSpace(cfg.Edge.EdgeInstanceID)
	}
	return reports.JobFilter{
		Status:         status,
		TaskID:         taskID,
		EdgeInstanceID: edgeInstanceID,
		Limit:          limit,
		Offset:         offset,
	}, nil
}

func validateReportJobStatus(status string) error {
	if status == "" {
		return nil
	}
	switch status {
	case reports.StatusPending,
		reports.StatusWaitingForSync,
		reports.StatusGenerating,
		reports.StatusSucceeded,
		reports.StatusFailed,
		reports.StatusWaitingLegacy,
		reports.StatusRunningLegacy,
		reports.StatusSuccessLegacy:
		return nil
	default:
		return errors.New("invalid report job status")
	}
}

func parseReportNotificationFilter(c *gin.Context) (reports.NotificationFilter, error) {
	limit, err := parseOptionalInt(c, "limit", 50)
	if err != nil {
		return reports.NotificationFilter{}, err
	}
	if limit <= 0 {
		return reports.NotificationFilter{}, errors.New("limit must be positive")
	}
	offset, err := parseOptionalInt(c, "offset", 0)
	if err != nil {
		return reports.NotificationFilter{}, err
	}
	if offset < 0 {
		return reports.NotificationFilter{}, errors.New("offset must be non-negative")
	}
	jobID, err := parseOptionalUint64Pointer(c, "job_id")
	if err != nil {
		return reports.NotificationFilter{}, err
	}
	level := strings.ToLower(strings.TrimSpace(c.Query("level")))
	if err := validateReportNotificationLevel(level); err != nil {
		return reports.NotificationFilter{}, err
	}
	var unread *bool
	if raw := strings.TrimSpace(c.Query("unread")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return reports.NotificationFilter{}, errors.New("unread must be a boolean")
		}
		unread = &value
	}
	return reports.NotificationFilter{
		JobID:  jobID,
		Level:  level,
		Unread: unread,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func validateReportNotificationLevel(level string) error {
	if level == "" {
		return nil
	}
	switch level {
	case "info", "success", "warning", "error":
		return nil
	default:
		return errors.New("invalid report notification level")
	}
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
	filter.ProjectGroup = strings.TrimSpace(c.Query("project_group"))
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

type stationViewItemsRequest struct {
	TemplateUID string                     `json:"template_uid"`
	ProjectID   uint                       `json:"project_id"`
	Items       []query.StationViewItemDTO `json:"items"`
}

func syncWriteMeta(c *gin.Context) query.SyncWriteMeta {
	meta := query.SyncWriteMeta{
		UpdatedByNode:  "main-server",
		EdgeInstanceID: strings.TrimSpace(c.Query("edge_instance_id")),
		SyncScope:      strings.TrimSpace(c.Query("sync_scope")),
	}
	if principal, ok := auth.PrincipalFromContext(c); ok {
		meta.UpdatedByUser = principal.Username
		if meta.UpdatedByUser == "" && principal.UserID > 0 {
			meta.UpdatedByUser = strconv.FormatUint(uint64(principal.UserID), 10)
		}
	}
	return meta
}

func detectionStandardDefinitionUpdates(req query.DetectionStandard) map[string]any {
	return map[string]any{
		"standard_code":      req.StandardCode,
		"name":               req.Name,
		"display_name":       req.DisplayName,
		"display_name_en":    req.DisplayNameEN,
		"display_name_ja":    req.DisplayNameJA,
		"project_id":         req.ProjectID,
		"project_code":       req.ProjectCode,
		"project_group":      req.ProjectGroup,
		"mode":               req.Mode,
		"report_template_id": req.ReportTemplateID,
		"enabled":            req.Enabled,
		"remark":             req.Remark,
		"sync_scope":         req.SyncScope,
		"edge_instance_id":   req.EdgeInstanceID,
	}
}

func taskFlowDefinitionUpdates(req query.TaskFlow) map[string]any {
	return map[string]any{
		"project_id":           req.ProjectID,
		"flow_code":            req.FlowCode,
		"name":                 req.Name,
		"enabled":              req.Enabled,
		"trigger_type":         req.TriggerType,
		"condition_script":     req.ConditionScript,
		"action_type":          req.ActionType,
		"action_script":        req.ActionScript,
		"action_payload":       req.ActionPayload,
		"steps_json":           req.StepsJSON,
		"timeout_ms":           req.TimeoutMS,
		"cooldown_ms":          req.CooldownMS,
		"hold_ms":              req.HoldMS,
		"schedule_interval_ms": req.ScheduleIntervalMS,
		"priority":             req.Priority,
		"remark":               req.Remark,
		"sync_scope":           req.SyncScope,
		"edge_instance_id":     req.EdgeInstanceID,
	}
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

func parseOptionalUint64Pointer(c *gin.Context, key string) (*uint64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return nil, errors.New(key + " must be positive")
	}
	return &value, nil
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

func writeReportJobError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "code": "not_found"})
	case errors.Is(err, reports.ErrReportNotRequested):
		c.JSON(http.StatusConflict, gin.H{"error": "report was not requested for this detection run", "code": "report_not_requested"})
	case errors.Is(err, reports.ErrJobNotRetryable):
		c.JSON(http.StatusConflict, gin.H{"error": "report job is not retryable", "code": "report_job_not_retryable"})
	case errors.Is(err, reports.ErrArtifactNotReady):
		c.JSON(http.StatusConflict, gin.H{"error": "report artifact is not ready", "code": "report_artifact_not_ready"})
	case errors.Is(err, reports.ErrArtifactUnavailable):
		c.JSON(http.StatusNotFound, gin.H{"error": "report artifact is unavailable", "code": "report_artifact_unavailable"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "report job operation failed", "code": "internal_error"})
	}
}

func writeReportTemplateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, reports.ErrTemplateNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "report template not found", "code": "report_template_not_found"})
	case errors.Is(err, reports.ErrInvalidReportTemplate):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_report_template"})
	case errors.Is(err, reports.ErrArtifactUnavailable):
		c.JSON(http.StatusNotFound, gin.H{"error": "report template artifact is unavailable", "code": "report_template_artifact_unavailable"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "report template operation failed", "code": "internal_error"})
	}
}

func writePlanImportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reports.ErrInvalidReportTemplate):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_plan_import"})
	case errors.Is(err, reports.ErrPlanImportNotReady):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "plan_import_not_ready"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plan import operation failed", "code": "internal_error"})
	}
}

func writeReportNotificationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found", "code": "not_found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "report notification operation failed", "code": "internal_error"})
	}
}

func writeStationViewError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, query.ErrEdgeInstanceMismatch):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "station view project does not belong to requested edge instance",
			"code":  "station_view_edge_instance_mismatch",
		})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "station view project not found",
			"code":  "station_view_project_not_found",
		})
	case errors.Is(err, query.ErrStationViewTemplateConflict):
		c.JSON(http.StatusConflict, gin.H{
			"error": "station view template assignment conflict",
			"code":  "station_view_template_conflict",
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
