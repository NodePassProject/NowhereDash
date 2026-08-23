package router

import (
	"NowhereDash/internal/api"
	"NowhereDash/internal/auth"
	"NowhereDash/internal/middleware"
	"NowhereDash/internal/models"
	"NowhereDash/internal/subscription"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRedactTokenInPathNeverRetainsToken(t *testing.T) {
	secret := strings.Repeat("A", 43)
	for _, path := range []string{
		"/sub/portal?token=" + secret + "&type=anywhere",
		"/sub/portal?type=anywhere&token=" + secret,
		"/sub/portal?token=" + secret + "%zz",
	} {
		redacted := redactTokenInPath(path)
		if strings.Contains(redacted, secret) || !strings.Contains(redacted, "REDACTED") {
			t.Fatalf("redactTokenInPath(%q) = %q", path, redacted)
		}
	}
}

func TestSubscriptionRouteAuthenticationAndSecurityHeaders(t *testing.T) {
	dsn := fmt.Sprintf("file:router-subscription-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.SystemConfig{}, &models.UserSession{}, &models.Endpoint{}, &models.Tunnel{},
		&models.PortalSubscription{}, &models.PortalSubscriptionTunnel{},
	); err != nil {
		t.Fatal(err)
	}
	service := subscription.NewService(db)
	created, err := service.Create(subscription.UpsertRequest{Name: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(subscriptionNoStoreMiddleware())
	api.SetupPublicSubscriptionRoutes(r, service)
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware(auth.NewService(db)))
	api.SetupSubscriptionRoutes(protected, service)

	management := httptest.NewRecorder()
	r.ServeHTTP(management, httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil))
	if management.Code != http.StatusUnauthorized {
		t.Fatalf("management status = %d, want 401", management.Code)
	}
	assertNoStoreHeaders(t, management)

	missing := httptest.NewRecorder()
	r.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/sub/portal?token=bad", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing public status = %d, want 404", missing.Code)
	}
	assertNoStoreHeaders(t, missing)

	empty := httptest.NewRecorder()
	r.ServeHTTP(empty, httptest.NewRequest(http.MethodGet, created.SubscriptionURL, nil))
	if empty.Code != http.StatusNotFound {
		t.Fatalf("empty public status = %d, want 404", empty.Code)
	}
	assertNoStoreHeaders(t, empty)

	endpoint := models.Endpoint{Name: "master", URL: "master://portal.example:10101", Hostname: "portal.example", APIPath: "/api/v2", APIKey: "key"}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	key, network := "secret", "tcp"
	portal := models.Tunnel{
		Name: "portal", EndpointID: endpoint.ID, Type: models.TunnelTypePortal,
		Status: models.TunnelStatusRunning, ListenHost: "*", ListenPort: "20001",
		CommandLine: "portal://secret@:20001", SharedKey: &key, Network: &network,
	}
	if err := db.Create(&portal).Error; err != nil {
		t.Fatal(err)
	}
	active, err := service.Create(subscription.UpsertRequest{Name: "active", TunnelIDs: []int64{portal.ID}})
	if err != nil {
		t.Fatal(err)
	}
	success := httptest.NewRecorder()
	r.ServeHTTP(success, httptest.NewRequest(http.MethodGet, active.SubscriptionURL, nil))
	if success.Code != http.StatusOK {
		t.Fatalf("active public status = %d, body=%q", success.Code, success.Body.String())
	}
	mediaType, parameters, err := mime.ParseMediaType(success.Header().Get("Content-Type"))
	if err != nil || mediaType != "text/plain" || strings.ToLower(parameters["charset"]) != "utf-8" {
		t.Fatalf("Content-Type = %q, parsed=%q %#v err=%v", success.Header().Get("Content-Type"), mediaType, parameters, err)
	}
	if !strings.HasSuffix(success.Body.String(), "\n") || !strings.HasPrefix(success.Body.String(), "nowhere://") || strings.Contains(success.Body.String(), "vector://") {
		t.Fatalf("unexpected public body: %q", success.Body.String())
	}
	assertNoStoreHeaders(t, success)

}

func assertNoStoreHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}
