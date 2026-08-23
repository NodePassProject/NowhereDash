package api

import (
	"NowhereDash/internal/metrics"
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"NowhereDash/internal/sse"
	"NowhereDash/internal/tunnel"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TunnelHandler struct {
	tunnelService *tunnel.Service
	sseManager    *sse.Manager
}

func NewTunnelHandler(tunnelService *tunnel.Service, sseManager *sse.Manager) *TunnelHandler {
	return &TunnelHandler{tunnelService: tunnelService, sseManager: sseManager}
}

func SetupTunnelRoutes(rg *gin.RouterGroup, tunnelService *tunnel.Service, sseManager *sse.Manager, sseProcessor *metrics.SSEProcessor) {
	handler := NewTunnelHandler(tunnelService, sseManager)
	metricsHandler := NewTunnelMetricsHandler(tunnelService, sseProcessor)

	rg.GET("/endpoints/:id/instances", handler.HandleGetInstances)
	rg.GET("/endpoints/:id/instances/:instanceId", handler.HandleGetInstance)
	rg.POST("/endpoints/:id/instances/:instanceId/control", handler.HandleControlInstance)
	rg.GET("/endpoints/:id/backup-instances", handler.HandleBackupInstances)
	rg.POST("/endpoints/:id/import-instances", handler.HandleImportInstances)

	rg.GET("/tunnels", handler.HandleGetTunnels)
	rg.POST("/tunnels", handler.HandleCreateTunnel)
	rg.POST("/tunnels/create_by_url", handler.HandleCreateTunnelByURL)
	rg.GET("/tunnels/:id", handler.HandleGetTunnel)
	rg.GET("/tunnels/:id/details", handler.HandleGetTunnelDetails)
	rg.PUT("/tunnels/:id", handler.HandleUpdateTunnel)
	rg.PATCH("/tunnels/:id", handler.HandlePatchTunnel)
	rg.DELETE("/tunnels/:id", handler.HandleDeleteTunnel)
	rg.PATCH("/tunnels/:id/status", handler.HandleControlTunnel)
	rg.POST("/tunnels/:id/action", handler.HandleControlTunnel)
	rg.PATCH("/tunnels/:id/restart", handler.HandleSetTunnelRestart)
	rg.PUT("/tunnels/:id/tags", handler.HandleUpdateInstanceTags)
	rg.POST("/tunnels/sorts", handler.HandleUpdateTunnelsSorts)
	rg.GET("/tunnels/:id/metrics-trend", metricsHandler.HandleGetTunnelMetricsTrend)

	rg.GET("/dashboard/operate_logs", handler.HandleGetTunnelLogs)
	rg.DELETE("/dashboard/operate_logs", handler.HandleClearTunnelLogs)
}

func errorResponse(c *gin.Context, status int, err error) {
	c.JSON(status, tunnel.TunnelResponse{Success: false, Error: err.Error()})
}

func (h *TunnelHandler) HandleGetTunnels(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	result, err := h.tunnelService.GetTunnelsWithPagination(tunnel.TunnelQueryParams{
		Search: c.Query("search"), Status: c.Query("status"), EndpointID: c.Query("endpoint_id"),
		PortFilter: c.Query("port_filter"), GroupID: c.Query("group_id"), Page: page, PageSize: pageSize,
		SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order"),
	})
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}
	if result.Data == nil {
		result.Data = []tunnel.TunnelWithStats{}
	}
	c.JSON(http.StatusOK, result)
}

func (h *TunnelHandler) HandleCreateTunnel(c *gin.Context) {
	var request tunnel.PortalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	created, err := h.tunnelService.CreatePortal(request)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, tunnel.TunnelResponse{Success: true, Message: "Tunnel instance created", Tunnel: created})
}

func (h *TunnelHandler) HandleCreateTunnelByURL(c *gin.Context) {
	var request struct {
		EndpointID int64  `json:"endpointId" binding:"required"`
		URL        string `json:"url" binding:"required"`
		Name       string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	created, err := h.tunnelService.CreatePortalURL(request.EndpointID, request.URL, request.Name)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, tunnel.TunnelResponse{Success: true, Message: "Tunnel instance created", Tunnel: created})
}

func (h *TunnelHandler) HandleGetTunnel(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	result, err := h.tunnelService.GetTunnel(id)
	if err != nil {
		status := http.StatusInternalServerError
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
		}
		errorResponse(c, status, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *TunnelHandler) HandleGetTunnelDetails(c *gin.Context) {
	var result models.Tunnel
	query := h.tunnelService.GormDB().Preload("Endpoint").Preload("Groups").Where("type = ?", models.TunnelTypePortal)
	if id, err := strconv.ParseInt(c.Param("id"), 10, 64); err == nil {
		query = query.Where("id = ?", id)
	} else {
		query = query.Where("instance_id = ?", c.Param("id"))
	}
	if err := query.First(&result).Error; err != nil {
		errorResponse(c, http.StatusNotFound, err)
		return
	}
	portalHost := result.Endpoint.Hostname
	if portalHost == "" {
		if endpointURL, parseErr := url.Parse(result.Endpoint.URL); parseErr == nil {
			portalHost = endpointURL.Hostname()
		}
	}
	configURL := nowhere.MatchingPortalConfigURL(result.CommandLine, result.ConfigLine)
	effectiveTunnel := result
	nowhere.ApplyInstanceConfig(&effectiveTunnel, nowhere.InstanceResult{
		URL:    result.CommandLine,
		Config: result.ConfigLine,
	})
	vectorURL, _ := nowhere.BuildVectorURL(effectiveTunnel, portalHost, "127.0.0.1:1080")
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"tunnel":     effectiveTunnel,
		"commandURL": result.CommandLine,
		"configURL":  configURL,
		"config":     nowhere.TunnelConfigFromTunnel(&effectiveTunnel),
		"endpoint":   result.Endpoint,
		"portalHost": portalHost,
		"vectorUrl":  vectorURL,
	})
}

