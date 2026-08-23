package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"NowhereDash/internal/models"
	tunnelservice "NowhereDash/internal/tunnel"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestTunnelDetailsUsesExpandedConfigURL(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&models.Endpoint{}, &models.Group{}, &models.Tunnel{}, &models.TunnelGroup{}); err != nil {
		t.Fatalf("migrate details models: %v", err)
	}
	endpoint := models.Endpoint{
		Name: "master", URL: "master://portal.example:10101", APIPath: "/api/v2", APIKey: "key",
	}
	if err := database.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	commandURL := "portal://runtime@:2077?net=tcp"
	configURL := "portal://:2077?net=tcp&tls=1&alpn=now%2F1&rate=0&etar=0&dial=auto&socks=none&next=none"
	portal := models.Tunnel{
		Name: "portal", EndpointID: endpoint.ID, Type: models.TunnelTypePortal,
		Status: models.TunnelStatusRunning, ListenPort: "2077", CommandLine: commandURL,
		ConfigLine: &configURL,
	}
	if err := database.Create(&portal).Error; err != nil {
		t.Fatalf("create Portal: %v", err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: fmt.Sprint(portal.ID)}}
	NewTunnelHandler(tunnelservice.NewService(database), nil).HandleGetTunnelDetails(context)

	if recorder.Code != 200 {
		t.Fatalf("details status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		CommandURL string                 `json:"commandURL"`
		ConfigURL  string                 `json:"configURL"`
		Config     map[string]interface{} `json:"config"`
		VectorURL  string                 `json:"vectorUrl"`
		Tunnel     models.Tunnel          `json:"tunnel"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode details response: %v", err)
	}
	if response.CommandURL != commandURL || response.ConfigURL != configURL {
		t.Fatalf("details URLs = command:%q config:%q", response.CommandURL, response.ConfigURL)
	}
	if response.Config["network"] != "tcp" || response.Config["sharedKey"] != "runtime" || response.Config["rate"] != "0" {
		t.Fatalf("details config did not use expanded URL: %#v", response.Config)
	}
	if response.Tunnel.Network == nil || *response.Tunnel.Network != "tcp" || response.Tunnel.SharedKey == nil ||
		*response.Tunnel.SharedKey != "runtime" || response.Tunnel.Rate == nil || *response.Tunnel.Rate != 0 ||
		response.Tunnel.ListenPort != "2077" {
		t.Fatalf("details tunnel did not merge expanded config: %+v", response.Tunnel)
	}
	if _, exists := response.Config["Network"]; exists {
		t.Fatalf("details config leaked Go field names: %#v", response.Config)
	}
	vectorURL, err := url.Parse(response.VectorURL)
	if err != nil {
		t.Fatalf("parse Vector URL: %v", err)
	}
	if vectorURL.Scheme != "nowhere" || vectorURL.User == nil || vectorURL.User.Username() != "runtime" || vectorURL.Host != "portal.example:2077" ||
		vectorURL.Query().Get("up") != "tcp" || vectorURL.Query().Get("down") != "tcp" {
		t.Fatalf("Vector URL did not use expanded config: %s", response.VectorURL)
	}
}
