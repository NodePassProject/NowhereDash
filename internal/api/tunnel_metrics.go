package api

import (
	log "NowhereDash/internal/log"
	"NowhereDash/internal/metrics"
	"NowhereDash/internal/models"
	"NowhereDash/internal/tunnel"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// TunnelMetricsHandler 改进版的隧道指标处理器，基于 Nezha 的 avg_delay 机制
type TunnelMetricsHandler struct {
	tunnelService *tunnel.Service
	sseProcessor  *metrics.SSEProcessor
}

// NewTunnelMetricsHandler 创建隧道指标处理器
func NewTunnelMetricsHandler(tunnelService *tunnel.Service, sseProcessor *metrics.SSEProcessor) *TunnelMetricsHandler {
	return &TunnelMetricsHandler{
		tunnelService: tunnelService,
		sseProcessor:  sseProcessor,
	}
}

// HandleGetTunnelTrafficTrendV2 获取隧道流量趋势数据（改进版）
// GET /api/tunnels/{id}/traffic-trend
func (h *TunnelMetricsHandler) HandleGetTunnelTrafficTrendV2(c *gin.Context) {

	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少隧道ID"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的隧道ID"})
		return
	}

	// 解析小时数参数，默认24小时
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsedHours, err := strconv.Atoi(h); err == nil && parsedHours > 0 && parsedHours <= 168 { // 最多7天
			hours = parsedHours
		}
	}

	db := h.tunnelService.DB()

	// 查询隧道基本信息
	var endpointID int64
	var instanceID sql.NullString
	if err := db.QueryRow(h.tunnelService.Rebind(`SELECT endpoint_id, instance_id FROM tunnels WHERE id = ?`), id).Scan(&endpointID, &instanceID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "隧道不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var trafficTrend []map[string]interface{}

	if instanceID.Valid && instanceID.String != "" {
		// 使用新的聚合器获取分钟级平均流量数据
		trendData, err := h.sseProcessor.GetTrafficTrend(endpointID, instanceID.String, hours)
		if err != nil {
			log.Errorf("获取流量趋势失败 [%d_%s]: %v", endpointID, instanceID.String, err)
			// 回退到空数据，不阻止响应
			trafficTrend = make([]map[string]interface{}, 0)
		} else {
			trafficTrend = trendData
		}

		// 补充缺失的时间点到当前时间（每分钟一个点）
		trafficTrend = h.fillMissingTimePoints(trafficTrend, hours, "traffic")

		log.Debugf("流量趋势查询完成 [%d_%s]: %d 个数据点", endpointID, instanceID.String, len(trafficTrend))
	}

	// 返回流量趋势数据，格式与原接口兼容
	response := map[string]interface{}{
		"success":      true,
		"trafficTrend": trafficTrend,
		"hours":        hours,
		"count":        len(trafficTrend),
		"source":       "aggregated_metrics", // 标识数据来源
		"timestamp":    time.Now().Unix(),
	}

	c.JSON(http.StatusOK, response)
}

// HandleGetTunnelPingTrendV2 获取隧道延迟趋势数据（改进版）
// GET /api/tunnels/{id}/ping-trend
func (h *TunnelMetricsHandler) HandleGetTunnelPingTrendV2(c *gin.Context) {

	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "缺少隧道ID"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "无效的隧道ID"})
		return
	}

	// 解析小时数参数，默认24小时
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsedHours, err := strconv.Atoi(h); err == nil && parsedHours > 0 && parsedHours <= 168 { // 最多7天
			hours = parsedHours
		}
	}

	db := h.tunnelService.DB()

	// 查询隧道基本信息
	var endpointID int64
	var instanceID sql.NullString
	if err := db.QueryRow(h.tunnelService.Rebind(`SELECT endpoint_id, instance_id FROM tunnels WHERE id = ?`), id).Scan(&endpointID, &instanceID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "隧道不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var pingTrend []map[string]interface{}

	if instanceID.Valid && instanceID.String != "" {
		// 使用新的聚合器获取分钟级平均延迟数据
		trendData, err := h.sseProcessor.GetPingTrend(endpointID, instanceID.String, hours)
		if err != nil {
			log.Errorf("获取延迟趋势失败 [%d_%s]: %v", endpointID, instanceID.String, err)
			// 回退到空数据，不阻止响应
			pingTrend = make([]map[string]interface{}, 0)
		} else {
			pingTrend = trendData
		}

		// 补充缺失的时间点到当前时间（每分钟一个点）
		pingTrend = h.fillMissingTimePoints(pingTrend, hours, "ping")

		log.Debugf("延迟趋势查询完成 [%d_%s]: %d 个数据点", endpointID, instanceID.String, len(pingTrend))
	}

	// 返回延迟趋势数据，格式与原接口兼容
	response := map[string]interface{}{
		"success":   true,
		"pingTrend": pingTrend,
		"hours":     hours,
		"count":     len(pingTrend),
		"source":    "aggregated_metrics", // 标识数据来源
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, response)
}

