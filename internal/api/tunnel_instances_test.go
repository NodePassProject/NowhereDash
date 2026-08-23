package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"NowhereDash/internal/nowhere"

	"github.com/gin-gonic/gin"
)

func TestGetInstancesUsesTunnelResponseEnvelope(t *testing.T) {
	const (
		endpointID = int64(91001)
		apiKey     = "test-key"
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/instances" {
			t.Errorf("upstream path = %q, want /instances", r.URL.Path)
		}
		if got := r.Header.Get("X-API-Key"); got != apiKey {
			t.Errorf("X-API-Key = %q, want %q", got, apiKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"portal-1","type":"portal","status":"running","url":"portal://secret@:2077","alias":"Portal One"},
			{"id":"legacy-1","type":"unsupported","status":"running","url":"unsupported://legacy"}
		]`))
	}))
	defer upstream.Close()

	cacheKey := strconv.FormatInt(endpointID, 10)
	nowhere.GetCache().Set(cacheKey, upstream.URL, apiKey)
	t.Cleanup(func() { nowhere.GetCache().Delete(cacheKey) })

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: cacheKey}}

	NewTunnelHandler(nil, nil).HandleGetInstances(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("instances status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Success bool                     `json:"success"`
		Data    []nowhere.InstanceResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode instances response: %v", err)
	}
	if !response.Success {
		t.Fatal("instances response success = false, want true")
	}
	if len(response.Data) != 1 || response.Data[0].ID != "portal-1" {
		t.Fatalf("instances data = %#v, want only portal-1", response.Data)
	}
}
