package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"NowhereDash/internal/models"

	"gorm.io/gorm"
)

// TrafficService 流量服务
type TrafficService struct {
	db *gorm.DB
}

func isSQLiteLocked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "busy")
}

// NewTrafficService 创建流量服务实例
func NewTrafficService(db *gorm.DB) *TrafficService {
	return &TrafficService{db: db}
}

// normalizeHourStart keeps persisted/query boundaries identical across
// SQLite and PostgreSQL. SQLite otherwise compares mixed-offset datetime
// strings lexicographically (for example +00:00 versus +08:00).
func normalizeHourStart(t time.Time) time.Time {
	return t.UTC().Truncate(time.Hour)
}

// AggregateTrafficData 聚合当前小时的流量数据
func (s *TrafficService) AggregateTrafficData() error {
	// Only aggregate completed hours. All database boundaries are UTC.
	lastHour := normalizeHourStart(time.Now()).Add(-time.Hour)
	return s.AggregateTrafficDataForHour(lastHour)
}

// AggregateTrafficDataForHour 为指定小时聚合流量数据
// 从service_history表获取上一小时59分的累计值，并计算与上一小时的差值
func (s *TrafficService) AggregateTrafficDataForHour(hourStart time.Time) error {
	hourStart = normalizeHourStart(hourStart)
	// 小时窗口结束时间
	hourEnd := hourStart.Add(1 * time.Hour)

	// 使用事务来确保数据一致性
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. UPSERT 当前小时窗口的聚合记录。
		// SQLite 3.24+ 与 PostgreSQL 都支持 ON CONFLICT DO UPDATE,
		// 唯一索引由 ensureOptimizedIndexes 保证存在。
		if err := tx.Exec(`
			INSERT INTO traffic_hourly_summary (
				hour_time,
				instance_id,
				endpoint_id,
				tcp_rx_total,
				tcp_tx_total,
				udp_rx_total,
				udp_tx_total,
				tcp_rx_increment,
				tcp_tx_increment,
				udp_rx_increment,
				udp_tx_increment,
				record_count,
				created_at,
				updated_at
			)
			SELECT
				?,
				sh.instance_id,
				sh.endpoint_id,
				sh.delta_tcp_in as tcp_rx_total,
				sh.delta_tcp_out as tcp_tx_total,
				sh.delta_udp_in as udp_rx_total,
				sh.delta_udp_out as udp_tx_total,
				sh.delta_tcp_in as tcp_rx_increment,
				sh.delta_tcp_out as tcp_tx_increment,
				sh.delta_udp_in as udp_rx_increment,
				sh.delta_udp_out as udp_tx_increment,
				1 as record_count,
				CURRENT_TIMESTAMP,
				CURRENT_TIMESTAMP
			FROM (
				SELECT
					sh.*,
					ROW_NUMBER() OVER (
						PARTITION BY sh.endpoint_id, sh.instance_id
						ORDER BY sh.record_time DESC, sh.id DESC
					) AS row_num
				FROM service_history sh
				WHERE sh.record_time >= ? AND sh.record_time < ?
			) sh
			WHERE sh.row_num = 1
			ON CONFLICT(hour_time, endpoint_id, instance_id) DO UPDATE SET
				tcp_rx_total = excluded.tcp_rx_total,
				tcp_tx_total = excluded.tcp_tx_total,
				udp_rx_total = excluded.udp_rx_total,
				udp_tx_total = excluded.udp_tx_total,
				tcp_rx_increment = excluded.tcp_rx_increment,
				tcp_tx_increment = excluded.tcp_tx_increment,
				udp_rx_increment = excluded.udp_rx_increment,
				udp_tx_increment = excluded.udp_tx_increment,
				record_count = excluded.record_count,
				updated_at = CURRENT_TIMESTAMP`,
			hourStart, hourStart, hourEnd).Error; err != nil {
			return fmt.Errorf("插入汇总数据失败: %v", err)
		}

		// 1.1 对于该小时窗口内没有任何 service_history 记录的实例:从上一小时 carry forward,
		// 避免"某小时缺行导致曲线断点/实例数抖动",同时 increment 在下一步自动算为 0。
		previousHour := hourStart.Add(-1 * time.Hour)
		if tx.Migrator().HasTable("tunnels") {
			if err := tx.Exec(`
				INSERT INTO traffic_hourly_summary (
					hour_time,
					instance_id,
					endpoint_id,
					tcp_rx_total,
					tcp_tx_total,
					udp_rx_total,
					udp_tx_total,
					tcp_rx_increment,
					tcp_tx_increment,
					udp_rx_increment,
					udp_tx_increment,
					record_count,
					created_at,
					updated_at
				)
				SELECT
					?,
					prev.instance_id,
					prev.endpoint_id,
					prev.tcp_rx_total,
					prev.tcp_tx_total,
					prev.udp_rx_total,
					prev.udp_tx_total,
					0,
					0,
					0,
					0,
					0,
					CURRENT_TIMESTAMP,
					CURRENT_TIMESTAMP
				FROM traffic_hourly_summary prev
				WHERE prev.hour_time = ?
					AND EXISTS (
						SELECT 1 FROM tunnels t
						WHERE t.endpoint_id = prev.endpoint_id
							AND t.instance_id = prev.instance_id
					)
					AND NOT EXISTS (
						SELECT 1 FROM traffic_hourly_summary cur
						WHERE cur.hour_time = ?
							AND cur.endpoint_id = prev.endpoint_id
							AND cur.instance_id = prev.instance_id
					)
				ON CONFLICT(hour_time, endpoint_id, instance_id) DO UPDATE SET
					tcp_rx_total = excluded.tcp_rx_total,
					tcp_tx_total = excluded.tcp_tx_total,
					udp_rx_total = excluded.udp_rx_total,
					udp_tx_total = excluded.udp_tx_total,
					tcp_rx_increment = excluded.tcp_rx_increment,
					tcp_tx_increment = excluded.tcp_tx_increment,
					udp_rx_increment = excluded.udp_rx_increment,
					udp_tx_increment = excluded.udp_tx_increment,
					record_count = excluded.record_count,
					updated_at = CURRENT_TIMESTAMP
			`, hourStart, previousHour, hourStart).Error; err != nil {
				return fmt.Errorf("carry-forward 数据失败: %v", err)
			}
		}

		// 2. 计算与上一小时的差值(increment 字段)
		if err := s.calculateIncrements(tx, hourStart); err != nil {
			return fmt.Errorf("计算增量失败: %v", err)
		}

		// 3. 执行 dashboard 汇总
		if err := s.aggregateDashboardTraffic(tx, hourStart); err != nil {
			return fmt.Errorf("dashboard汇总失败: %v", err)
		}

		return nil
	})
}

