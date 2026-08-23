package dashboard

import (
	"encoding/json"
	"errors"
	"fmt"

	"NowhereDash/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const cleanupConfigKey = "history_cleanup_config_v1"

// LoadCleanupConfig loads the persisted retention policy, falling back to the
// built-in defaults until an administrator saves an explicit configuration.
func LoadCleanupConfig(db *gorm.DB) (*CleanupConfig, error) {
	config := DefaultCleanupConfig()

	var rows []models.SystemConfig
	if err := db.Where("key = ?", cleanupConfigKey).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load history cleanup config: %w", err)
	}
	if len(rows) == 0 {
		return config, nil
	}

	if err := json.Unmarshal([]byte(rows[0].Value), config); err != nil {
		return nil, fmt.Errorf("decode history cleanup config: %w", err)
	}
	if err := ValidateCleanupConfig(config); err != nil {
		return nil, fmt.Errorf("validate history cleanup config: %w", err)
	}

	return config, nil
}

// SaveCleanupConfig persists the complete policy atomically for both SQLite
// and PostgreSQL.
func SaveCleanupConfig(db *gorm.DB, config *CleanupConfig) error {
	if err := ValidateCleanupConfig(config); err != nil {
		return err
	}

	value, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode history cleanup config: %w", err)
	}

	description := "Database history retention policy (v1)"
	row := models.SystemConfig{
		Key:         cleanupConfigKey,
		Value:       string(value),
		Description: &description,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "description", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("save history cleanup config: %w", err)
	}

	return nil
}

// ValidateCleanupConfig protects the chart recovery window and keeps manual
// configuration within practical bounds.
func ValidateCleanupConfig(config *CleanupConfig) error {
	if config == nil {
		return errors.New("history cleanup config is required")
	}
	if config.ServiceHistoryRetentionDays < 2 || config.ServiceHistoryRetentionDays > 30 {
		return errors.New("service history retention must be between 2 and 30 days")
	}
	if config.SummaryDataRetentionDays < 8 || config.SummaryDataRetentionDays > 3650 {
		return errors.New("hourly summary retention must be between 8 and 3650 days")
	}
	if config.DashboardSummaryRetentionDays < 1 || config.DashboardSummaryRetentionDays > 3650 {
		return errors.New("dashboard summary retention must be between 1 and 3650 days")
	}
	if config.OperationLogRetentionDays < 1 || config.OperationLogRetentionDays > 3650 {
		return errors.New("operation log retention must be between 1 and 3650 days")
	}
	if config.SSEDataRetentionDays < 1 || config.SSEDataRetentionDays > 3650 {
		return errors.New("legacy SSE retention must be between 1 and 3650 days")
	}
	if config.BatchSize < 1 || config.BatchSize > 20000 {
		return errors.New("cleanup batch size must be between 1 and 20000")
	}
	return nil
}
