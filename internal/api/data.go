package api

import (
	"NowhereDash/internal/endpoint"
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"NowhereDash/internal/sse"
	"NowhereDash/internal/tunnel"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const endpointExportVersion = "3.0"

type DataHandler struct {
	db              *gorm.DB
	sseManager      *sse.Manager
	endpointService *endpoint.Service
}

func NewDataHandler(db *gorm.DB, manager *sse.Manager, endpointService *endpoint.Service, _ *tunnel.Service) *DataHandler {
	return &DataHandler{db: db, sseManager: manager, endpointService: endpointService}
}

func SetupDataRoutes(rg *gin.RouterGroup, db *gorm.DB, manager *sse.Manager, endpointService *endpoint.Service, tunnelService *tunnel.Service) {
	handler := NewDataHandler(db, manager, endpointService, tunnelService)
	rg.GET("/data/export", handler.HandleExport)
	rg.POST("/data/import", handler.HandleImport)
	rg.POST("/data/validate-import", handler.HandleValidateImport)
	rg.POST("/data/batch-import", handler.HandleBatchImportEndpoints)
}

type EndpointExport struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	APIPath string `json:"apiPath"`
	APIKey  string `json:"apiKey"`
}

type endpointExportDocument struct {
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		Endpoints []EndpointExport `json:"endpoints"`
	} `json:"data"`
}

func (h *DataHandler) HandleExport(c *gin.Context) {
	endpoints, err := h.endpointService.GetEndpoints()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	document := endpointExportDocument{Version: endpointExportVersion, Timestamp: time.Now().Format(time.RFC3339)}
	document.Data.Endpoints = make([]EndpointExport, 0, len(endpoints))
	for _, item := range endpoints {
		document.Data.Endpoints = append(document.Data.Endpoints, EndpointExport{Name: item.Name, URL: item.URL, APIPath: item.APIPath, APIKey: item.APIKey})
	}
	c.Header("Content-Disposition", "attachment; filename=nowheredash-endpoints.json")
	c.JSON(http.StatusOK, document)
}

func (h *DataHandler) HandleImport(c *gin.Context) {
	var document endpointExportDocument
	if err := c.ShouldBindJSON(&document); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid JSON"})
		return
	}
	if document.Version != endpointExportVersion {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Only NowhereDash endpoint export version 3.0 is supported"})
		return
	}
	imported, skipped, created, err := h.importEndpoints(document.Data.Endpoints)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	h.connectEndpoints(created)
	c.JSON(http.StatusOK, gin.H{"success": true, "importedEndpoints": imported, "skippedEndpoints": skipped})
}

type ValidateImportResult struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	APIPath   string `json:"apiPath"`
	Version   string `json:"version"`
	CanImport bool   `json:"canImport"`
	Message   string `json:"message"`
	Status    string `json:"status"`
}

func (h *DataHandler) HandleValidateImport(c *gin.Context) {
	var document endpointExportDocument
	if err := c.ShouldBindJSON(&document); err != nil || document.Version != endpointExportVersion {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid NowhereDash endpoint export"})
		return
	}
	results := make([]ValidateImportResult, 0, len(document.Data.Endpoints))
	for index, item := range document.Data.Endpoints {
		item.URL = strings.TrimRight(strings.TrimSpace(item.URL), "/")
		item.APIPath = nowhere.NormalizeAPIPath(item.APIPath)
		result := ValidateImportResult{Name: item.Name, URL: item.URL, APIPath: item.APIPath, Version: "unknown", Status: "error"}
		temporaryID := int64(-1000 - index)
		nowhere.GetCache().Set(fmt.Sprintf("%d", temporaryID), nowhere.BuildAPIBaseURL(item.URL, item.APIPath), item.APIKey)
		info, err := nowhere.GetInfo(temporaryID)
		nowhere.GetCache().Delete(fmt.Sprintf("%d", temporaryID))
		if err != nil {
			result.Message = err.Error()
		} else {
			result.Version = info.Ver
			result.CanImport = true
			result.Status = "success"
			result.Message = "OpenCtrl endpoint is reachable"
		}
		results = append(results, result)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "results": results, "total": len(results)})
}

type BatchImportEndpoint = EndpointExport

func (h *DataHandler) HandleBatchImportEndpoints(c *gin.Context) {
	var request struct {
		Endpoints []EndpointExport `json:"endpoints"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Endpoints) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "No endpoints provided"})
		return
	}
	imported, skipped, created, err := h.importEndpoints(request.Endpoints)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	h.connectEndpoints(created)
	c.JSON(http.StatusOK, gin.H{"success": true, "importedEndpoints": imported, "skippedEndpoints": skipped})
}

type importedEndpoint struct {
	ID      int64
	URL     string
	APIPath string
	APIKey  string
}

func (h *DataHandler) importEndpoints(items []EndpointExport) (int, int, []importedEndpoint, error) {
	imported := 0
	skipped := 0
	created := make([]importedEndpoint, 0, len(items))
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			item.URL = strings.TrimRight(strings.TrimSpace(item.URL), "/")
			item.APIPath = nowhere.NormalizeAPIPath(item.APIPath)
			var count int64
			if err := tx.Model(&models.Endpoint{}).Where("url = ? AND api_path = ?", item.URL, item.APIPath).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				skipped++
				continue
			}
			row := models.Endpoint{
				Name: strings.TrimSpace(item.Name), URL: strings.TrimRight(item.URL, "/"), Hostname: endpointHostname(item.URL),
				APIPath: item.APIPath, APIKey: item.APIKey, Status: models.EndpointStatusOffline, LastCheck: time.Now(),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			nowhere.GetCache().Set(fmt.Sprintf("%d", row.ID), nowhere.BuildAPIBaseURL(row.URL, row.APIPath), row.APIKey)
			created = append(created, importedEndpoint{ID: row.ID, URL: row.URL, APIPath: row.APIPath, APIKey: row.APIKey})
			imported++
		}
		return nil
	})
	return imported, skipped, created, err
}

func (h *DataHandler) connectEndpoints(items []importedEndpoint) {
	if h.sseManager == nil {
		return
	}
	for _, item := range items {
		item := item
		go func() { _ = h.sseManager.ConnectEndpoint(item.ID, item.URL, item.APIPath, item.APIKey) }()
	}
}

func endpointHostname(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
