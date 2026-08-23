package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

const cleanupBatchPause = 25 * time.Millisecond

var (
	// ErrCleanupAlreadyRunning is returned when another cleanup is already
	// running in this process, even if it was started by another service instance.
	ErrCleanupAlreadyRunning = errors.New("data cleanup is already running")
	cleanupExecutionMu       sync.Mutex
)

// CleanupConfig 数据清理配置
type CleanupConfig struct {
	// SSEDataRetentionDays is retained for compatibility with legacy endpoint_sse tables.
	SSEDataRetentionDays int `json:"sseDataRetentionDays,omitempty"`

	// ServiceHistoryRetentionDays 分钟级服务历史保留天数（默认7天）
	ServiceHistoryRetentionDays int `json:"serviceHistoryRetentionDays"`

	// SummaryDataRetentionDays 实例小时汇总保留天数（默认365天）
	SummaryDataRetentionDays int `json:"summaryDataRetentionDays"`

	// DashboardSummaryRetentionDays Dashboard小时汇总保留天数（默认365天）
	DashboardSummaryRetentionDays int `json:"dashboardSummaryRetentionDays"`

	// OperationLogRetentionDays 操作日志保留天数（默认90天）
	OperationLogRetentionDays int `json:"operationLogRetentionDays"`

	// BatchSize 每批次清理的记录数
	BatchSize int `json:"batchSize"`

	// AutoCleanupEnabled 是否启用自动清理；手动触发不受此开关影响
	AutoCleanupEnabled bool `json:"autoCleanupEnabled"`
}

// DefaultCleanupConfig 默认清理配置
func DefaultCleanupConfig() *CleanupConfig {
	return &CleanupConfig{
		SSEDataRetentionDays:          30,
		ServiceHistoryRetentionDays:   7,
		SummaryDataRetentionDays:      365,
		DashboardSummaryRetentionDays: 365,
		OperationLogRetentionDays:     90,
		BatchSize:                     1000,
		AutoCleanupEnabled:            true,
	}
}

// CleanupRuntimeStatus describes the lifecycle of the latest cleanup run.
type CleanupRuntimeStatus struct {
	IsRunning       bool      `json:"isRunning"`
	LastCleanupTime time.Time `json:"lastCleanupTime"`
	LastError       string    `json:"lastError"`
}

// CleanupService 数据清理服务
type CleanupService struct {
	db *gorm.DB

	configMu sync.RWMutex
	config   CleanupConfig

	runtimeMu sync.RWMutex
	runtime   CleanupRuntimeStatus

	lifecycleMu sync.Mutex
	runWG       sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewCleanupService 创建数据清理服务
func NewCleanupService(db *gorm.DB, config *CleanupConfig) *CleanupService {
	normalized := normalizeCleanupConfig(config)
	ctx, cancel := context.WithCancel(context.Background())

	return &CleanupService{
		db:     db,
		config: normalized,
		ctx:    ctx,
		cancel: cancel,
	}
}

func normalizeCleanupConfig(config *CleanupConfig) CleanupConfig {
	defaults := *DefaultCleanupConfig()
	if config == nil {
		return defaults
	}

	normalized := *config
	if normalized.SSEDataRetentionDays <= 0 {
		normalized.SSEDataRetentionDays = defaults.SSEDataRetentionDays
	}
	if normalized.ServiceHistoryRetentionDays <= 0 {
		normalized.ServiceHistoryRetentionDays = defaults.ServiceHistoryRetentionDays
	}
	if normalized.SummaryDataRetentionDays <= 0 {
		normalized.SummaryDataRetentionDays = defaults.SummaryDataRetentionDays
	}
	if normalized.DashboardSummaryRetentionDays <= 0 {
		normalized.DashboardSummaryRetentionDays = defaults.DashboardSummaryRetentionDays
	}
	if normalized.OperationLogRetentionDays <= 0 {
		normalized.OperationLogRetentionDays = defaults.OperationLogRetentionDays
	}
	if normalized.BatchSize <= 0 {
		normalized.BatchSize = defaults.BatchSize
	}
	return normalized
}

// CleanupResult 清理结果
type CleanupResult struct {
	TableName    string        `json:"tableName"`
	DeletedCount int64         `json:"deletedCount"`
	Duration     time.Duration `json:"duration"`
	Error        error         `json:"error,omitempty"`
}

type cleanupTableSpec struct {
	tableName  string
	timeColumn string
	cutoffTime time.Time
}

// ExecuteFullCleanup 执行完整的数据清理。
func (s *CleanupService) ExecuteFullCleanup() ([]CleanupResult, error) {
	return s.ExecuteFullCleanupContext(context.Background())
}

// ExecuteFullCleanupContext executes cleanup synchronously and stops between
// batches when the context is cancelled.
func (s *CleanupService) ExecuteFullCleanupContext(ctx context.Context) (results []CleanupResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.registerRun(); err != nil {
		return nil, err
	}
	defer s.runWG.Done()

	runCtx, cancel := context.WithCancel(ctx)
	stopLifecycleCancel := context.AfterFunc(s.ctx, cancel)
	defer func() {
		stopLifecycleCancel()
		cancel()
	}()
	if err := runCtx.Err(); err != nil {
		return nil, err
	}
	if err := s.beginCleanup(); err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("cleanup panic: %v", recovered)
		}
		s.finishCleanup(err)
	}()

	return s.executeFullCleanup(runCtx)
}

