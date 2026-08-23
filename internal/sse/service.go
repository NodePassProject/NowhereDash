package sse

import (
	"NowhereDash/internal/endpoint"
	log "NowhereDash/internal/log"
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"NowhereDash/internal/subscription"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Service SSE服务
type Service struct {
	// 客户端管理
	clients    map[string]*Client            // 全局客户端
	tunnelSubs map[string]map[string]*Client // 隧道订阅者
	mu         sync.RWMutex

	// 数据存储
	db *gorm.DB

	// 端点服务引用
	endpointService *endpoint.Service

	// Manager引用（用于状态通知）
	manager *Manager

	// 历史数据处理Worker（类似Nezha的ServiceHistory）
	historyWorker *HistoryWorker

	// 文件日志管理器
	fileLogger *log.FileLogger // 文件日志管理器

	// 配置选项
	disableLogStore bool // 禁用日志记录到文件

	// 上下文控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewService 创建SSE服务实例
func NewService(db *gorm.DB, endpointService *endpoint.Service, disableLogStore bool) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	// 创建日志目录路径
	logDir := filepath.Join("logs")

	// 如果禁用日志存储，fileLogger 设置为 nil
	var fileLogger *log.FileLogger
	if !disableLogStore {
		fileLogger = log.NewFileLogger(logDir)
		log.Info("SSE 日志文件记录已启用")
	} else {
		log.Info("SSE 日志文件记录已禁用")
	}

	s := &Service{
		clients:         make(map[string]*Client),
		tunnelSubs:      make(map[string]map[string]*Client),
		db:              db,
		endpointService: endpointService,
		historyWorker:   NewHistoryWorker(db),
		fileLogger:      fileLogger,
		disableLogStore: disableLogStore,
		ctx:             ctx,
		cancel:          cancel,
	}

	return s
}

// SetManager 设置Manager引用
func (s *Service) SetManager(manager *Manager) {
	s.manager = manager
}

// Close 关闭服务
func (s *Service) Close() {
	log.Info("正在关闭SSE服务")

	// 停止上下文
	s.cancel()

	// 关闭历史数据Worker
	if s.historyWorker != nil {
		s.historyWorker.Close()
	}

	// 关闭所有客户端
	s.mu.Lock()
	for clientID, client := range s.clients {
		client.Close()
		delete(s.clients, clientID)
	}
	s.mu.Unlock()

	// 关闭文件日志管理器
	if s.fileLogger != nil {
		s.fileLogger.Close()
	}

	log.Info("SSE服务已关闭")
}

// ======================== 前端SSE处理器 ============================

// AddClient 添加客户端
func (s *Service) AddClient(clientID string, w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &Client{
		ID:     clientID,
		Writer: w,
	}

	s.clients[clientID] = client
	log.Infof("客户端 %s 已连接", clientID)
}

// RemoveClient 移除客户端
func (s *Service) RemoveClient(clientID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if client, exists := s.clients[clientID]; exists {
		delete(s.clients, clientID)
		log.Infof("客户端 %s 已断开连接", clientID)
		client.Close()
	}

	// 从隧道订阅中移除
	for tunnelID, subscribers := range s.tunnelSubs {
		if _, exists := subscribers[clientID]; exists {
			delete(subscribers, clientID)
			if len(subscribers) == 0 {
				delete(s.tunnelSubs, tunnelID)
			}
		}
	}
}

// SubscribeToTunnel 订阅隧道事件
func (s *Service) SubscribeToTunnel(clientID, tunnelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tunnelSubs[tunnelID] == nil {
		s.tunnelSubs[tunnelID] = make(map[string]*Client)
	}
	s.tunnelSubs[tunnelID][clientID] = s.clients[clientID]
	log.Infof("客户端 %s 订阅隧道 %s", clientID, tunnelID)
}

// UnsubscribeFromTunnel 取消订阅隧道事件
func (s *Service) UnsubscribeFromTunnel(clientID, tunnelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if subscribers, exists := s.tunnelSubs[tunnelID]; exists {
		delete(subscribers, clientID)
		if len(subscribers) == 0 {
			delete(s.tunnelSubs, tunnelID)
		}
		log.Infof("客户端 %s 取消订阅隧道 %s", clientID, tunnelID)
	}
}

// ======================== 事件处理器 ============================

// ProcessEvent 处理SSE事件
func (s *Service) ProcessEvent(payload SSEResp) {
	switch payload.Type {
	case "shutdown":
		s.handleShutdownEvent(payload)
	case "initial":
		s.handleInitialEvent(payload)
	case "create":
		s.handleCreateEvent(payload)
	case "update":
		if payload.Instance.Type != string(models.TunnelTypePortal) {
			return
		}
		if s.historyWorker != nil {
			s.historyWorker.Dispatch(payload)
		}
		s.handleUpdateEvent(payload)
	case "delete":
		if payload.Instance.Type != string(models.TunnelTypePortal) {
			return
		}
		s.handleDeleteEvent(payload)
	case "log":
		if payload.Instance.Type != string(models.TunnelTypePortal) {
			return
		}
		s.handleLogEvent(payload)
	}
}

// 事件处理方法
func (s *Service) handleInitialEvent(payload SSEResp) {
	// SSE initial 事件表示端点重新连接时报告现有隧道状态
	log.Debugf("[Master-%d]处理初始化事件: 隧道 %s", payload.EndpointID, payload.Instance.ID)
	if payload.Instance.Type == "" {
		// 当InstanceType为空时，尝试获取端点系统信息
		go s.fetchAndUpdateEndpointInfo(payload.EndpointID)
		return
	}
	if payload.Instance.Type != string(models.TunnelTypePortal) {
		log.Debugf("[Master-%d]忽略非隧道实例 %s (type=%s)", payload.EndpointID, payload.Instance.ID, payload.Instance.Type)
		return
	}

	// 检查隧道是否已存在（用 Find+Limit 避免 GORM 把 not-found 打成 ERROR）
	var existing []models.Tunnel
	if err := s.db.Where("endpoint_id = ? AND instance_id = ? AND type = ?", payload.EndpointID, payload.Instance.ID, models.TunnelTypePortal).Limit(1).Find(&existing).Error; err != nil {
		log.Errorf("[Master-%d]查询隧道 %s 失败: %v", payload.EndpointID, payload.Instance.ID, err)
		return
	}
	if len(existing) > 0 {
		// 隧道已存在（正常情况），更新运行时信息
		log.Debugf("[Master-%d]隧道 %s 已存在，更新运行时信息", payload.EndpointID, payload.Instance.ID)
		s.updateTunnelRuntimeInfo(payload)
		return
	}
	// 创建最小化隧道记录，包含从EndpointSSE获取的流量等信息
	tunnel := buildTunnel(payload)
	if err := s.db.Create(&tunnel).Error; err != nil {
		log.Errorf("[Master-%d]初始化隧道 %s 失败: %v", payload.EndpointID, payload.Instance.ID, err)
	} else {
		log.Infof("[Master-%d]最小化隧道记录 %s 初始化成功，包含流量信息", payload.EndpointID, payload.Instance.ID)
		s.updateEndpointTunnelCount(payload.EndpointID)

	}
}

func buildTunnel(payload SSEResp) *models.Tunnel {
	tunnel := nowhere.ParseInstanceTunnel(payload.Instance)
	// 补充从EndpointSSE获取的信息
	tunnel.EndpointID = payload.EndpointID
	tunnel.InstanceID = &payload.Instance.ID
	tunnel.TCPRx = payload.Instance.TCPRx
	tunnel.TCPTx = payload.Instance.TCPTx
	tunnel.UDPRx = payload.Instance.UDPRx
	tunnel.UDPTx = payload.Instance.UDPTx
	tunnel.TCPs = payload.Instance.TCPs
	tunnel.UDPs = payload.Instance.UDPs
	tunnel.Pool = payload.Instance.Pool
	tunnel.Ping = payload.Instance.Ping
	tunnel.LastEventTime = models.NullTime{Time: payload.TimeStamp, Valid: true}
	tunnel.EnableLogStore = true
	tunnel.Restart = payload.Instance.Restart
	if payload.Instance.Alias != nil && *payload.Instance.Alias != "" {
		tunnel.Name = *payload.Instance.Alias
	} else {
		tunnel.Name = payload.Instance.ID
	}
	tunnel.Status = models.TunnelStatus(payload.Instance.Status)
	if payload.Instance.Meta != nil {
		tunnel.Tags = payload.Instance.Meta.Tags
		tunnel.Peer = payload.Instance.Meta.Peer
	}

	return tunnel
}

func (s *Service) handleCreateEvent(payload SSEResp) {
	// SSE create 事件表示 Nowhere 客户端报告隧道创建成功
	// 此时隧道记录应该已经由 API 创建，我们只需要更新状态和流量信息
	log.Debugf("[Master-%d]处理创建事件: 隧道 %s", payload.EndpointID, payload.Instance.ID)
	if payload.Instance.Type != string(models.TunnelTypePortal) {
		return
	}
	// 先解析 URL 获取隧道配置信息
	tunnel := buildTunnel(payload)
	// Insert first, then use the event timestamp as an atomic update guard so
	// workers cannot let an older event overwrite a newer state.
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "endpoint_id"},
			{Name: "instance_id"},
		},
		DoNothing: true,
	}).Create(&tunnel).Error; err != nil {
		log.Errorf("[Master-%d]创建隧道记录 %s 失败: %v", payload.EndpointID, payload.Instance.ID, err)
		return
	}
	result := s.db.Model(&models.Tunnel{}).
		Where("endpoint_id = ? AND instance_id = ? AND type = ?", payload.EndpointID, payload.Instance.ID, models.TunnelTypePortal).
		Where("last_event_time IS NULL OR last_event_time <= ?", payload.TimeStamp).
		Updates(nowhere.TunnelToMap(tunnel))
	if result.Error != nil {
		log.Errorf("[Master-%d]更新隧道记录 %s 失败: %v", payload.EndpointID, payload.Instance.ID, result.Error)
		return
	}
	log.Infof("[Master-%d]隧道记录 %s 处理成功（创建或更新）", payload.EndpointID, payload.Instance.ID)
	s.updateEndpointTunnelCount(payload.EndpointID)
}

