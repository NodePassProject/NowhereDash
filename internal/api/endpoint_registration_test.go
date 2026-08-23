package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"NowhereDash/internal/endpoint"
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInstallerRegistrationFlow(t *testing.T) {
	database := openEndpointRegistrationTestDB(t)
	nowhere.GetCache().Clear()
	handler := NewEndpointRegistrationHandler(
		endpoint.NewService(database),
		nil,
		endpoint.NewRegistrationTokenStore(10*time.Minute),
	)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/endpoints/registration-token", handler.HandleIssueToken)
	router.POST("/api/endpoints/register", handler.HandleRegisterEndpoint)

	issue := performJSONRequest(t, router, http.MethodPost, "/api/endpoints/registration-token", map[string]string{
		"name": "guided-edge",
	})
	if issue.Code != http.StatusOK {
		t.Fatalf("issue status = %d, body=%q", issue.Code, issue.Body.String())
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(issue.Body.Bytes(), &issued); err != nil || issued.Token == "" {
		t.Fatalf("invalid issue response: body=%q err=%v", issue.Body.String(), err)
	}

	registration := map[string]string{
		"token":    issued.Token,
		"apiUrl":   "https://edge.example.com:8080/api/v2",
		"apiKey":   "openctrl-secret",
		"hostname": "edge.example.com",
	}
	registered := performJSONRequest(t, router, http.MethodPost, "/api/endpoints/register", registration)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body=%q", registered.Code, registered.Body.String())
	}

	var saved models.Endpoint
	if err := database.Where("name = ?", "guided-edge").First(&saved).Error; err != nil {
		t.Fatalf("load registered endpoint: %v", err)
	}
	if saved.URL != "https://edge.example.com:8080" || saved.APIPath != "/api/v2" || saved.APIKey != "openctrl-secret" {
		t.Fatalf("unexpected registered endpoint: %+v", saved)
	}

	replayed := performJSONRequest(t, router, http.MethodPost, "/api/endpoints/register", registration)
	if replayed.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, body=%q", replayed.Code, replayed.Body.String())
	}
}

func TestInstallerRegistrationTokenSurvivesCreateFailure(t *testing.T) {
	database := openEndpointRegistrationTestDB(t)
	nowhere.GetCache().Clear()
	service := endpoint.NewService(database)
	if _, err := service.CreateEndpoint(endpoint.CreateEndpointRequest{
		Name: "duplicate", URL: "https://existing.example.com", APIPath: "/api/v2", APIKey: "key",
	}); err != nil {
		t.Fatal(err)
	}
	store := endpoint.NewRegistrationTokenStore(10 * time.Minute)
	token, _, err := store.Issue("duplicate")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewEndpointRegistrationHandler(service, nil, store)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/endpoints/register", handler.HandleRegisterEndpoint)
	body := map[string]string{
		"token": token, "apiUrl": "https://new.example.com/api/v2", "apiKey": "new-key",
	}

	failed := performJSONRequest(t, router, http.MethodPost, "/api/endpoints/register", body)
	if failed.Code != http.StatusBadRequest {
		t.Fatalf("first register status = %d, body=%q", failed.Code, failed.Body.String())
	}
	if err := database.Where("name = ?", "duplicate").Delete(&models.Endpoint{}).Error; err != nil {
		t.Fatal(err)
	}
	retried := performJSONRequest(t, router, http.MethodPost, "/api/endpoints/register", body)
	if retried.Code != http.StatusCreated {
		t.Fatalf("retry status = %d, body=%q", retried.Code, retried.Body.String())
	}
}

func TestIssueRegistrationTokenRejectsExistingName(t *testing.T) {
	database := openEndpointRegistrationTestDB(t)
	service := endpoint.NewService(database)
	if _, err := service.CreateEndpoint(endpoint.CreateEndpointRequest{
		Name: "existing", URL: "https://existing-name.example.com", APIPath: "/api/v2", APIKey: "key",
	}); err != nil {
		t.Fatal(err)
	}
	handler := NewEndpointRegistrationHandler(
		service,
		nil,
		endpoint.NewRegistrationTokenStore(10*time.Minute),
	)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/endpoints/registration-token", handler.HandleIssueToken)

	response := performJSONRequest(t, router, http.MethodPost, "/api/endpoints/registration-token", map[string]string{
		"name": "existing",
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("issue status = %d, body=%q", response.Code, response.Body.String())
	}
}

func openEndpointRegistrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.Endpoint{}); err != nil {
		t.Fatal(err)
	}
	return database
}

func performJSONRequest(t *testing.T, handler http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
