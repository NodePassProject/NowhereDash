package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"NowhereDash/internal/endpoint"
	"NowhereDash/internal/models"
	"github.com/gin-gonic/gin"
)

func TestEndpointBillingCreateEditAndClear(t *testing.T) {
	database := openEndpointRegistrationTestDB(t)
	if err := database.AutoMigrate(&models.Tunnel{}, &models.EndpointTrafficCursor{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	SetupEndpointRoutes(router.Group("/api"), endpoint.NewService(database), nil)
	plan := map[string]interface{}{
		"price": 15.88, "currency": "CNY", "monthlyTrafficLimit": 1024,
		"trafficResetDate": "2027-02-09", "trafficResetDay": 9, "trafficResetTime": "00:00",
		"trafficResetTimezone": "Asia/Singapore", "autoResetTraffic": true,
	}
	created := performJSONRequest(t, router, http.MethodPost, "/api/endpoints", map[string]interface{}{
		"name": "billing", "url": "http://billing.test", "apiKey": "test", "billing": plan,
	})
	if created.Code != http.StatusOK {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var result struct {
		Endpoint models.Endpoint `json:"endpoint"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	id := result.Endpoint.ID
	if result.Endpoint.Price == nil || *result.Endpoint.Price != 15.88 || result.Endpoint.NextTrafficResetAt == nil || result.Endpoint.NextTrafficResetAt.Format("2006-01-02 15:04") != "2027-02-08 16:00" {
		t.Fatalf("created billing: %+v", result.Endpoint)
	}
	plan["price"] = 28.5
	updated := performJSONRequest(t, router, http.MethodPatch, "/api/endpoints", map[string]interface{}{
		"id": id, "action": "updateConfig", "billing": plan,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	var ep models.Endpoint
	if err := database.First(&ep, id).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Price == nil || *ep.Price != 28.5 {
		t.Fatal("PATCH dropped billing fields")
	}
	delete(plan, "price")
	delete(plan, "currency")
	for _, amount := range []string{"15.8800", "USD 20 / year", "0", ""} {
		plan["amount"] = amount
		response := performJSONRequest(t, router, http.MethodPatch, "/api/endpoints", map[string]interface{}{
			"id": id, "action": "updateConfig", "billing": plan,
		})
		if response.Code != http.StatusOK {
			t.Fatalf("save amount %q: %d %s", amount, response.Code, response.Body.String())
		}
		ep = models.Endpoint{}
		if err := database.First(&ep, id).Error; err != nil {
			t.Fatal(err)
		}
		if ep.Amount == nil || *ep.Amount != amount || ep.Price != nil {
			t.Fatalf("amount %q was not preserved: amount=%v, legacy price=%v", amount, ep.Amount, ep.Price)
		}
		if ep.NextTrafficResetAt == nil || ep.NextTrafficResetAt.Format("2006-01-02 15:04") != "2027-02-08 16:00" {
			t.Fatal("amount edit changed renewal date")
		}
	}
	invalid := performJSONRequest(t, router, http.MethodPatch, "/api/endpoints", map[string]interface{}{
		"id": id, "action": "updateConfig", "billing": map[string]interface{}{"price": -1},
	})
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid billing status = %d", invalid.Code)
	}
	plan["price"], plan["monthlyTrafficLimit"], plan["autoResetTraffic"] = nil, nil, false
	cleared := performJSONRequest(t, router, http.MethodPatch, "/api/endpoints", map[string]interface{}{
		"id": id, "action": "updateConfig", "billing": plan,
	})
	if cleared.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", cleared.Code, cleared.Body.String())
	}
	ep = models.Endpoint{}
	if err := database.First(&ep, id).Error; err != nil {
		t.Fatal(err)
	}
	if ep.Price != nil || ep.MonthlyTrafficLimit != nil || ep.NextTrafficResetAt != nil || ep.AutoResetTraffic {
		t.Fatalf("clear failed: %+v", ep)
	}
}