func (s *Service) handleUpdateEvent(payload SSEResp) {
	// SSE update 事件用于更新隧道的运行时信息
	log.Debugf("[Master-%d]处理更新事件: 隧道 %s", payload.EndpointID, payload.Instance.ID)
	if payload.Instance.Type != string(models.TunnelTypePortal) {
		return
	}

	// 先检查隧道是否存在（Find+Limit 避免 GORM error 日志）
	var checkRows []models.Tunnel
	if err := s.db.Where("endpoint_id = ? AND instance_id = ? AND type = ?", payload.EndpointID, payload.Instance.ID, models.TunnelTypePortal).Limit(1).Find(&checkRows).Error; err != nil {
		log.Errorf("[Master-%d]查询隧道 %s 失败: %v", payload.EndpointID, payload.Instance.ID, err)
		return
	}
	if len(checkRows) == 0 {
		log.Warnf("[Master-%d]收到更新事件但隧道 %s 不存在，可能是时序问题，跳过处理", payload.EndpointID, payload.Instance.ID)
		return
	}

	// 更新运行时信息
	s.updateTunnelRuntimeInfo(payload)
}

func (s *Service) handleDeleteEvent(payload SSEResp) {
	if payload.Instance.Type != string(models.TunnelTypePortal) {
		return
	}

	// 先获取隧道ID，用于删除相关的操作日志（Find+Limit 避免 GORM error 日志）
	var delRows []models.Tunnel
	if err := s.db.Where("endpoint_id = ? AND instance_id = ? AND type = ?", payload.EndpointID, payload.Instance.ID, models.TunnelTypePortal).Limit(1).Find(&delRows).Error; err != nil {
		log.Errorf("[Master-%d]获取隧道 %s 失败: %v", payload.EndpointID, payload.Instance.ID, err)
		return
	}
	if len(delRows) == 0 {
		return
	}
	tunnel := delRows[0]
	if err := subscription.NewService(s.db).AccountTunnels([]int64{tunnel.ID}); err != nil {
		log.Errorf("[Master-%d]结算隧道 %s 订阅流量失败: %v", payload.EndpointID, payload.Instance.ID, err)
		return
	}

	// 先删除相关的操作日志记录，避免外键约束错误
	if err := s.db.Where("tunnel_id = ?", tunnel.ID).Delete(&models.TunnelOperationLog{}).Error; err != nil {
		log.Warnf("[Master-%d]删除隧道 %s 操作日志失败: %v", payload.EndpointID, payload.Instance.ID, err)
	}

	// 删除隧道记录
	err := s.db.Where("endpoint_id = ? AND instance_id = ? AND type = ?", payload.EndpointID, payload.Instance.ID, models.TunnelTypePortal).Delete(&models.Tunnel{}).Error

	if err != nil {
		log.Errorf("[Master-%d]删除隧道 %s 失败: %v", payload.EndpointID, payload.Instance.ID, err)
	} else {
		log.Infof("[Master-%d]隧道 %s 删除成功", payload.EndpointID, payload.Instance.ID)
		// 更新端点隧道计数
		s.updateEndpointTunnelCount(payload.EndpointID)

	}
}

