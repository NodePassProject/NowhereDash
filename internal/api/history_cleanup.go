package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"NowhereDash/internal/dashboard"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const historyCleanupScheduleTime = "03:15"

type HistoryCleanupHandler struct {
	db             *gorm.DB
	cleanupService *dashboard.CleanupService
}

type historyCleanupConfigDTO struct {
	AutoCleanupEnabled          bool   `json:"autoCleanupEnabled"`
	ServiceHistoryRetentionDays int    `json:"serviceHistoryRetentionDays"`
	SummaryRetentionDays        int    `json:"summaryRetentionDays"`
	DashboardRetentionDays      int    `json:"dashboardRetentionDays"`
	OperationLogRetentionDays   int    `json:"operationLogRetentionDays"`
	BatchSize                   int    `json:"batchSize"`
	ScheduleTime                string `json:"scheduleTime"`
}

type updateHistoryCleanupConfigRequest struct {
	AutoCleanupEnabled          *bool `json:"autoCleanupEnabled"`
	ServiceHistoryRetentionDays *int  `json:"serviceHistoryRetentionDays"`
	SummaryRetentionDays        *int  `json:"summaryRetentionDays"`
	DashboardRetentionDays      *int  `json:"dashboardRetentionDays"`
	OperationLogRetentionDays   *int  `json:"operationLogRetentionDays"`
}

type historyCleanupTableStat struct {
	TableName     string     `json:"tableName"`
	TotalCount    int64      `json:"totalCount"`
	ExpiredCount  int64      `json:"expiredCount"`
	OldestRecord  *time.Time `json:"oldestRecord"`
	RetentionDays int        `json:"retentionDays"`
}

type historyCleanupStatsDTO struct {
	Driver            string                    `json:"driver"`
	DatabaseSizeBytes int64                     `json:"databaseSizeBytes"`
	ReusableBytes     int64                     `json:"reusableBytes"`
	IsRunning         bool                      `json:"isRunning"`
	LastCleanupTime   *time.Time                `json:"lastCleanupTime"`
	LastError         string                    `json:"lastError"`
	Tables            []historyCleanupTableStat `json:"tables"`
}

type historyCleanupStatusDTO struct {
	IsRunning       bool       `json:"isRunning"`
	LastCleanupTime *time.Time `json:"lastCleanupTime"`
	LastError       string     `json:"lastError"`
}

func SetupHistoryCleanupRoutes(rg *gin.RouterGroup, db *gorm.DB, cleanupService *dashboard.CleanupService) {
	handler := &HistoryCleanupHandler{db: db, cleanupService: cleanupService}
	rg.GET("/history-cleanup/config", handler.HandleGetConfig)
	rg.PUT("/history-cleanup/config", handler.HandleUpdateConfig)
	rg.GET("/history-cleanup/status", handler.HandleGetStatus)
	rg.GET("/history-cleanup/stats", handler.HandleGetStats)
	rg.POST("/history-cleanup/preview", handler.HandlePreview)
	rg.POST("/history-cleanup/trigger", handler.HandleTrigger)
}

func (h *HistoryCleanupHandler) HandleGetConfig(c *gin.Context) {
	config := h.cleanupService.ConfigSnapshot()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cleanupConfigDTO(config)})
}

func (h *HistoryCleanupHandler) HandleUpdateConfig(c *gin.Context) {
	var request updateHistoryCleanupConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid_request"})
		return
	}

	config := h.cleanupService.ConfigSnapshot()
	if request.AutoCleanupEnabled != nil {
		config.AutoCleanupEnabled = *request.AutoCleanupEnabled
	}
	if request.ServiceHistoryRetentionDays != nil {
		config.ServiceHistoryRetentionDays = *request.ServiceHistoryRetentionDays
	}
	if request.SummaryRetentionDays != nil {
		config.SummaryDataRetentionDays = *request.SummaryRetentionDays
	}
	if request.DashboardRetentionDays != nil {
		config.DashboardSummaryRetentionDays = *request.DashboardRetentionDays
	}
	if request.OperationLogRetentionDays != nil {
		config.OperationLogRetentionDays = *request.OperationLogRetentionDays
	}

	if err := dashboard.ValidateCleanupConfig(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	if err := dashboard.SaveCleanupConfig(h.db, &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "save_cleanup_config_failed"})
		return
	}
	if err := h.cleanupService.UpdateConfig(&config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "apply_cleanup_config_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": cleanupConfigDTO(config)})
}

func (h *HistoryCleanupHandler) HandleGetStats(c *gin.Context) {
	h.writeStats(c)
}

func (h *HistoryCleanupHandler) HandleGetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cleanupStatusDTO(h.cleanupService.RuntimeStatus()),
	})
}

func (h *HistoryCleanupHandler) HandlePreview(c *gin.Context) {
	h.writeStats(c)
}

func (h *HistoryCleanupHandler) HandleTrigger(c *gin.Context) {
	if err := h.cleanupService.StartFullCleanup(); err != nil {
		if errors.Is(err, dashboard.ErrCleanupAlreadyRunning) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "cleanup_in_progress"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "history cleanup started",
		"data":    gin.H{"started": true},
	})
}

