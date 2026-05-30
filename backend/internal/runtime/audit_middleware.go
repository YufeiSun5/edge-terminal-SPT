package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spindle-edge/backend/internal/auth"
	"spindle-edge/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type auditStore interface {
	CreateAuditLog(entry *models.SysAuditLog) error
}

func auditWriteMiddleware(store auditStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isWriteMethod(c.Request.Method) {
			c.Next()
			return
		}
		requestID := requestIDFromHeaders(c)
		if requestID == "" {
			requestID = newRequestID()
		}
		c.Header("X-Request-ID", requestID)

		startedAt := time.Now()
		c.Next()

		if store == nil {
			return
		}
		principal, _ := auth.PrincipalFromContext(c)
		actorType, actorID := auditActor(principal)
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		status := c.Writer.Status()
		detail := auditDetail{
			RequestID:   requestID,
			CommandID:   strings.TrimSpace(c.GetHeader("X-Command-ID")),
			Method:      c.Request.Method,
			Path:        c.Request.URL.Path,
			Route:       route,
			Status:      status,
			ClientIP:    c.ClientIP(),
			UserAgent:   c.Request.UserAgent(),
			LatencyMS:   time.Since(startedAt).Milliseconds(),
			Error:       auditError(c),
			ActorName:   principal.Username,
			ServiceName: principal.ClientID,
		}
		rawDetail, err := json.Marshal(detail)
		if err != nil {
			rawDetail = []byte(`{"error":"failed to marshal audit detail"}`)
		}
		_ = store.CreateAuditLog(&models.SysAuditLog{
			ActorType:  actorType,
			ActorID:    actorID,
			Action:     strings.ToLower("http." + c.Request.Method),
			TargetType: "http_endpoint",
			TargetID:   route,
			Result:     auditResult(status),
			Detail:     string(rawDetail),
			CreatedAt:  time.Now(),
		})
	}
}

type auditDetail struct {
	RequestID   string `json:"request_id"`
	CommandID   string `json:"command_id,omitempty"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Route       string `json:"route"`
	Status      int    `json:"status"`
	ClientIP    string `json:"client_ip,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
	LatencyMS   int64  `json:"latency_ms"`
	Error       string `json:"error,omitempty"`
	ActorName   string `json:"actor_name,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestIDFromHeaders(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Request-ID")); value != "" {
		return value
	}
	return strings.TrimSpace(c.GetHeader("X-Request-Id"))
}

func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buf[:])
}

func auditActor(principal auth.Principal) (string, string) {
	switch principal.AuthType {
	case "user":
		return "user", strconv.FormatUint(uint64(principal.UserID), 10)
	case "service":
		return "service", principal.ClientID
	default:
		return "unknown", ""
	}
}

func auditResult(status int) string {
	if status >= 200 && status < 400 {
		return "success"
	}
	return "failed"
}

func auditError(c *gin.Context) string {
	if len(c.Errors) == 0 {
		if c.Writer.Status() >= 400 {
			return http.StatusText(c.Writer.Status())
		}
		return ""
	}
	return c.Errors.String()
}
