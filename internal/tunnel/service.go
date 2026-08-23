package tunnel

import (
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"NowhereDash/internal/subscription"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db *gorm.DB
}

type OperationLog struct {
	ID         int64          `json:"id"`
	TunnelID   sql.NullInt64  `json:"tunnelId,omitempty"`
	TunnelName string         `json:"tunnelName"`
	Action     string         `json:"action"`
	Status     string         `json:"status"`
	Message    sql.NullString `json:"message,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Rebind(query string) string {
	if s.db.Dialector.Name() != "postgres" {
		return query
	}
	var out strings.Builder
	position := 1
	for _, char := range query {
		if char == '?' {
			fmt.Fprintf(&out, "$%d", position)
			position++
		} else {
			out.WriteRune(char)
		}
	}
	return out.String()
}

func (s *Service) DB() *sql.DB {
	db, _ := s.db.DB()
	return db
}

func (s *Service) GormDB() *gorm.DB { return s.db }

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func portalFromRequest(req PortalRequest) models.Tunnel {
	return models.Tunnel{
		Name:           strings.TrimSpace(req.Name),
		EndpointID:     req.EndpointID,
		Type:           models.TunnelTypePortal,
		Status:         models.TunnelStatusStopped,
		ListenHost:     strings.TrimSpace(req.ListenHost),
		ListenPort:     strings.TrimSpace(req.ListenPort),
		SharedKey:      stringPointer(req.SharedKey),
		Network:        stringPointer(req.Network),
		TLSMode:        models.TLSMode(req.TLSMode),
		CertPath:       stringPointer(req.CertPath),
		KeyPath:        stringPointer(req.KeyPath),
		ALPN:           stringPointer(req.ALPN),
		Rate:           req.Rate,
		Etar:           req.Etar,
		Dial:           stringPointer(req.Dial),
		Socks:          stringPointer(req.Socks),
		Next:           stringPointer(req.Next),
		Up:             stringPointer(req.Up),
		Down:           stringPointer(req.Down),
		PoolSize:       req.PoolSize,
		Sni:            stringPointer(req.Sni),
		Pin:            stringPointer(req.Pin),
		LogLevel:       models.LogLevel(req.LogLevel),
		Restart:        &req.Restart,
		Tags:           req.Tags,
		Peer:           req.Peer,
		EnableLogStore: req.EnableStore,
	}
}

func applyInstanceState(tunnel *models.Tunnel, instance nowhere.InstanceResult) {
	tunnel.InstanceID = &instance.ID
	nowhere.ApplyInstanceConfig(tunnel, instance)
	if instance.Status != "" {
		tunnel.Status = models.TunnelStatus(instance.Status)
	}
	tunnel.TCPRx = instance.TCPRx
	tunnel.TCPTx = instance.TCPTx
	tunnel.UDPRx = instance.UDPRx
	tunnel.UDPTx = instance.UDPTx
	tunnel.TCPs = instance.TCPs
	tunnel.UDPs = instance.UDPs
	tunnel.Pool = instance.Pool
	tunnel.Ping = instance.Ping
	if instance.Restart != nil {
		tunnel.Restart = instance.Restart
	}
	if instance.Meta != nil {
		tunnel.Tags = instance.Meta.Tags
		tunnel.Peer = instance.Meta.Peer
	}
}

func applySubmittedMetadata(tunnel *models.Tunnel, tags *map[string]string, peer *models.Peer) {
	tunnel.Tags = tags
	tunnel.Peer = peer
}

func (s *Service) syncMetadata(endpointID int64, instanceID string, tags *map[string]string, peer *models.Peer) error {
	if tags == nil && peer == nil {
		return nil
	}
	_, err := nowhere.UpdateInstanceMetadata(endpointID, instanceID, tags, peer)
	return err
}

var createdPortalAssignmentColumns = []string{
	"name",
	"type",
	"listen_host",
	"listen_port",
	"tls_mode",
	"cert_path",
	"key_path",
	"log_level",
	"command_line",
	"shared_key",
	"restart",
	"rate",
	"enable_log_store",
	"tags",
	"peer",
	"network",
	"alpn",
	"etar",
	"dial",
	"socks",
	"next",
	"up",
	"down",
	"pool_size",
	"sni",
	"pin",
	"sorts",
	"updated_at",
}

// persistCreatedPortal reconciles the API result with a row that may already
// have been inserted by OpenCtrl's create SSE event. Configuration submitted
// by the API wins, while the SSE row remains authoritative for runtime state.
func (s *Service) persistCreatedPortal(tunnel *models.Tunnel) error {
	if tunnel.InstanceID == nil || *tunnel.InstanceID == "" {
		return errors.New("instance ID is missing")
	}

	instanceID := *tunnel.InstanceID
	var canonical models.Tunnel
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "endpoint_id"},
				{Name: "instance_id"},
			},
			DoUpdates: clause.AssignmentColumns(createdPortalAssignmentColumns),
		}).Create(tunnel).Error; err != nil {
			return fmt.Errorf("upsert created tunnel: %w", err)
		}

		if err := tx.Where("endpoint_id = ? AND instance_id = ?", tunnel.EndpointID, instanceID).
			First(&canonical).Error; err != nil {
			return fmt.Errorf("load created tunnel: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	*tunnel = canonical
	return nil
}

func (s *Service) CreatePortal(req PortalRequest) (*models.Tunnel, error) {
	tunnel := portalFromRequest(req)
	if err := nowhere.ValidatePortalTunnel(tunnel); err != nil {
		return nil, err
	}
	commandLine := nowhere.BuildTunnelURLs(tunnel)
	created, err := nowhere.CreateInstance(req.EndpointID, commandLine)
	if err != nil {
		return nil, err
	}

	if req.Name != "" {
		if _, err = nowhere.RenameInstance(req.EndpointID, created.ID, req.Name); err != nil {
			_ = nowhere.DeleteInstance(req.EndpointID, created.ID)
			return nil, fmt.Errorf("set instance alias: %w", err)
		}
	}
	if _, err = nowhere.SetRestartInstance(req.EndpointID, created.ID, req.Restart); err != nil {
		_ = nowhere.DeleteInstance(req.EndpointID, created.ID)
		return nil, fmt.Errorf("set restart policy: %w", err)
	}
	if err = s.syncMetadata(req.EndpointID, created.ID, req.Tags, req.Peer); err != nil {
		_ = nowhere.DeleteInstance(req.EndpointID, created.ID)
		return nil, fmt.Errorf("set metadata: %w", err)
	}

	tunnel.CommandLine = commandLine
	applyInstanceState(&tunnel, created)
	applySubmittedMetadata(&tunnel, req.Tags, req.Peer)
	if tunnel.Name == "" {
		tunnel.Name = created.ID
	}
	var maxSort int64
	s.db.Model(&models.Tunnel{}).Select("COALESCE(MAX(sorts), -1)").Scan(&maxSort)
	tunnel.Sorts = maxSort + 1
	if err = s.persistCreatedPortal(&tunnel); err != nil {
		return nil, err
	}
	s.updateEndpointTunnelCount(req.EndpointID)
	s.recordOperation(&tunnel, models.OperationActionCreate, "Tunnel instance created")
	return &tunnel, nil
}

func (s *Service) CreatePortalURL(endpointID int64, rawURL, name string) (*models.Tunnel, error) {
	parsed := nowhere.ParseTunnelURL(strings.TrimSpace(rawURL))
	parsed.EndpointID = endpointID
	parsed.Name = strings.TrimSpace(name)
	if err := nowhere.ValidatePortalTunnel(*parsed); err != nil {
		return nil, err
	}
	request := PortalRequest{
		Name: parsed.Name, EndpointID: endpointID, ListenHost: parsed.ListenHost,
		ListenPort: parsed.ListenPort, SharedKey: value(parsed.SharedKey), Network: value(parsed.Network),
		TLSMode: parsed.TLSMode, CertPath: value(parsed.CertPath), KeyPath: value(parsed.KeyPath),
		ALPN: value(parsed.ALPN), Rate: parsed.Rate, Etar: parsed.Etar, Dial: value(parsed.Dial),
		Socks: value(parsed.Socks), Next: value(parsed.Next), Up: value(parsed.Up), Down: value(parsed.Down),
		PoolSize: parsed.PoolSize, Sni: value(parsed.Sni), Pin: value(parsed.Pin), LogLevel: parsed.LogLevel,
		EnableStore: true,
	}
	return s.CreatePortal(request)
}

func value(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}

func (s *Service) UpdatePortal(id int64, req PortalRequest) (*models.Tunnel, error) {
	var existing models.Tunnel
	if err := s.db.Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&existing).Error; err != nil {
		return nil, err
	}
	if existing.InstanceID == nil || *existing.InstanceID == "" {
		return nil, errors.New("instance ID is missing")
	}
	req.EndpointID = existing.EndpointID
	updated := portalFromRequest(req)
	updated.ID = existing.ID
	updated.InstanceID = existing.InstanceID
	updated.Sorts = existing.Sorts
	updated.CreatedAt = existing.CreatedAt
	if req.Tags == nil {
		updated.Tags = existing.Tags
	}
	if req.Peer == nil {
		updated.Peer = existing.Peer
	}
	desiredTags := updated.Tags
	desiredPeer := updated.Peer
	if err := nowhere.ValidatePortalTunnel(updated); err != nil {
		return nil, err
	}
	commandLine := nowhere.BuildTunnelURLs(updated)
	remote, err := nowhere.UpdateInstance(existing.EndpointID, *existing.InstanceID, commandLine)
	if err != nil {
		return nil, err
	}
	if req.Name != "" && req.Name != existing.Name {
		if _, err = nowhere.RenameInstance(existing.EndpointID, *existing.InstanceID, req.Name); err != nil {
			return nil, err
		}
	}
	if _, err = nowhere.SetRestartInstance(existing.EndpointID, *existing.InstanceID, req.Restart); err != nil {
		return nil, err
	}
	if err = s.syncMetadata(existing.EndpointID, *existing.InstanceID, desiredTags, desiredPeer); err != nil {
		return nil, err
	}
	updated.CommandLine = commandLine
	applyInstanceState(&updated, remote)
	applySubmittedMetadata(&updated, desiredTags, desiredPeer)
	updates := nowhere.TunnelToMap(&updated)
	if updated.ConfigLine == nil {
		updates["config_line"] = nil
	}
	updates["name"] = updated.Name
	updates["enable_log_store"] = updated.EnableLogStore
	if err = s.db.Model(&models.Tunnel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.recordOperation(&updated, models.OperationActionRename, "Tunnel instance updated")
	return &updated, nil
}

func (s *Service) GetTunnel(id int64) (*models.Tunnel, error) {
	var result models.Tunnel
	err := s.db.Preload("Endpoint").Preload("Groups").Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&result).Error
	return &result, err
}

func (s *Service) GetTunnelsWithPagination(params TunnelQueryParams) (*TunnelListResult, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 200 {
		params.PageSize = 10
	}
	query := s.db.Model(&models.Tunnel{}).Where("tunnels.type = ?", models.TunnelTypePortal)
	if params.Search != "" {
		like := "%" + params.Search + "%"
		query = query.Where("tunnels.name LIKE ? OR tunnels.listen_host LIKE ? OR tunnels.listen_port LIKE ?", like, like, like)
	}
	if params.Status != "" && params.Status != "all" {
		query = query.Where("tunnels.status = ?", params.Status)
	}
	if params.EndpointID != "" && params.EndpointID != "all" {
		query = query.Where("tunnels.endpoint_id = ?", params.EndpointID)
	}
	if params.PortFilter != "" {
		query = query.Where("tunnels.listen_port = ?", params.PortFilter)
	}
	if params.GroupID != "" && params.GroupID != "all" {
		query = query.Joins("JOIN tunnel_groups ON tunnel_groups.tunnel_id = tunnels.id").Where("tunnel_groups.group_id = ?", params.GroupID)
	}
	var total int64
	if err := query.Distinct("tunnels.id").Count(&total).Error; err != nil {
		return nil, err
	}

	allowedSorts := map[string]string{
		"id": "tunnels.id", "sorts": "tunnels.sorts", "type": "tunnels.type", "name": "tunnels.name",
		"endpoint_id": "tunnels.endpoint_id", "status": "tunnels.status", "listen_port": "tunnels.listen_port",
	}
	sortColumn := allowedSorts[params.SortBy]
	if sortColumn == "" {
		sortColumn = "tunnels.sorts"
	}
	sortOrder := "DESC"
	if strings.EqualFold(params.SortOrder, "asc") {
		sortOrder = "ASC"
	}
	var rows []TunnelWithStats
	err := query.Select("tunnels.*, (tunnels.tcp_rx + tunnels.udp_rx) AS total_rx, (tunnels.tcp_tx + tunnels.udp_tx) AS total_tx, endpoints.name AS endpoint_name, COALESCE(endpoints.ver, '') AS endpoint_version, endpoints.hostname AS portal_host").
		Joins("LEFT JOIN endpoints ON endpoints.id = tunnels.endpoint_id").
		Order(sortColumn + " " + sortOrder + ", tunnels.id DESC").
		Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for index := range rows {
		if rows[index].PortalHost == "" {
			var endpointURL string
			_ = s.db.Model(&models.Endpoint{}).Where("id = ?", rows[index].EndpointID).Pluck("url", &endpointURL).Error
			if parsed, parseErr := url.Parse(endpointURL); parseErr == nil {
				rows[index].PortalHost = parsed.Hostname()
			}
		}
		if vectorURL, buildErr := nowhere.BuildVectorURL(rows[index].Tunnel, rows[index].PortalHost, "127.0.0.1:1080"); buildErr == nil {
			rows[index].VectorURL = vectorURL
		}
	}
	return &TunnelListResult{Data: rows, Total: total, Page: params.Page, PageSize: params.PageSize, TotalPages: int(math.Ceil(float64(total) / float64(params.PageSize)))}, nil
}

func (s *Service) DeleteTunnel(id int64) error {
	var tunnel models.Tunnel
	if err := s.db.Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return err
	}
	if err := subscription.NewService(s.db).AccountTunnels([]int64{tunnel.ID}); err != nil {
		return fmt.Errorf("account tunnel subscription traffic: %w", err)
	}
	if tunnel.InstanceID != nil && *tunnel.InstanceID != "" {
		if err := nowhere.DeleteInstance(tunnel.EndpointID, *tunnel.InstanceID); err != nil && !strings.Contains(err.Error(), "404") {
			return err
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tunnel_id = ?", tunnel.ID).Delete(&models.PortalSubscriptionTunnel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnel.ID).Delete(&models.TunnelOperationLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnel.ID).Delete(&models.TunnelGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Tunnel{}, tunnel.ID).Error; err != nil {
			return err
		}
		s.updateEndpointTunnelCount(tunnel.EndpointID)
		return nil
	})
}

func (s *Service) ControlTunnel(req TunnelActionRequest) error {
	var tunnel models.Tunnel
	if err := s.db.Where("instance_id = ? AND type = ?", req.InstanceID, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return err
	}
	result, err := nowhere.ControlInstance(tunnel.EndpointID, req.InstanceID, req.Action)
	if err != nil {
		return err
	}
	if result.Status != "" {
		_ = s.db.Model(&tunnel).Updates(map[string]interface{}{"status": result.Status, "updated_at": time.Now()}).Error
	}
	s.recordOperation(&tunnel, models.OperationAction(req.Action), "Tunnel instance action completed")
	return nil
}

func (s *Service) RenameTunnel(id int64, name string) error {
	var tunnel models.Tunnel
	if err := s.db.Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return err
	}
	if tunnel.InstanceID == nil {
		return errors.New("instance ID is missing")
	}
	if _, err := nowhere.RenameInstance(tunnel.EndpointID, *tunnel.InstanceID, strings.TrimSpace(name)); err != nil {
		return err
	}
	return s.db.Model(&tunnel).Updates(map[string]interface{}{"name": strings.TrimSpace(name), "updated_at": time.Now()}).Error
}

func (s *Service) SetTunnelRestart(id int64, restart bool) error {
	var tunnel models.Tunnel
	if err := s.db.Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return err
	}
	if tunnel.InstanceID == nil {
		return errors.New("instance ID is missing")
	}
	if _, err := nowhere.SetRestartInstance(tunnel.EndpointID, *tunnel.InstanceID, restart); err != nil {
		return err
	}
	return s.db.Model(&tunnel).Updates(map[string]interface{}{"restart": restart, "updated_at": time.Now()}).Error
}

func (s *Service) UpdateTags(id int64, tags map[string]string) error {
	var tunnel models.Tunnel
	if err := s.db.Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return err
	}
	if tunnel.InstanceID == nil {
		return errors.New("instance ID is missing")
	}
	if _, err := nowhere.UpdateInstanceMetadata(tunnel.EndpointID, *tunnel.InstanceID, &tags, tunnel.Peer); err != nil {
		return err
	}
	return s.db.Model(&tunnel).Updates(map[string]interface{}{"tags": &tags, "updated_at": time.Now()}).Error
}

func (s *Service) ResetTunnelTrafficByInstanceID(instanceID string) error {
	var tunnel models.Tunnel
	if err := s.db.Where("instance_id = ? AND type = ?", instanceID, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return err
	}
	if _, err := nowhere.ResetTraffic(tunnel.EndpointID, instanceID); err != nil {
		return err
	}
	return s.db.Model(&tunnel).Updates(map[string]interface{}{"tcp_rx": 0, "tcp_tx": 0, "udp_rx": 0, "udp_tx": 0}).Error
}

func (s *Service) GetInstanceIDByTunnelID(id int64) (string, error) {
	var tunnel models.Tunnel
	if err := s.db.Select("instance_id").Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
		return "", err
	}
	if tunnel.InstanceID == nil {
		return "", errors.New("instance ID is missing")
	}
	return *tunnel.InstanceID, nil
}

func (s *Service) GetEndpointIDByTunnelID(id int64) (int64, error) {
	var tunnel models.Tunnel
	err := s.db.Select("endpoint_id").Where("id = ? AND type = ?", id, models.TunnelTypePortal).First(&tunnel).Error
	return tunnel.EndpointID, err
}

func (s *Service) GetEndpointIDByInstanceID(instanceID string) (int64, error) {
	var tunnel models.Tunnel
	err := s.db.Select("endpoint_id").Where("instance_id = ? AND type = ?", instanceID, models.TunnelTypePortal).First(&tunnel).Error
	return tunnel.EndpointID, err
}

func (s *Service) GetOperationLogs(limit int) ([]OperationLog, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	var logs []OperationLog
	err := s.db.Model(&models.TunnelOperationLog{}).Order("created_at DESC").Limit(limit).Scan(&logs).Error
	return logs, err
}

func (s *Service) ClearOperationLogs() (int64, error) {
	result := s.db.Where("1 = 1").Delete(&models.TunnelOperationLog{})
	return result.RowsAffected, result.Error
}

func (s *Service) UpdateTunnelsSorts(id, sorts int64) error {
	return s.db.Model(&models.Tunnel{}).Where("id = ? AND type = ?", id, models.TunnelTypePortal).Updates(map[string]interface{}{"sorts": sorts, "updated_at": time.Now()}).Error
}

func (s *Service) recordOperation(tunnel *models.Tunnel, action models.OperationAction, message string) {
	_ = s.db.Create(&models.TunnelOperationLog{TunnelID: &tunnel.ID, TunnelName: tunnel.Name, Action: action, Status: "success", Message: &message}).Error
}

func (s *Service) updateEndpointTunnelCount(endpointID int64) {
	var count int64
	if s.db.Model(&models.Tunnel{}).Where("endpoint_id = ? AND type = ?", endpointID, models.TunnelTypePortal).Count(&count).Error == nil {
		_ = s.db.Model(&models.Endpoint{}).Where("id = ?", endpointID).Update("tunnel_count", count).Error
	}
}

func ParseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid tunnel ID")
	}
	return id, nil
}
