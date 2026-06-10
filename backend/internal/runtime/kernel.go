package runtime

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/discovery"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/mqttx"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/runtime/handlers"
	"spindle-edge/backend/internal/services"
	"spindle-edge/backend/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Kernel struct {
	cfg      *config.Config
	repo     *database.Repository
	channels *pipeline.Channels
	tags     *pipeline.TagManager
	tasks    *pipeline.TaskManager
	flows    *pipeline.TaskFlowExecutor
	notify   *services.NotificationHub
	mqtt     *mqttx.Manager
	router   *gin.Engine
	auth     *auth.Service
}

func NewKernel(cfg *config.Config, db *gorm.DB) *Kernel {
	channels := pipeline.NewChannels()
	tags := pipeline.NewTagManager()
	tasks := pipeline.NewTaskManager()
	repo := database.NewRepository(db)
	notifications := services.NewNotificationHub(nil)
	flows := pipeline.NewTaskFlowExecutor(repo, tags, tasks, channels)
	jwt := auth.NewJWTManager(cfg.Auth.JWTSecret, time.Duration(cfg.Auth.AccessTokenTTLSeconds)*time.Second)

	k := &Kernel{
		cfg:      cfg,
		repo:     repo,
		channels: channels,
		tags:     tags,
		tasks:    tasks,
		flows:    flows,
		notify:   notifications,
		mqtt:     mqttx.NewManager(channels),
		router:   gin.New(),
		auth: auth.NewService(repo, jwt, auth.Options{
			EdgeInstanceID: cfg.Auth.EdgeInstanceID,
			MainSiteURL:    cfg.Auth.MainSiteURL,
			SSOTicketTTL:   time.Duration(cfg.Auth.SSOTicketTTLSeconds) * time.Second,
		}),
	}
	k.mountRoutes()
	return k
}

func (k *Kernel) Start() error {
	if err := k.seedGateways(); err != nil {
		return err
	}
	if err := k.seedAuth(); err != nil {
		return err
	}
	if err := k.repo.EnsureProjectDisplayNameFallbacks(); err != nil {
		return err
	}
	if err := k.repo.EnsureDefaultStationViewTemplate(); err != nil {
		return err
	}

	tagConfigs, err := k.repo.LoadTags(k.cfg.Auth.EdgeInstanceID)
	if err != nil {
		return err
	}
	k.tags.Load(tagConfigs)
	log.Printf("loaded tags: %d", k.tags.Count())

	activeTasks, err := k.repo.LoadActiveDetectionTasks()
	if err != nil {
		return err
	}
	k.tasks.Load(activeTasks)
	log.Printf("loaded active detection tasks: %d", len(activeTasks))

	taskRules, err := k.repo.LoadEnabledTaskRules()
	if err != nil {
		return err
	}
	k.tasks.LoadTaskRules(taskRules)
	log.Printf("loaded task rules: %d", len(taskRules))

	taskFlows, err := k.repo.LoadEnabledTaskFlows()
	if err != nil {
		return err
	}
	k.flows.Load(taskFlows)
	log.Printf("loaded task flows: %d", len(taskFlows))

	gateways, err := k.repo.LoadGateways(k.cfg.Auth.EdgeInstanceID)
	if err != nil {
		return err
	}

	discovery.Start(k.channels, k.repo, k.tags, k.cfg.Auth.EdgeInstanceID)
	services.NewNotificationDispatcher(k.repo, k.notify).Start(k.channels.Notify)
	k.flows.Start(1)
	k.flows.StartScheduleScanner(time.Second)
	k.flows.RecoverActiveDetectionGuards(activeTasks)
	pipeline.StartChannelPressureLogger(k.channels, 5*time.Second, 0.8)
	pipeline.StartLogicWorkers(k.cfg.App.LogicWorkers, k.channels, k.tags, k.tasks, k.flows)
	pipeline.StartCycleScanner(k.channels, k.tags, k.tasks)
	storage.StartWorkers(k.cfg.App.StoreWorkers, k.cfg.App.HistoryBatch, k.channels, k.repo)
	storage.StartAlarmWorkers(1, k.channels, k.repo)
	k.mqtt.StartAll(gateways)
	log.Printf("loaded mqtt gateways: %d", len(gateways))
	return nil
}