// StartFullCleanup atomically reserves the process-wide cleanup slot and then
// runs cleanup asynchronously using this service's lifecycle context.
func (s *CleanupService) StartFullCleanup() error {
	if err := s.registerRun(); err != nil {
		return err
	}
	if err := s.beginCleanup(); err != nil {
		s.runWG.Done()
		return err
	}

	go func() {
		var runErr error
		defer s.runWG.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("cleanup panic: %v", recovered)
				log.Printf("[数据清理] 异步任务异常: %v", runErr)
			}
			s.finishCleanup(runErr)
		}()
		_, runErr = s.executeFullCleanup(s.ctx)
	}()

	return nil
}

func (s *CleanupService) registerRun() error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()

	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.runWG.Add(1)
	return nil
}

func (s *CleanupService) beginCleanup() error {
	if !cleanupExecutionMu.TryLock() {
		return ErrCleanupAlreadyRunning
	}

	s.runtimeMu.Lock()
	s.runtime.IsRunning = true
	s.runtime.LastError = ""
	s.runtimeMu.Unlock()
	return nil
}

func (s *CleanupService) finishCleanup(err error) {
	s.runtimeMu.Lock()
	s.runtime.IsRunning = false
	s.runtime.LastCleanupTime = time.Now().UTC()
	if err != nil {
		s.runtime.LastError = err.Error()
	} else {
		s.runtime.LastError = ""
	}
	s.runtimeMu.Unlock()

	cleanupExecutionMu.Unlock()
}

func (s *CleanupService) executeFullCleanup(ctx context.Context) ([]CleanupResult, error) {
	log.Println("[数据清理] 开始执行完整数据清理...")

	existingTables, err := s.cleanupTableSet(ctx)
	if err != nil {
		return nil, err
	}

	config := s.ConfigSnapshot()
	now := time.Now().UTC()
	specs := []cleanupTableSpec{
		{tableName: "endpoint_sse", timeColumn: "event_time", cutoffTime: now.AddDate(0, 0, -config.SSEDataRetentionDays)},
		{tableName: "service_history", timeColumn: "record_time", cutoffTime: now.AddDate(0, 0, -config.ServiceHistoryRetentionDays)},
		{tableName: "traffic_hourly_summary", timeColumn: "hour_time", cutoffTime: now.AddDate(0, 0, -config.SummaryDataRetentionDays)},
		{tableName: "dashboard_traffic_summary", timeColumn: "hour_time", cutoffTime: now.AddDate(0, 0, -config.DashboardSummaryRetentionDays)},
		{tableName: "tunnel_operation_logs", timeColumn: "created_at", cutoffTime: now.AddDate(0, 0, -config.OperationLogRetentionDays)},
	}

	results := make([]CleanupResult, 0, len(specs))
	var cleanupErrors []error
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			cleanupErrors = append(cleanupErrors, err)
			break
		}
		if _, exists := existingTables[spec.tableName]; !exists {
			results = append(results, CleanupResult{TableName: spec.tableName})
			continue
		}

		result := s.cleanupTable(ctx, spec, config.BatchSize)
		results = append(results, result)
		if result.Error != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%s: %w", result.TableName, result.Error))
			log.Printf("[数据清理] %s 清理失败: %v", result.TableName, result.Error)
			if errors.Is(result.Error, context.Canceled) || errors.Is(result.Error, context.DeadlineExceeded) {
				break
			}
			continue
		}
		log.Printf("[数据清理] %s 清理完成: 删除 %d 条记录，耗时 %v", result.TableName, result.DeletedCount, result.Duration)
	}

	log.Println("[数据清理] 完整数据清理执行完毕")
	return results, errors.Join(cleanupErrors...)
}

func (s *CleanupService) cleanupTableSet(ctx context.Context) (map[string]struct{}, error) {
	tables, err := s.db.WithContext(ctx).Migrator().GetTables()
	if err != nil {
		return nil, fmt.Errorf("读取清理表清单失败: %w", err)
	}

	existing := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		existing[table] = struct{}{}
	}
	return existing, nil
}