func (s *Service) handleLogEvent(payload SSEResp) {
	if payload.Instance.Type != string(models.TunnelTypePortal) {
		return
	}

	// 日志事件需要写入文件日志系统
	log.Debugf("[Master-%d]处理日志事件: 隧道 %s", payload.EndpointID, payload.Instance.ID)

	if payload.Logs == nil || *payload.Logs == "" {
		log.Debugf("[Master-%d]隧道 %s 日志事件内容为空，跳过文件写入", payload.EndpointID, payload.Instance.ID)
		return
	}

	// 如果禁用了日志存储，只推流不写文件
	if s.disableLogStore {
		log.Debugf("[Master-%d]SSE 日志记录已禁用，跳过文件写入", payload.EndpointID)
		// 推流转发给前端订阅
		s.sendTunnelUpdateByInstanceId(payload.Instance.ID, payload)
		return
	}

	// 使用文件日志管理器写入日志文件
	if s.fileLogger != nil {
		err := s.fileLogger.WriteLog(payload.EndpointID, payload.Instance.ID, *payload.Logs)
		if err != nil {
			log.Errorf("[Master-%d]写入隧道 %s 日志到文件失败: %v", payload.EndpointID, payload.Instance.ID, err)
		} else {
			log.Debugf("[Master-%d]隧道 %s 日志已写入文件", payload.EndpointID, payload.Instance.ID)
		}
	} else {
		log.Warnf("[Master-%d]文件日志管理器未初始化，无法写入日志", payload.EndpointID)
	}
	// 推流转发给前端订阅
	// log.Debugf("[Master-%d#SSE]准备推送事件给前端，eventType=%s instanceID=%s", endpointID, event.EventType, event.InstanceID)
	s.sendTunnelUpdateByInstanceId(payload.Instance.ID, payload)
}