func (k *Kernel) Stop() {
	k.mqtt.StopAll()
}

func (k *Kernel) Router() http.Handler {
	return k.router
}

func (k *Kernel) seedGateways() error {
	if len(k.cfg.Gateways) == 0 {
		return nil
	}
	gateways := make([]models.GatewayConfig, 0, len(k.cfg.Gateways))
	for _, item := range k.cfg.Gateways {
		gateways = append(gateways, models.GatewayConfig{
			ID:               item.ID,
			EdgeInstanceID:   firstNonEmpty(item.EdgeInstanceID, k.cfg.Auth.EdgeInstanceID),
			Name:             item.Name,
			Broker:           item.Broker,
			ClientID:         item.ClientID,
			Username:         item.Username,
			Password:         item.Password,
			Topic:            item.Topic,
			QOS:              item.QOS,
			ParserType:       item.ParserType,
			KIOClientID:      item.KIOClientID,
			KIOWriter:        item.KIOWriter,
			KIOWriteUsername: item.KIOWriteUsername,
			KIOWritePassword: item.KIOWritePassword,
			SetDataTopic:     item.SetDataTopic,
			WriteResultTopic: item.WriteResultTopic,
			QueryAllTopic:    item.QueryAllTopic,
			Enabled:          item.Enabled,
		})
	}
	return k.repo.UpsertGatewaySeeds(gateways)
}