func (h *HistoryCleanupHandler) writeStats(c *gin.Context) {
	stats, err := h.collectStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

func (h *HistoryCleanupHandler) collectStats(ctx context.Context) (*historyCleanupStatsDTO, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	db := h.db.WithContext(ctx)
	config := h.cleanupService.ConfigSnapshot()
	existingTableNames, err := db.Migrator().GetTables()
	if err != nil {
		return nil, fmt.Errorf("list history cleanup tables: %w", err)
	}
	existingTables := make(map[string]struct{}, len(existingTableNames))
	for _, tableName := range existingTableNames {
		existingTables[tableName] = struct{}{}
	}

	policies := []struct {
		table         string
		timeColumn    string
		retentionDays int
	}{
		{table: "endpoint_sse", timeColumn: "event_time", retentionDays: config.SSEDataRetentionDays},
		{table: "service_history", timeColumn: "record_time", retentionDays: config.ServiceHistoryRetentionDays},
		{table: "traffic_hourly_summary", timeColumn: "hour_time", retentionDays: config.SummaryDataRetentionDays},
		{table: "dashboard_traffic_summary", timeColumn: "hour_time", retentionDays: config.DashboardSummaryRetentionDays},
		{table: "tunnel_operation_logs", timeColumn: "created_at", retentionDays: config.OperationLogRetentionDays},
	}

	tables := make([]historyCleanupTableStat, 0, len(policies))
	for _, policy := range policies {
		stat := historyCleanupTableStat{TableName: policy.table, RetentionDays: policy.retentionDays}
		if _, exists := existingTables[policy.table]; exists {
			if err := db.Table(policy.table).Count(&stat.TotalCount).Error; err != nil {
				return nil, fmt.Errorf("count %s: %w", policy.table, err)
			}
			cutoff := time.Now().UTC().AddDate(0, 0, -policy.retentionDays)
			if err := db.Table(policy.table).Where(policy.timeColumn+" < ?", cutoff).Count(&stat.ExpiredCount).Error; err != nil {
				return nil, fmt.Errorf("count expired %s: %w", policy.table, err)
			}

			if stat.TotalCount > 0 {
				var oldest struct {
					Value time.Time `gorm:"column:value"`
				}
				if err := db.Table(policy.table).
					Select(policy.timeColumn + " AS value").
					Order(policy.timeColumn + " ASC").
					Limit(1).
					Scan(&oldest).Error; err != nil {
					return nil, fmt.Errorf("find oldest %s: %w", policy.table, err)
				}
				stat.OldestRecord = &oldest.Value
			}
		}
		tables = append(tables, stat)
	}

	databaseSize, reusable, err := h.databaseSpaceStats(ctx)
	if err != nil {
		return nil, err
	}
	runtime := cleanupStatusDTO(h.cleanupService.RuntimeStatus())

	return &historyCleanupStatsDTO{
		Driver:            h.db.Dialector.Name(),
		DatabaseSizeBytes: databaseSize,
		ReusableBytes:     reusable,
		IsRunning:         runtime.IsRunning,
		LastCleanupTime:   runtime.LastCleanupTime,
		LastError:         runtime.LastError,
		Tables:            tables,
	}, nil
}

func (h *HistoryCleanupHandler) databaseSpaceStats(ctx context.Context) (int64, int64, error) {
	db := h.db.WithContext(ctx)
	switch h.db.Dialector.Name() {
	case "sqlite":
		var pageCount, pageSize, freePages int64
		if err := db.Raw("PRAGMA page_count").Scan(&pageCount).Error; err != nil {
			return 0, 0, fmt.Errorf("read SQLite page count: %w", err)
		}
		if err := db.Raw("PRAGMA page_size").Scan(&pageSize).Error; err != nil {
			return 0, 0, fmt.Errorf("read SQLite page size: %w", err)
		}
		if err := db.Raw("PRAGMA freelist_count").Scan(&freePages).Error; err != nil {
			return 0, 0, fmt.Errorf("read SQLite free pages: %w", err)
		}
		databaseSize := pageCount * pageSize
		var databases []struct {
			Name string `gorm:"column:name"`
			File string `gorm:"column:file"`
		}
		if err := db.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
			return 0, 0, fmt.Errorf("read SQLite database path: %w", err)
		}
		for _, database := range databases {
			if database.Name != "main" || database.File == "" {
				continue
			}
			if info, statErr := os.Stat(database.File); statErr == nil {
				databaseSize = info.Size()
			} else if !os.IsNotExist(statErr) {
				return 0, 0, fmt.Errorf("read SQLite database file size: %w", statErr)
			}
			if info, statErr := os.Stat(database.File + "-wal"); statErr == nil {
				databaseSize += info.Size()
			} else if !os.IsNotExist(statErr) {
				return 0, 0, fmt.Errorf("read SQLite WAL file size: %w", statErr)
			}
			break
		}
		return databaseSize, freePages * pageSize, nil
	case "postgres":
		var size int64
		if err := db.Raw("SELECT pg_database_size(current_database())").Scan(&size).Error; err != nil {
			return 0, 0, fmt.Errorf("read PostgreSQL database size: %w", err)
		}
		return size, 0, nil
	default:
		return 0, 0, nil
	}
}

func cleanupStatusDTO(runtime dashboard.CleanupRuntimeStatus) historyCleanupStatusDTO {
	var lastCleanup *time.Time
	if !runtime.LastCleanupTime.IsZero() {
		value := runtime.LastCleanupTime
		lastCleanup = &value
	}

	return historyCleanupStatusDTO{
		IsRunning:       runtime.IsRunning,
		LastCleanupTime: lastCleanup,
		LastError:       runtime.LastError,
	}
}

func cleanupConfigDTO(config dashboard.CleanupConfig) historyCleanupConfigDTO {
	return historyCleanupConfigDTO{
		AutoCleanupEnabled:          config.AutoCleanupEnabled,
		ServiceHistoryRetentionDays: config.ServiceHistoryRetentionDays,
		SummaryRetentionDays:        config.SummaryDataRetentionDays,
		DashboardRetentionDays:      config.DashboardSummaryRetentionDays,
		OperationLogRetentionDays:   config.OperationLogRetentionDays,
		BatchSize:                   config.BatchSize,
		ScheduleTime:                historyCleanupScheduleTime,
	}
}