// calculateIncrements 计算与上一小时的差值
func (s *TrafficService) calculateIncrements(tx *gorm.DB, hourStart time.Time) error {
	previousHour := hourStart.Add(-1 * time.Hour)
	hourEnd := hourStart.Add(time.Hour)

	var currentRows []models.TrafficHourlySummary
	if err := tx.Where("hour_time = ?", hourStart).Find(&currentRows).Error; err != nil {
		return fmt.Errorf("查询当前小时汇总失败: %v", err)
	}
	if len(currentRows) == 0 {
		return nil
	}

	type trafficKey struct {
		EndpointID int64
		InstanceID string
	}
	previousByInstance := make(map[trafficKey]models.TrafficHourlySummary)
	var previousRows []models.TrafficHourlySummary
	if err := tx.Where("hour_time = ?", previousHour).Find(&previousRows).Error; err != nil {
		return fmt.Errorf("查询上一小时汇总失败: %v", err)
	}
	for _, row := range previousRows {
		previousByInstance[trafficKey{row.EndpointID, row.InstanceID}] = row
	}

	type firstSnapshot struct {
		EndpointID int64  `gorm:"column:endpoint_id"`
		InstanceID string `gorm:"column:instance_id"`
		TCPRx      int64  `gorm:"column:tcp_rx"`
		TCPTx      int64  `gorm:"column:tcp_tx"`
		UDPRx      int64  `gorm:"column:udp_rx"`
		UDPTx      int64  `gorm:"column:udp_tx"`
	}
	var firstSnapshots []firstSnapshot
	if err := tx.Raw(`
		SELECT
			sh.endpoint_id,
			sh.instance_id,
			sh.delta_tcp_in AS tcp_rx,
			sh.delta_tcp_out AS tcp_tx,
			sh.delta_udp_in AS udp_rx,
			sh.delta_udp_out AS udp_tx
		FROM service_history sh
		INNER JOIN (
			SELECT endpoint_id, instance_id, MIN(record_time) AS min_record_time
			FROM service_history
			WHERE record_time >= ? AND record_time < ?
			GROUP BY endpoint_id, instance_id
		) first_record ON sh.endpoint_id = first_record.endpoint_id
			AND sh.instance_id = first_record.instance_id
			AND sh.record_time = first_record.min_record_time
	`, hourStart, hourEnd).Scan(&firstSnapshots).Error; err != nil {
		return fmt.Errorf("查询小时初始快照失败: %v", err)
	}
	firstByInstance := make(map[trafficKey]firstSnapshot)
	for _, row := range firstSnapshots {
		firstByInstance[trafficKey{row.EndpointID, row.InstanceID}] = row
	}

	trafficDelta := func(current, baseline int64) int64 {
		if current >= baseline {
			return current - baseline
		}
		// A lower cumulative counter means the upstream instance reset.
		return current
	}

	for _, current := range currentRows {
		key := trafficKey{current.EndpointID, current.InstanceID}
		baselineTCPRx := current.TCPRxTotal
		baselineTCPTx := current.TCPTxTotal
		baselineUDPRx := current.UDPRxTotal
		baselineUDPTx := current.UDPTxTotal

		if previous, ok := previousByInstance[key]; ok {
			baselineTCPRx = previous.TCPRxTotal
			baselineTCPTx = previous.TCPTxTotal
			baselineUDPRx = previous.UDPRxTotal
			baselineUDPTx = previous.UDPTxTotal
		} else if first, ok := firstByInstance[key]; ok {
			baselineTCPRx = first.TCPRx
			baselineTCPTx = first.TCPTx
			baselineUDPRx = first.UDPRx
			baselineUDPTx = first.UDPTx
		}

		updates := map[string]interface{}{
			"tcp_rx_increment": trafficDelta(current.TCPRxTotal, baselineTCPRx),
			"tcp_tx_increment": trafficDelta(current.TCPTxTotal, baselineTCPTx),
			"udp_rx_increment": trafficDelta(current.UDPRxTotal, baselineUDPRx),
			"udp_tx_increment": trafficDelta(current.UDPTxTotal, baselineUDPTx),
		}
		if err := tx.Model(&models.TrafficHourlySummary{}).
			Where("id = ?", current.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("更新实例小时增量失败 [%d_%s]: %v", current.EndpointID, current.InstanceID, err)
		}
	}

	return nil
}