func (s *CleanupService) cleanupTable(ctx context.Context, spec cleanupTableSpec, batchSize int) (result CleanupResult) {
	start := time.Now()
	result = CleanupResult{TableName: spec.tableName}
	defer func() { result.Duration = time.Since(start) }()

	if err := ctx.Err(); err != nil {
		result.Error = err
		return result
	}

	db := s.db.WithContext(ctx)
	statement := fmt.Sprintf(`
		DELETE FROM %s
		WHERE id IN (
			SELECT id FROM %s
			WHERE %s < ?
			ORDER BY %s ASC, id ASC
			LIMIT ?
		)`, spec.tableName, spec.tableName, spec.timeColumn, spec.timeColumn)

	for {
		if err := ctx.Err(); err != nil {
			result.Error = err
			return result
		}

		execResult := db.Exec(statement, spec.cutoffTime, batchSize)
		if execResult.Error != nil {
			result.Error = fmt.Errorf("批量删除失败: %w", execResult.Error)
			return result
		}

		deletedCount := execResult.RowsAffected
		result.DeletedCount += deletedCount
		if deletedCount < int64(batchSize) {
			return result
		}

		timer := time.NewTimer(cleanupBatchPause)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			result.Error = ctx.Err()
			return result
		case <-timer.C:
		}
	}
}

// GetCleanupStats 获取清理统计信息
func (s *CleanupService) GetCleanupStats() (map[string]interface{}, error) {
	config := s.ConfigSnapshot()
	now := time.Now().UTC()
	stats := make(map[string]interface{})
	ctx := context.Background()
	existingTables, err := s.cleanupTableSet(ctx)
	if err != nil {
		return nil, err
	}

	type statsSpec struct {
		prefix     string
		tableName  string
		timeColumn string
		cutoffTime time.Time
	}
	specs := []statsSpec{
		{prefix: "sse", tableName: "endpoint_sse", timeColumn: "event_time", cutoffTime: now.AddDate(0, 0, -config.SSEDataRetentionDays)},
		{prefix: "service_history", tableName: "service_history", timeColumn: "record_time", cutoffTime: now.AddDate(0, 0, -config.ServiceHistoryRetentionDays)},
		{prefix: "summary", tableName: "traffic_hourly_summary", timeColumn: "hour_time", cutoffTime: now.AddDate(0, 0, -config.SummaryDataRetentionDays)},
		{prefix: "dashboard_summary", tableName: "dashboard_traffic_summary", timeColumn: "hour_time", cutoffTime: now.AddDate(0, 0, -config.DashboardSummaryRetentionDays)},
		{prefix: "log", tableName: "tunnel_operation_logs", timeColumn: "created_at", cutoffTime: now.AddDate(0, 0, -config.OperationLogRetentionDays)},
	}

	for _, spec := range specs {
		if _, exists := existingTables[spec.tableName]; !exists {
			stats[spec.prefix+"_total_count"] = int64(0)
			stats[spec.prefix+"_cleanup_count"] = int64(0)
			continue
		}
		total, expired, err := s.tableCleanupStats(ctx, spec.tableName, spec.timeColumn, spec.cutoffTime)
		if err != nil {
			return nil, err
		}
		stats[spec.prefix+"_total_count"] = total
		stats[spec.prefix+"_cleanup_count"] = expired
	}
	stats["config"] = config

	return stats, nil
}

func (s *CleanupService) tableCleanupStats(ctx context.Context, tableName, timeColumn string, cutoff time.Time) (int64, int64, error) {
	db := s.db.WithContext(ctx)
	var total int64
	if err := db.Table(tableName).Count(&total).Error; err != nil {
		return 0, 0, fmt.Errorf("统计 %s 记录失败: %w", tableName, err)
	}

	var expired int64
	condition := fmt.Sprintf("%s < ?", timeColumn)
	if err := db.Table(tableName).Where(condition, cutoff).Count(&expired).Error; err != nil {
		return 0, 0, fmt.Errorf("统计 %s 过期记录失败: %w", tableName, err)
	}
	return total, expired, nil
}

// ConfigSnapshot returns a copy safe for concurrent readers.
func (s *CleanupService) ConfigSnapshot() CleanupConfig {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config
}

// UpdateConfig validates and atomically replaces the cleanup configuration.
func (s *CleanupService) UpdateConfig(config *CleanupConfig) error {
	if config == nil {
		return errors.New("cleanup config must not be nil")
	}

	updated := *config
	if updated.SSEDataRetentionDays <= 0 {
		updated.SSEDataRetentionDays = DefaultCleanupConfig().SSEDataRetentionDays
	}
	if err := ValidateCleanupConfig(&updated); err != nil {
		return err
	}

	s.configMu.Lock()
	s.config = updated
	s.configMu.Unlock()
	return nil
}

// RuntimeStatus returns a copy of the latest cleanup runtime state.
func (s *CleanupService) RuntimeStatus() CleanupRuntimeStatus {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.runtime
}

// Close cancels cleanup work and waits until registered runs have stopped.
func (s *CleanupService) Close() {
	s.lifecycleMu.Lock()
	s.cancel()
	s.lifecycleMu.Unlock()
	s.runWG.Wait()
}
