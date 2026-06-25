package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"spindle-edge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

func TestRuntimeDraftsHandlerPutGetDelete(t *testing.T) {
	router := runtimeDraftTestRouter()

	body := `{"scope_type":"project","scope_id":"1","expected_revision":0,"data":{"standard_id":123,"items":[{"var_id":1,"var_name":"T1"}]}}`
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/runtime-drafts/station.detection_preload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("put status = %d body=%s", resp.Code, resp.Body.String())
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/runtime-drafts/station.detection_preload?scope_type=project&scope_id=1", nil)
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte(`"revision":1`)) {
		t.Fatalf("get body missing revision: %s", resp.Body.String())
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/runtime-drafts/station.detection_preload?scope_type=project&scope_id=1&expected_revision=1", nil)
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", resp.Code, resp.Body.String())
	}

	resp = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/runtime-drafts/station.detection_preload?scope_type=project&scope_id=1", nil)
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeDraftsHandlerRevisionConflict(t *testing.T) {
	router := runtimeDraftTestRouter()
	body := `{"scope_type":"project","scope_id":"1","expected_revision":0,"data":{"ok":true}}`
	for i := 0; i < 2; i++ {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/runtime-drafts/station.detection_preload", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(resp, req)
		if i == 0 && resp.Code != http.StatusOK {
			t.Fatalf("first put status = %d body=%s", resp.Code, resp.Body.String())
		}
		if i == 1 && resp.Code != http.StatusConflict {
			t.Fatalf("second put status = %d body=%s", resp.Code, resp.Body.String())
		}
	}
}

func runtimeDraftTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := services.NewRuntimeDraftService(nil)
	handler := NewRuntimeDraftsHandler(service)
	router := gin.New()
	router.GET("/runtime-drafts/:namespace", handler.get)
	router.PUT("/runtime-drafts/:namespace", handler.put)
	router.DELETE("/runtime-drafts/:namespace", handler.clear)
	return router
}