func (s *Service) handleShutdownEvent(payload SSEResp) {
	// 使用GORM更新端点状态
	if err := s.db.Model(&models.Endpoint{}).
		Where("id = ?", payload.EndpointID).
		Updates(map[string]interface{}{
			"status":     models.EndpointStatusOffline,
			"last_check": time.Now(),
			"updated_at": time.Now(),
		}).Error; err != nil {
		log.Errorf("[Master-%d#SSE]更新端点状态失败: %v", payload.EndpointID, err)
		return
	}

	log.Infof("[Master-%d]端点状态已更新为: %s", payload.EndpointID, models.EndpointStatusOffline)

	// 如果端点状态变为离线，设置所有相关隧道为离线状态
	if err := s.setTunnelsOfflineForEndpoint(payload.EndpointID); err != nil {
		log.Errorf("[Master-%d]设置隧道离线状态失败: %v", payload.EndpointID, err)
	}

	// 通知Manager状态变化
	s.manager.NotifyEndpointStatusChanged(payload.EndpointID, string(models.EndpointStatusOffline))
}

// updateTunnelRuntimeInfo 更新隧道运行时信息（流量、状态、ping等）
func (s *Service) updateTunnelRuntimeInfo(payload SSEResp) {
	// 准备更新字段
	tunnel := buildTunnel(payload)
	updates := nowhere.TunnelToMap(tunnel)
	// 更新 tunnel 表
	result := s.db.Model(&models.Tunnel{}).
		Where("endpoint_id = ? AND instance_id = ? AND type = ?", payload.EndpointID, payload.Instance.ID, models.TunnelTypePortal).
		Where("last_event_time IS NULL OR last_event_time <= ?", payload.TimeStamp).
		Updates(updates)

	if result.Error != nil {
		log.Errorf("[Master-%d]更新隧道 %s 运行时信息失败: %v", payload.EndpointID, payload.Instance.ID, result.Error)
		return
	}

	// 检查是否真的更新了记录（可能找不到匹配的tunnel）
	if result.RowsAffected == 0 {
		log.Debugf("[Master-%d]隧道 %s 事件已过期或记录不存在，跳过更新",
			payload.EndpointID, payload.Instance.ID)
		return
	}

	log.Debugf("[Master-%d]隧道 %s 运行时信息已更新", payload.EndpointID, payload.Instance.ID)

}