// HandleGetTunnelPoolTrendV2 获取隧道连接池趋势数据（改进版）
// GET /api/tunnels/{id}/pool-trend
func (h *TunnelMetricsHandler) HandleGetTunnelPoolTrendV2(c *gin.Context) {

	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "缺少隧道ID"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "无效的隧道ID"})
		return
	}

	// 解析小时数参数，默认24小时
	hours := 24
	if h := c.Query("hours"); h != "" {
		if parsedHours, err := strconv.Atoi(h); err == nil && parsedHours > 0 && parsedHours <= 168 { // 最多7天
			hours = parsedHours
		}
	}

	db := h.tunnelService.DB()

	// 查询隧道基本信息
	var endpointID int64
	var instanceID sql.NullString
	if err := db.QueryRow(h.tunnelService.Rebind(`SELECT endpoint_id, instance_id FROM tunnels WHERE id = ?`), id).Scan(&endpointID, &instanceID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "隧道不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var poolTrend []map[string]interface{}

	if instanceID.Valid && instanceID.String != "" {
		// 使用新的聚合器获取分钟级平均连接池数据
		trendData, err := h.sseProcessor.GetPoolTrend(endpointID, instanceID.String, hours)
		if err != nil {
			log.Errorf("获取连接池趋势失败 [%d_%s]: %v", endpointID, instanceID.String, err)
			// 回退到空数据，不阻止响应
			poolTrend = make([]map[string]interface{}, 0)
		} else {
			poolTrend = trendData
		}

		// 补充缺失的时间点到当前时间（每分钟一个点）
		poolTrend = h.fillMissingTimePoints(poolTrend, hours, "pool")

		log.Debugf("连接池趋势查询完成 [%d_%s]: %d 个数据点", endpointID, instanceID.String, len(poolTrend))
	}

	// 返回连接池趋势数据，格式与原接口兼容
	response := map[string]interface{}{
		"success":   true,
		"poolTrend": poolTrend,
		"hours":     hours,
		"count":     len(poolTrend),
		"source":    "aggregated_metrics", // 标识数据来源
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, response)
}

// fillMissingTimePoints 补充缺失的时间点，确保每分钟都有数据点
func (h *TunnelMetricsHandler) fillMissingTimePoints(data []map[string]interface{}, hours int, metricType string) []map[string]interface{} {
	if len(data) == 0 {
		// 如果没有任何数据，创建全零的时间序列
		return h.createEmptyTimeSeries(hours, metricType)
	}

	// 创建时间索引映射
	timeMap := make(map[string]map[string]interface{})
	for _, item := range data {
		if eventTime, ok := item["eventTime"].(string); ok {
			timeMap[eventTime] = item
		}
	}

	// 生成完整的时间序列
	result := make([]map[string]interface{}, 0)
	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	for current := startTime.Truncate(time.Minute); current.Before(now); current = current.Add(time.Minute) {
		timeKey := current.Format("2006-01-02 15:04")

		if existingData, exists := timeMap[timeKey]; exists {
			// 使用现有数据
			result = append(result, existingData)
		} else {
			// 创建缺失时间点的零值数据
			zeroData := h.createZeroDataPoint(timeKey, metricType)
			result = append(result, zeroData)
		}
	}

	return result
}

// createEmptyTimeSeries 创建空的时间序列
func (h *TunnelMetricsHandler) createEmptyTimeSeries(hours int, metricType string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	for current := startTime.Truncate(time.Minute); current.Before(now); current = current.Add(time.Minute) {
		timeKey := current.Format("2006-01-02 15:04")
		zeroData := h.createZeroDataPoint(timeKey, metricType)
		result = append(result, zeroData)
	}

	return result
}