// aggregateDashboardTraffic 聚合 dashboard 流量数据。
// 使用 ON CONFLICT(hour_time) DO UPDATE,兼容 SQLite 3.24+ 与 PostgreSQL。
func (s *TrafficService) aggregateDashboardTraffic(tx *gorm.DB, hourStart time.Time) error {
	if err := tx.Exec(`
		INSERT INTO dashboard_traffic_summary (
			hour_time,
			tcp_rx_total,
			tcp_tx_total,
			udp_rx_total,
			udp_tx_total,
			instance_count,
			created_at,
			updated_at
		)
		SELECT
			?,
			COALESCE(SUM(tcp_rx_total), 0) as tcp_rx_total,
			COALESCE(SUM(tcp_tx_total), 0) as tcp_tx_total,
			COALESCE(SUM(udp_rx_total), 0) as udp_rx_total,
			COALESCE(SUM(udp_tx_total), 0) as udp_tx_total,
			COUNT(*) as instance_count,
			CURRENT_TIMESTAMP,
			CURRENT_TIMESTAMP
		FROM traffic_hourly_summary
		WHERE hour_time = ?
		HAVING COUNT(*) > 0
		ON CONFLICT(hour_time) DO UPDATE SET
			tcp_rx_total = excluded.tcp_rx_total,
			tcp_tx_total = excluded.tcp_tx_total,
			udp_rx_total = excluded.udp_rx_total,
			udp_tx_total = excluded.udp_tx_total,
			instance_count = excluded.instance_count,
			updated_at = CURRENT_TIMESTAMP`,
		hourStart, hourStart).Error; err != nil {
		return fmt.Errorf("插入dashboard汇总数据失败: %v", err)
	}

	// Remove stale rows created by older versions when an hour had no source
	// records. SUM over an empty set previously produced NULL traffic values.
	if err := tx.Exec(`
		DELETE FROM dashboard_traffic_summary
		WHERE hour_time = ?
			AND NOT EXISTS (
				SELECT 1 FROM traffic_hourly_summary
				WHERE hour_time = ?
			)`, hourStart, hourStart).Error; err != nil {
		return fmt.Errorf("清理空dashboard汇总数据失败: %v", err)
	}

	return nil
}