func (k *Kernel) seedAuth() error {
	if k.cfg.Auth.BootstrapAdminUsername != "" && k.cfg.Auth.BootstrapAdminPassword != "" {
		count, err := k.repo.CountUsers()
		if err != nil {
			return err
		}
		if count == 0 {
			passwordHash, err := auth.HashPassword(k.cfg.Auth.BootstrapAdminPassword)
			if err != nil {
				return err
			}
			if err := k.repo.CreateUser(&models.SysUser{
				Username:           k.cfg.Auth.BootstrapAdminUsername,
				PasswordHash:       passwordHash,
				Role:               auth.RoleAdmin,
				Enabled:            true,
				PermissionsVersion: 1,
			}); err != nil {
				return err
			}
			log.Printf("bootstrapped admin user: %s", k.cfg.Auth.BootstrapAdminUsername)
		}
	}

	for _, seed := range k.cfg.Auth.ServiceClients {
		if seed.ClientID == "" || seed.Token == "" {
			continue
		}
		if err := k.repo.UpsertServiceClient(models.SysServiceClient{
			ClientID:     seed.ClientID,
			SecretHash:   auth.HashOpaqueToken(seed.Token),
			Scopes:       auth.NormalizeScopes(seed.Scopes),
			AllowedCIDRs: auth.NormalizeScopes(seed.AllowedCIDRs),
			Enabled:      seed.Enabled,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (k *Kernel) mountRoutes() {
	k.router.Use(corsMiddleware(), gin.Logger(), gin.Recovery())

	k.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"tags":     k.tags.Count(),
			"channels": k.channels.Stats(),
			"gateways": k.mqtt.Status(),
		})
	})

	v1 := k.router.Group("/api/v1")
	v1.POST("/auth/login", k.auth.Login)
	v1.POST("/auth/sso-ticket/verify", k.auth.RequireServiceScope(auth.ScopeServiceSSOVerify), k.auth.VerifySSOTicket)

	protected := v1.Group("")
	protected.Use(k.auth.RequireUser(), auditWriteMiddleware(k.repo))
	protected.GET("/auth/me", k.auth.Me)
	protected.POST("/auth/logout", k.auth.Logout)
	protected.POST("/auth/sso-ticket", k.auth.RequirePermission(auth.PermSSOHandoff), k.auth.CreateSSOTicket)

	variablesService := services.NewVariablesService(k.repo, k.tags, k.cfg.Auth.EdgeInstanceID)
	detectionRunsService := services.NewDetectionRunsService(k.repo, k.tasks, services.DetectionRunsRuntimeDeps{Tags: k.tags, Channels: k.channels, Flows: k.flows})
	reportTemplatesService := services.NewReportTemplatesService(k.repo)
	systemConfigService := services.NewSystemConfigService(k.cfg)
	realtimeWSService := services.NewRealtimeWSService(k.tags, k.tasks)
	kioWriteService := services.NewKIOWriteService(k.mqtt)
	variableWriteService := services.NewVariableWriteService(k.repo, k.tags, kioWriteService, k.flows)
	k.flows.SetVariableWriter(func(ctx context.Context, input pipeline.TaskFlowVariableWriteInput) (map[string]any, error) {
		result, err := variableWriteService.Write(ctx, services.VariableWriteInput{
			VarID:          input.VarID,
			Value:          input.Value,
			Quality:        input.Quality,
			Trigger:        input.Trigger,
			WaitAck:        input.WaitAck,
			AckTimeoutSec:  input.AckTimeoutSec,
			OriginFlowID:   input.OriginFlowID,
			OriginRunID:    input.OriginRunID,
			Depth:          input.Depth,
			MaxDepth:       input.MaxDepth,
			AllowReentrant: input.AllowReentrant,
			RequestID:      input.RequestID,
		})
		out := map[string]any{
			"var_id":            result.VarID,
			"var_name":          result.VarName,
			"source_type":       result.SourceType,
			"value":             result.Value,
			"quality":           result.Quality,
			"triggered":         result.Triggered,
			"origin_flow_id":    result.OriginFlowID,
			"origin_run_id":     result.OriginRunID,
			"depth":             result.Depth,
			"next_depth":        result.NextDepth,
			"max_depth":         result.MaxDepth,
			"allow_reentrant":   result.AllowReentrant,
			"request_id":        result.RequestID,
			"broker_accepted":   result.BrokerAccepted,
			"project_confirmed": result.ProjectConfirmed,
		}
		if result.ProjectID != nil {
			out["project_id"] = *result.ProjectID
		}
		return out, err
	})

	handlers.NewRealtimeWSHandler(realtimeWSService, detectionRunsService, k.repo, variableWriteService).WithNotificationHub(k.notify).Register(v1, k.auth)
	handlers.NewEdgeControlHandler(k.repo, detectionRunsService, variableWriteService).Register(v1, k.auth)
	handlers.NewEdgeRealtimeHandler(variablesService).Register(v1, k.auth)
	gatewaysHandler := handlers.NewGatewaysHandler(k.repo, k.mqtt, k.channels, k.notify)
	taskFlowsHandler := handlers.NewTaskFlowsHandler(k.repo, k.flows)
	gatewaysHandler.RegisterServiceRoutes(v1, k.auth)
	taskFlowsHandler.RegisterServiceRoutes(v1, k.auth)
	handlers.NewUsersHandler(k.repo).Register(protected, k.auth)
	handlers.NewProjectsHandler(k.repo).Register(protected, k.auth)
	handlers.NewStationViewHandler(k.repo, k.cfg.Auth.EdgeInstanceID).Register(protected, k.auth)
	handlers.NewVariablesHandler(variablesService).Register(protected, k.auth)
	handlers.NewStorageRoutesHandler(k.repo).Register(protected, k.auth)
	handlers.NewHistoryHandler(k.repo).Register(protected, k.auth)
	handlers.NewDetectionStandardsHandler(k.repo).Register(protected, k.auth)
	gatewaysHandler.Register(protected, k.auth)
	handlers.NewReportTemplatesHandler(reportTemplatesService).Register(protected, k.auth)
	handlers.NewDetectionRunsHandler(detectionRunsService).Register(protected, k.auth)
	handlers.NewSystemConfigHandler(systemConfigService).Register(protected, k.auth)
	handlers.NewAuditLogsHandler(k.repo).Register(protected, k.auth)
	handlers.NewNotificationsHandler(k.repo).Register(protected, k.auth)
	handlers.NewLimitAlarmsHandler(k.repo).Register(protected, k.auth)
	taskFlowsHandler.Register(protected, k.auth)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if isAllowedDesktopOrigin(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,Accept")
		c.Header("Access-Control-Max-Age", "600")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func isAllowedDesktopOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	return strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://[::1]:")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