// createZeroDataPoint 创建零值数据点
func (h *TunnelMetricsHandler) createZeroDataPoint(timeKey, metricType string) map[string]interface{} {
	data := map[string]interface{}{
		"eventTime": timeKey,
	}

	switch metricType {
	case "ping":
		data["ping"] = float64(0)
		data["minPing"] = float64(0)
		data["maxPing"] = float64(0)
		data["successRate"] = float64(0)

	case "pool":
		data["pool"] = float64(0)
		data["minPool"] = float64(0)
		data["maxPool"] = float64(0)

	case "traffic":
		data["tcpRxRate"] = float64(0)
		data["tcpTxRate"] = float64(0)
		data["udpRxRate"] = float64(0)
		data["udpTxRate"] = float64(0)
	}

	return data
}

// HandleGetTunnelMetricsTrend 获取隧道所有趋势数据的统一接口（基于ServiceHistory表）
// GET /api/tunnels/{instanceId}/metrics-trend
func (h *TunnelMetricsHandler) HandleGetTunnelMetricsTrend(c *gin.Context) {

	instanceId := c.Param("id")
	if instanceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少实例ID"})
		return
	}

	// 解析小时数参数，默认24小时
	// hours := 24
	// if h := c.Query("hours"); h != "" {
	// 	if parsedHours, err := strconv.Atoi(h); err == nil && parsedHours > 0 && parsedHours <= 168 { // 最多7天
	// 		hours = parsedHours
	// 	}
	// }

	// 构建统一的趋势数据响应
	unifiedData, err := h.getUnifiedTrendDataFromServiceHistory(instanceId, 24)
	if err != nil {
		// 如果数据库已关闭，则不再继续刷日志
		if strings.Contains(err.Error(), "database is closed") {
			log.Warnf("趋势数据查询取消（数据库已关闭）[%s]", instanceId)
		} else {
			log.Errorf("获取统一趋势数据失败 [%s]: %v", instanceId, err)
		}
		// 回退到空数据，不阻止响应
		log.Info("回退到空数据，不阻止响应")
		unifiedData = h.createEmptyTrendData(24)
	}

	// 安全地获取数据长度，支持不同的数据类型
	getArrayLength := func(data interface{}) int {
		if data == nil {
			return 0
		}
		switch v := data.(type) {
		case []float64:
			return len(v)
		case []interface{}:
			return len(v)
		case []int64:
			return len(v)
		default:
			return 0
		}
	}

	trafficLen := getArrayLength(unifiedData["traffic"].(map[string]interface{})["avg_delay"])
	pingLen := getArrayLength(unifiedData["ping"].(map[string]interface{})["avg_delay"])
	poolLen := getArrayLength(unifiedData["pool"].(map[string]interface{})["avg_delay"])

	log.Debugf("ServiceHistory趋势查询完成 [%s]: 数据点数量 = traffic:%d, ping:%d, pool:%d",
		instanceId, trafficLen, pingLen, poolLen,
	)

	// 返回统一的趋势数据
	response := map[string]interface{}{
		"success":   true,
		"data":      unifiedData,
		"timestamp": time.Now().Unix(),
	}

	c.JSON(http.StatusOK, response)
}