// InitializeRecentTrafficData 初始化最近24小时的流量汇总数据
// 支持更新处理：如果数据已存在则进行更新
func (s *TrafficService) InitializeRecentTrafficData() error {
	if err := s.cleanInvalidDashboardTraffic(); err != nil {
		return err
	}

	end := normalizeHourStart(time.Now())
	start := end.Add(-24 * time.Hour)

	var firstErr error
	for hour := start; hour.Before(end); hour = hour.Add(time.Hour) {
		// 分小时重试，避免单个小时 locked 导致整体初始化失败
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			err = s.initializeTrafficDataForHour(hour)
			if err == nil {
				break
			}
			if !isSQLiteLocked(err) {
				break
			}
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}

		if err != nil {
			// 继续初始化其它小时，最后返回首个错误，避免启动期“全失败”
			if firstErr == nil {
				firstErr = fmt.Errorf("初始化小时数据失败 %s: %v", hour.Format("2006-01-02 15:04"), err)
			}
		}

		// 轻微让出 CPU/锁，降低单核机器抖动
		time.Sleep(50 * time.Millisecond)
	}

	return firstErr
}

func (s *TrafficService) cleanInvalidDashboardTraffic() error {
	if err := s.db.Exec(`
		DELETE FROM dashboard_traffic_summary
		WHERE instance_count = 0
			OR tcp_rx_total IS NULL
			OR tcp_tx_total IS NULL
			OR udp_rx_total IS NULL
			OR udp_tx_total IS NULL
	`).Error; err != nil {
		return fmt.Errorf("清理无效dashboard流量数据失败: %v", err)
	}
	return nil
}

// initializeTrafficDataForHour 初始化指定小时的流量数据（支持更新处理）
func (s *TrafficService) initializeTrafficDataForHour(hourStart time.Time) error {
	return s.AggregateTrafficDataForHour(hourStart)
}

// CleanOldTrafficData 清理老旧的流量数据。
// Deprecated: use CleanupService so scheduled and manual cleanup share one implementation.
func (s *TrafficService) CleanOldTrafficData() error {
	config, err := LoadCleanupConfig(s.db)
	if err != nil {
		return err
	}

	cleanupService := NewCleanupService(s.db, config)
	defer cleanupService.Close()

	_, err = cleanupService.ExecuteFullCleanup()
	return err
}

// GetTrafficData 获取指定时间范围的流量数据（根据隧道实例ID）
func (s *TrafficService) GetTrafficData(instanceID string, start, end time.Time) ([]models.TrafficHourlySummary, error) {
	var data []models.TrafficHourlySummary

	err := s.db.Where("instance_id = ? AND hour_time >= ? AND hour_time < ?",
		instanceID, start, end).
		Order("hour_time ASC").
		Find(&data).Error

	if err != nil {
		return nil, fmt.Errorf("获取流量数据失败: %v", err)
	}

	return data, nil
}

// GetDashboardTrafficData 获取指定时间范围的dashboard流量数据
func (s *TrafficService) GetDashboardTrafficData(start, end time.Time) ([]models.DashboardTrafficSummary, error) {
	var data []models.DashboardTrafficSummary

	err := s.db.Where("hour_time >= ? AND hour_time < ?", start, end).
		Order("hour_time ASC").
		Find(&data).Error

	if err != nil {
		return nil, fmt.Errorf("获取dashboard流量数据失败: %v", err)
	}

	return data, nil
}

