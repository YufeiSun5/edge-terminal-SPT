package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/database"
	"spindle-edge/backend/internal/models"
	"spindle-edge/backend/internal/mqttx"
	"spindle-edge/backend/internal/pipeline"
	"spindle-edge/backend/internal/protocol/kio"
	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type GatewaysHandler struct {
	repo     *database.Repository
	mqtt     *mqttx.Manager
	channels *pipeline.Channels
	notify   *services.NotificationHub
	kio      *services.KIOWriteService
}

type gatewayConfigCreateRequest struct {
	ID               int    `json:"id"`
	EdgeInstanceID   string `json:"edge_instance_id"`
	Name             string `json:"name" binding:"required"`
	Broker           string `json:"broker" binding:"required"`
	ClientID         string `json:"client_id" binding:"required"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	Topic            string `json:"topic" binding:"required"`
	QOS              byte   `json:"qos"`
	ParserType       string `json:"parser_type"`
	KIOClientID      string `json:"kio_client_id"`
	KIOWriter        string `json:"kio_writer"`
	KIOWriteUsername string `json:"kio_write_username"`
	KIOWritePassword string `json:"kio_write_password"`
	SetDataTopic     string `json:"setdata_topic"`
	WriteResultTopic string `json:"write_result_topic"`
	QueryAllTopic    string `json:"query_all_topic"`
	Enabled          *bool  `json:"enabled"`
}

type gatewayConfigPatchRequest struct {
	EdgeInstanceID   *string `json:"edge_instance_id"`
	Name             *string `json:"name"`
	Broker           *string `json:"broker"`
	ClientID         *string `json:"client_id"`
	Username         *string `json:"username"`
	Password         *string `json:"password"`
	Topic            *string `json:"topic"`
	QOS              *byte   `json:"qos"`
	ParserType       *string `json:"parser_type"`
	KIOClientID      *string `json:"kio_client_id"`
	KIOWriter        *string `json:"kio_writer"`
	KIOWriteUsername *string `json:"kio_write_username"`
	KIOWritePassword *string `json:"kio_write_password"`
	SetDataTopic     *string `json:"setdata_topic"`
	WriteResultTopic *string `json:"write_result_topic"`
	QueryAllTopic    *string `json:"query_all_topic"`
	Enabled          *bool   `json:"enabled"`
}

type publishRequest struct {
	Topic   string          `json:"topic" binding:"required"`
	Payload json.RawMessage `json:"payload" binding:"required"`
	QOS     byte            `json:"qos"`
	Retain  bool            `json:"retain"`
}

type subscribeRequest struct {
	Topic string `json:"topic" binding:"required"`
	QOS   byte   `json:"qos"`
}

type kioWriteRequest struct {
	ClientID      string           `json:"client_id"`
	Topic         string           `json:"topic"`
	AckTopic      string           `json:"ack_topic"`
	Writer        string           `json:"writer"`
	WriteTime     string           `json:"write_time"`
	Username      string           `json:"username"`
	Password      string           `json:"password"`
	QID           int64            `json:"qid"`
	Values        []kio.WriteValue `json:"values" binding:"required"`
	QOS           byte             `json:"qos"`
	Retain        bool             `json:"retain"`
	WaitAck       bool             `json:"wait_ack"`
	AckTimeoutSec int              `json:"ack_timeout_sec"`
}

type kioQueryAllRequest struct {
	ClientID string `json:"client_id"`
	Topic    string `json:"topic"`
	Payload  string `json:"payload"`
	QOS      byte   `json:"qos"`
}

func NewGatewaysHandler(repo *database.Repository, mqtt *mqttx.Manager, channels *pipeline.Channels, notify *services.NotificationHub) *GatewaysHandler {
	return &GatewaysHandler{repo: repo, mqtt: mqtt, channels: channels, notify: notify, kio: services.NewKIOWriteService(mqtt)}
}

func (h *GatewaysHandler) Register(group *gin.RouterGroup, authService *auth.Service) {
	group.GET("/runtime/channels", authService.RequirePermission(auth.PermSystemSettings), h.runtimeChannels)
	group.GET("/runtime/channels/detail", authService.RequirePermission(auth.PermSystemSettings), h.runtimeChannelDetails)
	group.GET("/runtime/notifications", authService.RequirePermission(auth.PermSystemSettings), h.runtimeNotifications)
	group.GET("/runtime/workers", authService.RequirePermission(auth.PermSystemSettings), h.runtimeWorkers)
	group.GET("/gateways", authService.RequirePermission(auth.PermViewRealtime), h.status)
	group.GET("/gateway-configs", authService.RequirePermission(auth.PermManageGateways), h.listConfigs)
	group.GET("/gateway-configs/:gateway_id", authService.RequirePermission(auth.PermManageGateways), h.getConfig)
	group.POST("/gateway-configs", authService.RequirePermission(auth.PermManageGateways), h.createConfig)
	group.PATCH("/gateway-configs/:gateway_id", authService.RequirePermission(auth.PermManageGateways), h.patchConfig)
	group.DELETE("/gateway-configs/:gateway_id", authService.RequirePermission(auth.PermManageGateways), h.deleteConfig)
	group.POST("/gateway-configs/:gateway_id/discover", authService.RequirePermission(auth.PermManageGateways), h.discover)
	group.POST("/gateways/:gateway_id/publish", authService.RequirePermission(auth.PermKIOWrite), h.publish)
	group.POST("/gateways/:gateway_id/subscribe", authService.RequirePermission(auth.PermManageGateways), h.subscribe)
	group.POST("/gateways/:gateway_id/kio/write", authService.RequirePermission(auth.PermKIOWrite), h.kioWrite)
	group.POST("/gateways/:gateway_id/kio/query-all", authService.RequirePermission(auth.PermManageGateways), h.kioQueryAll)
}

func (h *GatewaysHandler) RegisterServiceRoutes(group *gin.RouterGroup, authService *auth.Service) {
	control := group.Group("/edge-control")
	control.GET("/gateways", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.status)
	control.GET("/runtime/channels", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.runtimeChannels)
	control.GET("/runtime/channels/detail", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.runtimeChannelDetails)
	control.GET("/runtime/notifications", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.runtimeNotifications)
	control.GET("/runtime/workers", authService.RequireServiceScope(auth.ScopeServiceRuntimeRead), h.runtimeWorkers)
}

func (h *GatewaysHandler) runtimeChannels(c *gin.Context) {
	c.JSON(http.StatusOK, h.channels.Stats())
}

func (h *GatewaysHandler) runtimeChannelDetails(c *gin.Context) {
	const pressureThreshold = 0.8
	c.JSON(http.StatusOK, gin.H{
		"items":              h.channels.DetailedStatsWithDiagnosis(pressureThreshold),
		"pressure":           h.channels.Pressure(pressureThreshold),
		"pressure_threshold": pressureThreshold,
	})
}

func (h *GatewaysHandler) runtimeNotifications(c *gin.Context) {
	c.JSON(http.StatusOK, h.notify.RuntimeStats())
}

func (h *GatewaysHandler) runtimeWorkers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"items": pipeline.WorkerRecoveryStats()})
}

func (h *GatewaysHandler) status(c *gin.Context) {
	c.JSON(http.StatusOK, h.mqtt.Status())
}

func (h *GatewaysHandler) listConfigs(c *gin.Context) {
	gateways, err := h.repo.ListGatewayConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gateways)
}

func (h *GatewaysHandler) getConfig(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	gateway, err := h.repo.GetGatewayConfig(gatewayID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gateway)
}

func (h *GatewaysHandler) createConfig(c *gin.Context) {
	var req gatewayConfigCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	qos := req.QOS
	if qos == 0 {
		qos = 1
	}
	gateway := models.GatewayConfig{
		ID:               req.ID,
		EdgeInstanceID:   req.EdgeInstanceID,
		Name:             req.Name,
		Broker:           req.Broker,
		ClientID:         req.ClientID,
		Username:         req.Username,
		Password:         req.Password,
		Topic:            req.Topic,
		QOS:              qos,
		ParserType:       firstNonEmpty(req.ParserType, "kingiot_kio"),
		KIOClientID:      req.KIOClientID,
		KIOWriter:        req.KIOWriter,
		KIOWriteUsername: req.KIOWriteUsername,
		KIOWritePassword: req.KIOWritePassword,
		SetDataTopic:     req.SetDataTopic,
		WriteResultTopic: req.WriteResultTopic,
		QueryAllTopic:    req.QueryAllTopic,
		Enabled:          enabled,
	}
	if err := h.repo.CreateGatewayConfig(&gateway); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.mqtt.ApplyConfig(gateway); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "gateway": gateway})
		return
	}
	c.JSON(http.StatusOK, gateway)
}

func (h *GatewaysHandler) patchConfig(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	var req gatewayConfigPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	gateway, err := h.repo.UpdateGatewayConfig(gatewayID, gatewayConfigUpdates(req))
	if err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	if err := h.mqtt.ApplyConfig(gateway); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "gateway": gateway})
		return
	}
	c.JSON(http.StatusOK, gateway)
}

func (h *GatewaysHandler) deleteConfig(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	h.mqtt.Stop(gatewayID)
	if err := h.repo.DeleteGatewayConfig(gatewayID); err != nil {
		c.JSON(services.HTTPStatusForError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *GatewaysHandler) discover(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	var req kioQueryAllRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.publishKIOQueryAll(c, gatewayID, req)
}

func (h *GatewaysHandler) publish(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	var req publishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.mqtt.Publish(ctx, gatewayID, req.Topic, req.Payload, req.QOS, req.Retain); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "broker_accepted": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"gateway_id":        gatewayID,
		"topic":             req.Topic,
		"broker_accepted":   true,
		"Project_confirmed": false,
	})
}

func (h *GatewaysHandler) subscribe(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	var req subscribeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.mqtt.Subscribe(ctx, gatewayID, req.Topic, req.QOS); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"gateway_id": gatewayID, "topic": req.Topic, "subscribed": true})
}

func (h *GatewaysHandler) kioWrite(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	var req kioWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.kio.Write(c.Request.Context(), services.KIOWriteInput{
		GatewayID:     gatewayID,
		ClientID:      req.ClientID,
		Topic:         req.Topic,
		AckTopic:      req.AckTopic,
		Writer:        req.Writer,
		WriteTime:     req.WriteTime,
		Username:      req.Username,
		Password:      req.Password,
		QID:           req.QID,
		Values:        req.Values,
		QOS:           req.QOS,
		Retain:        req.Retain,
		WaitAck:       req.WaitAck,
		AckTimeoutSec: req.AckTimeoutSec,
	})
	if err != nil {
		c.JSON(services.HTTPStatusForKIOError(err), gin.H{"error": err.Error(), "broker_accepted": result.BrokerAccepted, "status": result.Status})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *GatewaysHandler) kioQueryAll(c *gin.Context) {
	gatewayID, err := strconv.Atoi(c.Param("gateway_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gateway_id"})
		return
	}
	var req kioQueryAllRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.publishKIOQueryAll(c, gatewayID, req)
}

func (h *GatewaysHandler) publishKIOQueryAll(c *gin.Context, gatewayID int, req kioQueryAllRequest) {
	gatewayCfg, ok := h.mqtt.Config(gatewayID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway not found"})
		return
	}
	clientID := firstNonEmpty(req.ClientID, gatewayCfg.KIOClientID)
	topic := firstNonEmpty(req.Topic, gatewayCfg.QueryAllTopic)
	if topic == "" && clientID != "" {
		topic = kio.QueryAllTagsTopic(clientID)
	}
	if topic == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "topic or client_id is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	qos := req.QOS
	if qos == 0 {
		qos = gatewayCfg.QOS
	}
	if err := h.mqtt.Publish(ctx, gatewayID, topic, []byte(req.Payload), qos, false); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "broker_accepted": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"gateway_id": gatewayID, "topic": topic, "broker_accepted": true})
}

func gatewayConfigUpdates(req gatewayConfigPatchRequest) map[string]interface{} {
	updates := make(map[string]interface{})
	setStringUpdate(updates, "edge_instance_id", req.EdgeInstanceID)
	setStringUpdate(updates, "name", req.Name)
	setStringUpdate(updates, "broker", req.Broker)
	setStringUpdate(updates, "client_id", req.ClientID)
	setStringUpdate(updates, "username", req.Username)
	setStringUpdate(updates, "password", req.Password)
	setStringUpdate(updates, "topic", req.Topic)
	if req.QOS != nil {
		updates["qos"] = *req.QOS
	}
	setStringUpdate(updates, "parser_type", req.ParserType)
	setStringUpdate(updates, "kio_client_id", req.KIOClientID)
	setStringUpdate(updates, "kio_writer", req.KIOWriter)
	setStringUpdate(updates, "kio_write_username", req.KIOWriteUsername)
	setStringUpdate(updates, "kio_write_password", req.KIOWritePassword)
	setStringUpdate(updates, "setdata_topic", req.SetDataTopic)
	setStringUpdate(updates, "write_result_topic", req.WriteResultTopic)
	setStringUpdate(updates, "query_all_topic", req.QueryAllTopic)
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	return updates
}
