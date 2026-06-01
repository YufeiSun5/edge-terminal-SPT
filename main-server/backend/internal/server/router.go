package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"spindle-main-server/backend/internal/config"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(cfg *config.Config, db *gorm.DB) http.Handler {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), cors())

	edgeURL, _ := url.Parse(cfg.Edge.BaseURL)
	edgeProxy := httputil.NewSingleHostReverseProxy(edgeURL)
	edgeProxy.ErrorHandler = func(c http.ResponseWriter, r *http.Request, err error) {
		c.Header().Set("Content-Type", "application/json")
		c.WriteHeader(http.StatusBadGateway)
		_, _ = c.Write([]byte(`{"error":"edge backend unavailable"}`))
	}

	router.GET("/health", func(c *gin.Context) {
		sqlDB, err := db.DB()
		dbOK := err == nil && sqlDB.Ping() == nil
		c.JSON(http.StatusOK, gin.H{
			"status":        statusText(dbOK),
			"role":          "main_server",
			"database_ok":   dbOK,
			"edge_base_url": cfg.Edge.BaseURL,
			"time":          time.Now().Format(time.RFC3339Nano),
		})
	})

	router.GET("/api/v1/main-server/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"role":                "main_server",
			"query_source":        "local_mysql",
			"edge_control_target": cfg.Edge.BaseURL,
			"query_proxy_enabled": cfg.Edge.QueryProxyEnabled,
		})
	})

	router.Any("/api/v1/edge-proxy/*path", func(c *gin.Context) {
		proxyToEdge(c, edgeProxy, edgeURL, strings.TrimPrefix(c.Param("path"), "/"))
	})

	router.NoRoute(func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if isWriteMethod(c.Request.Method) || cfg.Edge.QueryProxyEnabled {
			proxyToEdge(c, edgeProxy, edgeURL, path)
			return
		}
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":       "main server local query route is not implemented yet",
			"path":        c.Request.URL.Path,
			"next_action": "port the matching edge read handler to main-server/backend/internal/query",
		})
	})

	return router
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func proxyToEdge(c *gin.Context, proxy *httputil.ReverseProxy, edgeURL *url.URL, path string) {
	c.Request.URL.Scheme = edgeURL.Scheme
	c.Request.URL.Host = edgeURL.Host
	c.Request.URL.Path = "/" + strings.TrimPrefix(path, "/")
	c.Request.Host = edgeURL.Host
	proxy.ServeHTTP(c.Writer, c.Request)
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