// GetTrafficTrendOptimized 获取优化后的流量趋势数据
func (s *TrafficService) GetTrafficTrendOptimized(hours int) ([]TrafficTrendItem, error) {
	end := time.Now()
	start := end.Add(-time.Duration(hours) * time.Hour)

	// 获取所有隧道的汇总数据
	var summaries []models.TrafficHourlySummary
	err := s.db.Where("hour_time >= ? AND hour_time < ?", start, end).
		Order("hour_time ASC").
		Find(&summaries).Error
	if err != nil {
		return nil, fmt.Errorf("获取流量趋势数据失败: %v", err)
	}

	// 按小时汇总所有隧道的流量
	hourlyTraffic := make(map[string]*TrafficTrendItem)
	for _, summary := range summaries {
		hourKey := summary.HourTime.Format("2006-01-02 15:00:00")
		if _, exists := hourlyTraffic[hourKey]; !exists {
			hourlyTraffic[hourKey] = &TrafficTrendItem{
				HourTime:    summary.HourTime.Unix(),
				HourDisplay: summary.HourTime.Format("15:04"),
				TCPRx:       0,
				TCPTx:       0,
				UDPRx:       0,
				UDPTx:       0,
				RecordCount: 0,
			}
		}

		item := hourlyTraffic[hourKey]
		item.TCPRx += summary.TCPRxIncrement
		item.TCPTx += summary.TCPTxIncrement
		item.UDPRx += summary.UDPRxIncrement
		item.UDPTx += summary.UDPTxIncrement
		item.RecordCount++
	}

	// 转换为切片并排序
	var result []TrafficTrendItem
	for _, item := range hourlyTraffic {
		result = append(result, *item)
	}

	// 按时间排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].HourTime < result[j].HourTime
	})

	// 确保返回空数组而不是nil
	if result == nil {
		result = []TrafficTrendItem{}
	}

	return result, nil
}

// GetLatestTrafficData 获取最新的流量数据（根据隧道实例ID）
func (s *TrafficService) GetLatestTrafficData(instanceID string) (*models.TrafficHourlySummary, error) {
	var data models.TrafficHourlySummary

	err := s.db.Where("instance_id = ?", instanceID).
		Order("hour_time DESC").
		First(&data).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("获取最新流量数据失败: %v", err)
	}

	return &data, nil
}

// TodayTrafficIncrement 今日流量增量(所有实例合计)
type TodayTrafficIncrement struct {
	TCPRx int64 `json:"tcpIn" gorm:"column:tcp_rx"`
	TCPTx int64 `json:"tcpOut" gorm:"column:tcp_tx"`
	UDPRx int64 `json:"udpIn" gorm:"column:udp_rx"`
	UDPTx int64 `json:"udpOut" gorm:"column:udp_tx"`
}

// Total 今日 TCP+UDP 双向合计
func (t TodayTrafficIncrement) Total() int64 {
	return t.TCPRx + t.TCPTx + t.UDPRx + t.UDPTx
}

// GetTodayTrafficIncrement 汇总当日(本地零点起)所有实例的每小时增量。
func (s *TrafficService) GetTodayTrafficIncrement() (TodayTrafficIncrement, error) {
	now := time.Now()
	todayLocal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayStart := todayLocal.UTC()
	tomorrowStart := todayLocal.AddDate(0, 0, 1).UTC()

	var result TodayTrafficIncrement
	err := s.db.Model(&models.TrafficHourlySummary{}).
		Select(`COALESCE(SUM(CASE WHEN tcp_rx_increment > 0 THEN tcp_rx_increment ELSE 0 END), 0) AS tcp_rx,
			COALESCE(SUM(CASE WHEN tcp_tx_increment > 0 THEN tcp_tx_increment ELSE 0 END), 0) AS tcp_tx,
			COALESCE(SUM(CASE WHEN udp_rx_increment > 0 THEN udp_rx_increment ELSE 0 END), 0) AS udp_rx,
			COALESCE(SUM(CASE WHEN udp_tx_increment > 0 THEN udp_tx_increment ELSE 0 END), 0) AS udp_tx`).
		Where("hour_time >= ? AND hour_time < ?", todayStart, tomorrowStart).
		Scan(&result).Error
	if err != nil {
		return TodayTrafficIncrement{}, fmt.Errorf("获取今日流量增量失败: %v", err)
	}
	return result, nil
}

// GetLatestDashboardTrafficData 获取最新的dashboard流量数据
func (s *TrafficService) GetLatestDashboardTrafficData() (*models.DashboardTrafficSummary, error) {
	var data models.DashboardTrafficSummary

	err := s.db.Order("hour_time DESC").
		First(&data).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("获取最新dashboard流量数据失败: %v", err)
	}

	return &data, nil
}
