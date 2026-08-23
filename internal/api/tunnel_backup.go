package api

import (
	"NowhereDash/internal/models"
	"NowhereDash/internal/tunnel"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const BackupVersion = 2

type BackupInstance struct {
	Name       string             `json:"name"`
	Type       string             `json:"type"`
	ListenHost string             `json:"listenHost"`
	ListenPort string             `json:"listenPort"`
	SharedKey  *string            `json:"sharedKey"`
	Network    *string            `json:"network"`
	TLSMode    string             `json:"tlsMode"`
	CertPath   *string            `json:"certPath,omitempty"`
	KeyPath    *string            `json:"keyPath,omitempty"`
	ALPN       *string            `json:"alpn"`
	Rate       *int64             `json:"rate"`
	Etar       *int64             `json:"etar"`
	Dial       *string            `json:"dial"`
	Socks      *string            `json:"socks"`
	Next       *string            `json:"next"`
	Up         *string            `json:"up"`
	Down       *string            `json:"down"`
	PoolSize   *int64             `json:"poolSize,omitempty"`
	Sni        *string            `json:"sni,omitempty"`
	Pin        *string            `json:"pin,omitempty"`
	LogLevel   string             `json:"logLevel"`
	Restart    *bool              `json:"restart,omitempty"`
	Tags       *map[string]string `json:"tags,omitempty"`
	Peer       *models.Peer       `json:"peer,omitempty"`
}

type BackupSource struct {
	EndpointID   int64   `json:"endpointId"`
	EndpointName string  `json:"endpointName"`
	EndpointURL  string  `json:"endpointUrl"`
	EndpointVer  *string `json:"endpointVer,omitempty"`
}

type BackupExport struct {
	Version    int              `json:"version"`
	ExportedAt time.Time        `json:"exportedAt"`
	Source     BackupSource     `json:"source"`
	Count      int              `json:"count"`
	Instances  []BackupInstance `json:"instances"`
}

type ImportResultItem struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	InstanceID string `json:"instanceId,omitempty"`
}

type ImportResponse struct {
	Success  bool               `json:"success"`
	Imported int                `json:"imported"`
	Skipped  int                `json:"skipped"`
	Failed   int                `json:"failed"`
	Total    int                `json:"total"`
	Results  []ImportResultItem `json:"results"`
}

func tunnelToBackupInstance(item models.Tunnel) BackupInstance {
	return BackupInstance{
		Name: item.Name, Type: string(models.TunnelTypePortal), ListenHost: item.ListenHost,
		ListenPort: item.ListenPort, SharedKey: item.SharedKey, Network: item.Network,
		TLSMode: string(item.TLSMode), CertPath: item.CertPath, KeyPath: item.KeyPath,
		ALPN: item.ALPN, Rate: item.Rate, Etar: item.Etar, Dial: item.Dial, Socks: item.Socks,
		Next: item.Next, Up: item.Up, Down: item.Down, PoolSize: item.PoolSize, Sni: item.Sni,
		Pin: item.Pin, LogLevel: string(item.LogLevel), Restart: item.Restart, Tags: item.Tags, Peer: item.Peer,
	}
}

func backupValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (item BackupInstance) portalRequest(endpointID int64) tunnel.PortalRequest {
	restart := false
	if item.Restart != nil {
		restart = *item.Restart
	}
	return tunnel.PortalRequest{
		Name: item.Name, EndpointID: endpointID, ListenHost: item.ListenHost, ListenPort: item.ListenPort,
		SharedKey: backupValue(item.SharedKey), Network: backupValue(item.Network), TLSMode: models.TLSMode(item.TLSMode),
		CertPath: backupValue(item.CertPath), KeyPath: backupValue(item.KeyPath), ALPN: backupValue(item.ALPN),
		Rate: item.Rate, Etar: item.Etar, Dial: backupValue(item.Dial), Socks: backupValue(item.Socks),
		Next: backupValue(item.Next), Up: backupValue(item.Up), Down: backupValue(item.Down), PoolSize: item.PoolSize,
		Sni: backupValue(item.Sni), Pin: backupValue(item.Pin), LogLevel: models.LogLevel(item.LogLevel),
		Restart: restart, Tags: item.Tags, Peer: item.Peer, EnableStore: true,
	}
}

func (h *TunnelHandler) HandleBackupInstances(c *gin.Context) {
	endpointID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid endpoint ID"})
		return
	}
	var endpoint models.Endpoint
	if err = h.tunnelService.GormDB().Select("id, name, url, api_path, ver").First(&endpoint, endpointID).Error; err != nil {
		status := http.StatusInternalServerError
		if err == gorm.ErrRecordNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	var rows []models.Tunnel
	if err = h.tunnelService.GormDB().Where("endpoint_id = ? AND type = ?", endpointID, models.TunnelTypePortal).Order("id ASC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	instances := make([]BackupInstance, 0, len(rows))
	for _, row := range rows {
		instances = append(instances, tunnelToBackupInstance(row))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": BackupExport{
		Version: BackupVersion, ExportedAt: time.Now(), Count: len(instances), Instances: instances,
		Source: BackupSource{EndpointID: endpoint.ID, EndpointName: endpoint.Name, EndpointURL: endpoint.URL + endpoint.APIPath, EndpointVer: endpoint.Ver},
	}})
}

func (h *TunnelHandler) HandleImportInstances(c *gin.Context) {
	endpointID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid endpoint ID"})
		return
	}
	var input BackupExport
	if err = c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if input.Version != BackupVersion {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Only NowhereDash backup version 2 is supported"})
		return
	}
	var endpointCount int64
	if err = h.tunnelService.GormDB().Model(&models.Endpoint{}).Where("id = ?", endpointID).Count(&endpointCount).Error; err != nil || endpointCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Endpoint not found"})
		return
	}

	var existingRows []struct{ Name string }
	_ = h.tunnelService.GormDB().Model(&models.Tunnel{}).Where("endpoint_id = ?", endpointID).Select("name").Find(&existingRows).Error
	existing := make(map[string]struct{}, len(existingRows))
	for _, row := range existingRows {
		existing[strings.TrimSpace(row.Name)] = struct{}{}
	}
	response := ImportResponse{Success: true, Total: len(input.Instances), Results: make([]ImportResultItem, 0, len(input.Instances))}
	for _, item := range input.Instances {
		name := strings.TrimSpace(item.Name)
		if item.Type != "" && item.Type != string(models.TunnelTypePortal) {
			response.Failed++
			response.Results = append(response.Results, ImportResultItem{Name: name, Status: "failed", Reason: "only portal instances are supported"})
			continue
		}
		if _, found := existing[name]; found && name != "" {
			response.Skipped++
			response.Results = append(response.Results, ImportResultItem{Name: name, Status: "skipped", Reason: "an instance with this name already exists"})
			continue
		}
		created, createErr := h.tunnelService.CreatePortal(item.portalRequest(endpointID))
		if createErr != nil {
			response.Failed++
			response.Results = append(response.Results, ImportResultItem{Name: name, Status: "failed", Reason: createErr.Error()})
			continue
		}
		response.Imported++
		existing[name] = struct{}{}
		instanceID := ""
		if created.InstanceID != nil {
			instanceID = *created.InstanceID
		}
		response.Results = append(response.Results, ImportResultItem{Name: name, Status: "success", InstanceID: instanceID})
	}
	response.Success = response.Failed == 0
	c.JSON(http.StatusOK, response)
}