func (h *TunnelHandler) HandleUpdateTunnel(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	var request tunnel.PortalRequest
	if err = c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	updated, err := h.tunnelService.UpdatePortal(id, request)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true, Message: "Tunnel instance updated", Tunnel: updated})
}

func (h *TunnelHandler) HandleDeleteTunnel(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	if err = h.tunnelService.DeleteTunnel(id); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true, Message: "Tunnel instance deleted"})
}

func (h *TunnelHandler) HandleControlTunnel(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	var request struct {
		Action string `json:"action" binding:"required,oneof=start stop restart"`
	}
	if err = c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	instanceID, err := h.tunnelService.GetInstanceIDByTunnelID(id)
	if err == nil {
		err = h.tunnelService.ControlTunnel(tunnel.TunnelActionRequest{InstanceID: instanceID, Action: request.Action})
	}
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true, Message: "Tunnel instance action completed"})
}

func (h *TunnelHandler) HandlePatchTunnel(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	var request struct {
		Action  string `json:"action" binding:"required"`
		Name    string `json:"name"`
		Sorts   int64  `json:"sorts"`
		Restart bool   `json:"restart"`
	}
	if err = c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	switch request.Action {
	case "rename":
		err = h.tunnelService.RenameTunnel(id, request.Name)
	case "updateSort":
		err = h.tunnelService.UpdateTunnelsSorts(id, request.Sorts)
	case "reset":
		var instanceID string
		instanceID, err = h.tunnelService.GetInstanceIDByTunnelID(id)
		if err == nil {
			err = h.tunnelService.ResetTunnelTrafficByInstanceID(instanceID)
		}
	default:
		err = strconv.ErrSyntax
	}
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true})
}

func (h *TunnelHandler) HandleSetTunnelRestart(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	var request struct {
		Restart bool `json:"restart"`
	}
	if err = c.ShouldBindJSON(&request); err == nil {
		err = h.tunnelService.SetTunnelRestart(id, request.Restart)
	}
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true})
}

func (h *TunnelHandler) HandleUpdateInstanceTags(c *gin.Context) {
	id, err := tunnel.ParseID(c.Param("id"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	var tags map[string]string
	if err = c.ShouldBindJSON(&tags); err == nil {
		err = h.tunnelService.UpdateTags(id, tags)
	}
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true, Message: "Metadata tags updated"})
}

func (h *TunnelHandler) HandleUpdateTunnelsSorts(c *gin.Context) {
	var request tunnel.UpdateTunnelsSortsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	for _, item := range request.Tunnels {
		if err := h.tunnelService.UpdateTunnelsSorts(item.ID, item.Sorts); err != nil {
			errorResponse(c, http.StatusBadRequest, err)
			return
		}
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true})
}

func (h *TunnelHandler) HandleGetInstances(c *gin.Context) {
	endpointID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	instances, err := nowhere.GetInstances(endpointID)
	if err != nil {
		errorResponse(c, http.StatusBadGateway, err)
		return
	}
	portalInstances := make([]nowhere.InstanceResult, 0, len(instances))
	for _, instance := range instances {
		if instance.Type == string(models.TunnelTypePortal) {
			portalInstances = append(portalInstances, instance)
		}
	}
	c.JSON(http.StatusOK, tunnel.TunnelResponse{Success: true, Data: portalInstances})
}

func (h *TunnelHandler) HandleGetInstance(c *gin.Context) {
	endpointID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	instance, err := nowhere.GetInstance(endpointID, c.Param("instanceId"))
	if err != nil {
		errorResponse(c, http.StatusBadGateway, err)
		return
	}
	if instance.Type != string(models.TunnelTypePortal) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Tunnel instance not found"})
		return
	}
	c.JSON(http.StatusOK, instance)
}

func (h *TunnelHandler) HandleControlInstance(c *gin.Context) {
	endpointID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	instance, err := nowhere.GetInstance(endpointID, c.Param("instanceId"))
	if err != nil {
		errorResponse(c, http.StatusBadGateway, err)
		return
	}
	if instance.Type != string(models.TunnelTypePortal) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Tunnel instance not found"})
		return
	}
	var request struct {
		Action string `json:"action" binding:"required,oneof=start stop restart"`
	}
	if err = c.ShouldBindJSON(&request); err != nil {
		errorResponse(c, http.StatusBadRequest, err)
		return
	}
	result, err := nowhere.ControlInstance(endpointID, c.Param("instanceId"), request.Action)
	if err != nil {
		errorResponse(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *TunnelHandler) HandleGetTunnelLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	logs, err := h.tunnelService.GetOperationLogs(limit)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, logs)
}

func (h *TunnelHandler) HandleClearTunnelLogs(c *gin.Context) {
	deleted, err := h.tunnelService.ClearOperationLogs()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": deleted})
}

func cleanName(name string) string { return strings.TrimSpace(name) }
