package api

import (
	"NowhereDash/internal/subscription"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SubscriptionHandler struct {
	service *subscription.Service
}

func NewSubscriptionHandler(service *subscription.Service) *SubscriptionHandler {
	return &SubscriptionHandler{service: service}
}

func SetupSubscriptionRoutes(rg *gin.RouterGroup, service *subscription.Service) {
	handler := NewSubscriptionHandler(service)
	subscriptions := rg.Group("/subscriptions")
	subscriptions.Use(subscriptionManagementHeaders())
	subscriptions.GET("", handler.HandleList)
	subscriptions.POST("", handler.HandleCreate)
	subscriptions.GET("/:id", handler.HandleGet)
	subscriptions.PUT("/:id", handler.HandleUpdate)
	subscriptions.DELETE("/:id", handler.HandleDelete)
	subscriptions.POST("/:id/token/rotate", handler.HandleRotateToken)
	subscriptions.POST("/:id/traffic/reset", handler.HandleResetTraffic)
	subscriptions.GET("/:id/preview", handler.HandlePreview)
}

func SetupPublicSubscriptionRoutes(r *gin.Engine, service *subscription.Service) {
	handler := NewSubscriptionHandler(service)
	r.GET("/sub/portal", handler.HandlePublicPortal)
}

func (h *SubscriptionHandler) HandleList(c *gin.Context) {
	response, err := h.service.List()
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) HandleCreate(c *gin.Context) {
	var request subscription.UpsertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	response, err := h.service.Create(request)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response)
}

func (h *SubscriptionHandler) HandleGet(c *gin.Context) {
	id, ok := subscriptionID(c)
	if !ok {
		return
	}
	response, err := h.service.Get(id)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) HandleUpdate(c *gin.Context) {
	id, ok := subscriptionID(c)
	if !ok {
		return
	}
	var request subscription.UpsertRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	response, err := h.service.Update(id, request)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) HandleDelete(c *gin.Context) {
	id, ok := subscriptionID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(id); err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SubscriptionHandler) HandleRotateToken(c *gin.Context) {
	id, ok := subscriptionID(c)
	if !ok {
		return
	}
	response, err := h.service.RotateToken(id)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) HandleResetTraffic(c *gin.Context) {
	id, ok := subscriptionID(c)
	if !ok {
		return
	}
	response, err := h.service.ResetTraffic(id)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) HandlePreview(c *gin.Context) {
	id, ok := subscriptionID(c)
	if !ok {
		return
	}
	response, err := h.service.Preview(id)
	if err != nil {
		subscriptionError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SubscriptionHandler) HandlePublicPortal(c *gin.Context) {
	setSubscriptionSecurityHeaders(c)
	rendered, err := h.service.RenderPublic(c.Query("token"))
	if err != nil {
		switch {
		case errors.Is(err, subscription.ErrNotFound):
			c.Data(http.StatusNotFound, "text/plain; charset=utf-8", []byte("Subscription not found\n"))
		case errors.Is(err, subscription.ErrEntitlementUnavailable):
			c.Data(http.StatusForbidden, "text/plain; charset=utf-8", []byte("Subscription entitlement is unavailable\n"))
		default:
			c.Data(http.StatusInternalServerError, "text/plain; charset=utf-8", []byte("Internal server error\n"))
		}
		return
	}
	for key, value := range rendered.Headers {
		c.Header(key, value)
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(rendered.Content))
}

func subscriptionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription ID"})
		return 0, false
	}
	return id, true
}

func subscriptionError(c *gin.Context, err error) {
	if errors.Is(err, subscription.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": subscription.ErrNotFound.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func setSubscriptionSecurityHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
}

func subscriptionManagementHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		setSubscriptionSecurityHeaders(c)
		c.Next()
	}
}