// fetchAndUpdateEndpointInfo 获取并更新端点系统信息
func (s *Service) fetchAndUpdateEndpointInfo(endpointID int64) {

	// 尝试获取系统信息 (处理低版本API不存在的情况)
	var info *nowhere.EndpointInfoResult
	if r := recover(); r != nil {
		log.Warnf("[Master-%d] 获取系统信息失败(可能为低版本): %v", endpointID, r)
	}

	info, err := nowhere.GetInfo(endpointID)
	if err != nil {
		log.Warnf("[Master-%d] 获取系统信息失败: %v", endpointID, err)
		// 不返回错误，继续处理
	}

	// 如果成功获取到信息，更新数据库
	if info != nil && err == nil {
		if updateErr := s.endpointService.UpdateEndpointInfo(endpointID, *info); updateErr != nil {
			log.Errorf("[Master-%d] 更新系统信息失败: %v", endpointID, updateErr)
		} else {
			// 在日志中显示uptime信息
			uptimeMsg := fmt.Sprintf("%d秒", info.Uptime)
			log.Infof("[Master-%d] 系统信息已更新: OS=%s, Arch=%s, Ver=%s, Uptime=%s", endpointID, info.OS, info.Arch, info.Ver, uptimeMsg)
		}
	}
}

// updateEndpointTunnelCount 更新端点的隧道计数
func (s *Service) updateEndpointTunnelCount(endpointID int64) {
	var count int64
	if err := s.db.Model(&models.Tunnel{}).
		Where("endpoint_id = ? AND type = ?", endpointID, models.TunnelTypePortal).
		Count(&count).Error; err != nil {
		log.Errorf("[Master-%d] 统计端点隧道数量失败: %v", endpointID, err)
		return
	}
	if err := s.db.Model(&models.Endpoint{}).
		Where("id = ?", endpointID).
		Update("tunnel_count", count).Error; err != nil {
		log.Errorf("[Master-%d] 更新端点隧道计数失败: %v", endpointID, err)
		return
	}

	log.Infof("[Master-%d#SSE]更新端点隧道计数为: %d", endpointID, count)
}

// sendTunnelUpdateByInstanceId 根据实例ID发送隧道更新
func (s *Service) sendTunnelUpdateByInstanceId(instanceID string, data SSEResp) {
	s.mu.RLock()
	subscribers, exists := s.tunnelSubs[instanceID]
	if !exists {
		s.mu.RUnlock()
		// log.Debug("[SSE]隧道 %s 没有订阅者", instanceID)
		return
	}

	// 创建订阅者副本，避免在遍历时修改map
	clientList := make([]*Client, 0, len(subscribers))
	clientIDs := make([]string, 0, len(subscribers))
	for clientID, client := range subscribers {
		clientList = append(clientList, client)
		clientIDs = append(clientIDs, clientID)
	}
	s.mu.RUnlock()

	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Errorf("序列化隧道数据失败: %v", err)
		return
	}

	// 记录需要删除的客户端
	disconnectedClients := make([]string, 0)

	for i, client := range clientList {
		clientID := clientIDs[i]
		if err := client.Send(jsonData); err != nil {
			// 检查是否是连接断开错误，使用更合适的日志级别
			if client.IsDisconnected() {
				log.Warnf("[SSE]客户端 %s 连接已断开，标记移除订阅", clientID)
				disconnectedClients = append(disconnectedClients, clientID)
			} else {
				log.Errorf("发送隧道更新给客户端 %s 失败: %v", clientID, err)
			}
		} else {
			log.Debugf("[SSE]隧道 %s 的订阅者 %s 推送成功", instanceID, clientID)
		}
	}

	// 如果有断开的客户端，获取写锁并移除它们
	if len(disconnectedClients) > 0 {
		s.mu.Lock()
		if subscribers, exists := s.tunnelSubs[instanceID]; exists {
			for _, clientID := range disconnectedClients {
				delete(subscribers, clientID)
				log.Infof("[SSE]已移除断开连接的客户端 %s from tunnel %s", clientID, instanceID)
			}
			// 如果订阅者列表为空，删除整个条目
			if len(subscribers) == 0 {
				delete(s.tunnelSubs, instanceID)
				log.Debugf("[SSE]隧道 %s 无订阅者，清理订阅列表", instanceID)
			}
		}
		s.mu.Unlock()
	}
}

// setTunnelsOfflineForEndpoint 将端点的所有隧道设置为离线状态
func (s *Service) setTunnelsOfflineForEndpoint(endpointID int64) error {
	// 使用GORM批量更新隧道状态
	result := s.db.Model(&models.Tunnel{}).
		Where("endpoint_id = ? AND type = ?", endpointID, models.TunnelTypePortal).
		Updates(map[string]interface{}{
			"status":          models.TunnelStatusOffline,
			"updated_at":      time.Now(),
			"last_event_time": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}

	log.Infof("[Master-%d]已将%d个隧道设置为离线状态", endpointID, result.RowsAffected)
	return nil
}

// =============== 文件日志 ===============
// GetFileLogger 获取文件日志管理器
func (s *Service) GetFileLogger() *log.FileLogger {
	return s.fileLogger
}