func (h *TunnelMetricsHandler) getUnifiedTrendDataFromServiceHistory(instanceID string, hours int) (map[string]interface{}, error) {
	startTime := time.Now().Add(-time.Duration(hours) * time.Hour).Truncate(time.Minute)
	var records []models.ServiceHistory
	if err := h.tunnelService.GormDB().
		Where("instance_id = ? AND record_time >= ?", instanceID, startTime).
		Order("record_time ASC, id ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}

	byMinute := make(map[int64]models.ServiceHistory, len(records))
	for _, record := range records {
		byMinute[record.RecordTime.Truncate(time.Minute).Unix()] = record
	}

	timePoints := h.generateTimePoints(startTime, hours)
	timestamps := make([]int64, 0, len(timePoints))
	traffic := make([]float64, 0, len(timePoints))
	ping := make([]float64, 0, len(timePoints))
	pool := make([]float64, 0, len(timePoints))
	tcps := make([]float64, 0, len(timePoints))
	udps := make([]float64, 0, len(timePoints))
	speedIn := make([]float64, 0, len(timePoints))
	speedOut := make([]float64, 0, len(timePoints))
	tcpIn := make([]float64, 0, len(timePoints))
	tcpOut := make([]float64, 0, len(timePoints))
	udpIn := make([]float64, 0, len(timePoints))
	udpOut := make([]float64, 0, len(timePoints))

	for _, point := range timePoints {
		timestamps = append(timestamps, point.UnixMilli())
		record, ok := byMinute[point.Unix()]
		if !ok {
			traffic = append(traffic, 0)
			ping = append(ping, 0)
			pool = append(pool, 0)
			tcps = append(tcps, 0)
			udps = append(udps, 0)
			speedIn = append(speedIn, 0)
			speedOut = append(speedOut, 0)
			tcpIn = append(tcpIn, 0)
			tcpOut = append(tcpOut, 0)
			udpIn = append(udpIn, 0)
			udpOut = append(udpOut, 0)
			continue
		}

		tcpInValue := float64(record.DeltaTCPIn)
		tcpOutValue := float64(record.DeltaTCPOut)
		udpInValue := float64(record.DeltaUDPIn)
		udpOutValue := float64(record.DeltaUDPOut)
		traffic = append(traffic, tcpInValue+tcpOutValue+udpInValue+udpOutValue)
		ping = append(ping, record.AvgPing)
		pool = append(pool, float64(record.AvgPool))
		tcps = append(tcps, float64(record.AvgTCPs))
		udps = append(udps, float64(record.AvgUDPs))
		speedIn = append(speedIn, record.AvgSpeedIn)
		speedOut = append(speedOut, record.AvgSpeedOut)
		tcpIn = append(tcpIn, tcpInValue)
		tcpOut = append(tcpOut, tcpOutValue)
		udpIn = append(udpIn, udpInValue)
		udpOut = append(udpOut, udpOutValue)
	}

	series := func(values []float64) map[string]interface{} {
		return map[string]interface{}{"avg_delay": values, "created_at": timestamps}
	}
	return map[string]interface{}{
		"traffic":   series(traffic),
		"ping":      series(ping),
		"pool":      series(pool),
		"tcps":      series(tcps),
		"udps":      series(udps),
		"speed_in":  series(speedIn),
		"speed_out": series(speedOut),
		"tcp_in":    series(tcpIn),
		"tcp_out":   series(tcpOut),
		"udp_in":    series(udpIn),
		"udp_out":   series(udpOut),
	}, nil
}

// generateTimePoints 生成完整的时间点序列
func (h *TunnelMetricsHandler) generateTimePoints(startTime time.Time, hours int) []time.Time {
	var timePoints []time.Time
	current := startTime.Truncate(time.Minute)
	// 结束时间设为当前时间的前一分钟，避免包含当前分钟（可能还没有数据）
	end := time.Now().Add(-time.Minute).Truncate(time.Minute)

	for current.Before(end) || current.Equal(end) {
		timePoints = append(timePoints, current)
		current = current.Add(time.Minute)
	}

	return timePoints
}

// createEmptyTrendData 创建空的趋势数据
func (h *TunnelMetricsHandler) createEmptyTrendData(hours int) map[string]interface{} {
	// 生成时间点
	startTime := time.Now().Add(-time.Duration(hours) * time.Hour).Truncate(time.Minute)
	timePoints := h.generateTimePoints(startTime, hours)

	var (
		timestampsMs []int64
		emptyData    []float64
	)

	for _, timePoint := range timePoints {
		timestampsMs = append(timestampsMs, timePoint.UnixMilli())
		emptyData = append(emptyData, 0)
	}

	return map[string]interface{}{
		"traffic": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"ping": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"pool": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"tcps": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"udps": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"speed_in": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"speed_out": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		// 新增：分开的流量数据
		"tcp_in": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"tcp_out": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"udp_in": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
		"udp_out": map[string]interface{}{
			"avg_delay":  emptyData,
			"created_at": timestampsMs,
		},
	}
}

// GetMetricsStats 获取指标统计信息（调试用）
func (h *TunnelMetricsHandler) GetMetricsStats() map[string]interface{} {
	return h.sseProcessor.GetStats()
}
