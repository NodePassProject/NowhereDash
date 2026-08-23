package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"NowhereDash/internal/endpoint"
	log "NowhereDash/internal/log"
	"NowhereDash/internal/sse"

	"github.com/gin-gonic/gin"
)

const endpointRegistrationTokenTTL = 10 * time.Minute

type EndpointRegistrationHandler struct {
	endpointService *endpoint.Service
	sseManager      *sse.Manager
	tokens          *endpoint.RegistrationTokenStore
}

func NewEndpointRegistrationHandler(
	endpointService *endpoint.Service,
	sseManager *sse.Manager,
	tokens *endpoint.RegistrationTokenStore,
) *EndpointRegistrationHandler {
	return &EndpointRegistrationHandler{
		endpointService: endpointService,
		sseManager:      sseManager,
		tokens:          tokens,
	}
}

// SetupEndpointRegistrationRoutes separates authenticated token issuance from
// the public, token-authenticated installer callback.
func SetupEndpointRegistrationRoutes(
	publicGroup *gin.RouterGroup,
	protectedGroup *gin.RouterGroup,
	endpointService *endpoint.Service,
	sseManager *sse.Manager,
) {
	handler := NewEndpointRegistrationHandler(
		endpointService,
		sseManager,
		endpoint.NewRegistrationTokenStore(endpointRegistrationTokenTTL),
	)
	protectedGroup.POST("/endpoints/registration-token", handler.HandleIssueToken)
	publicGroup.POST("/endpoints/register", handler.HandleRegisterEndpoint)
}

func (h *EndpointRegistrationHandler) HandleIssueToken(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	name := strings.TrimSpace(req.Name)
	exists, err := h.endpointService.EndpointNameExists(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate endpoint name"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "Endpoint name already exists"})
		return
	}

	token, claims, err := h.tokens.Issue(name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, endpoint.ErrTooManyRegistrationTokens) {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":     token,
		"expiresAt": claims.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *EndpointRegistrationHandler) HandleRegisterEndpoint(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req endpoint.InstallerRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	createReq, err := endpoint.BuildInstallerEndpointRequest(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var created *endpoint.Endpoint
	err = h.tokens.Use(req.Token, func(claims endpoint.RegistrationClaims) error {
		createReq.Name = strings.TrimSpace(claims.Name)
		var createErr error
		created, createErr = h.endpointService.CreateEndpoint(createReq)
		return createErr
	})
	if err != nil {
		if errors.Is(err, endpoint.ErrInvalidRegistrationToken) || errors.Is(err, endpoint.ErrRegistrationTokenBusy) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired registration token"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.sseManager != nil && created != nil {
		go func(ep *endpoint.Endpoint) {
			log.Infof("[Master-%v] 安装器注册成功，准备启动 SSE 监听", ep.ID)
			if connectErr := h.sseManager.ConnectEndpoint(ep.ID, ep.URL, ep.APIPath, ep.APIKey); connectErr != nil {
				log.Errorf("[Master-%v] 启动 SSE 监听失败: %v", ep.ID, connectErr)
			}
		}(created)
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":    true,
		"message":    "Endpoint registered successfully",
		"endpointId": created.ID,
	})
}
