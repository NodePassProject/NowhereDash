package subscription

import (
	log "NowhereDash/internal/log"
	"NowhereDash/internal/models"
	"NowhereDash/internal/nowhere"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var (
	ErrNotFound               = errors.New("subscription not found")
	ErrEntitlementUnavailable = errors.New("subscription entitlement is unavailable")
)

type Service struct {
	db              *gorm.DB
	now             func() time.Time
	controlInstance func(endpointID int64, instanceID, action string) (nowhere.InstanceResult, error)
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now, controlInstance: nowhere.ControlInstance}
}

type EntitlementEnforcer struct {
	service  *Service
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
}

func NewEntitlementEnforcer(db *gorm.DB, interval time.Duration) *EntitlementEnforcer {
	if interval <= 0 {
		interval = time.Minute
	}
	return &EntitlementEnforcer{
		service: NewService(db), interval: interval,
		stop: make(chan struct{}), done: make(chan struct{}),
	}
}

func (e *EntitlementEnforcer) Start() {
	log.Infof("订阅授权执行器已启动，检查间隔: %v", e.interval)
	go func() {
		defer close(e.done)
		e.runOnce()

		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				e.runOnce()
			case <-e.stop:
				log.Info("订阅授权执行器已停止")
				return
			}
		}
	}()
}

func (e *EntitlementEnforcer) Stop() {
	e.once.Do(func() {
		close(e.stop)
		<-e.done
	})
}

func (e *EntitlementEnforcer) runOnce() {
	if err := e.service.EnforceAllEntitlements(); err != nil {
		log.Warnf("订阅授权执行失败: %v", err)
	}
}

func (s *Service) Create(req UpsertRequest) (*Response, error) {
	values, tunnelIDs, err := s.validateRequest(req)
	if err != nil {
		return nil, err
	}

	var created models.PortalSubscription
	err = silentDB(s.db).Transaction(func(tx *gorm.DB) error {
		token, tokenErr := generateUniqueToken(tx, 0)
		if tokenErr != nil {
			return tokenErr
		}
		created = models.PortalSubscription{
			Name: values.Name, ProfileTitle: values.ProfileTitle, Token: token,
			ExpiresAt: values.ExpiresAt, TrafficLimit: values.TrafficLimit,
			ExpandCarrierCombos: values.ExpandCarrierCombos,
			UpCarrier:           values.UpCarrier, DownCarrier: values.DownCarrier, IncludeIPv6: values.IncludeIPv6,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		return replaceTunnelLinks(tx, &created, tunnelIDs)
	})
	if err != nil {
		return nil, err
	}
	s.enforceEntitlementBestEffort(created.ID)
	return s.Get(created.ID)
}

func (s *Service) Update(id int64, req UpsertRequest) (*Response, error) {
	if err := s.accountAndEnforce(id); err != nil {
		return nil, err
	}
	values, tunnelIDs, err := s.validateRequest(req)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var current models.PortalSubscription
		query := lockRows(tx).Where("id = ?", id).First(&current)
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if query.Error != nil {
			return query.Error
		}
		overLimit := isOverLimit(current.TrafficUsed, values.TrafficLimit)
		updates := map[string]interface{}{
			"name": values.Name, "profile_title": values.ProfileTitle,
			"expires_at": values.ExpiresAt, "traffic_limit": values.TrafficLimit, "over_limit": overLimit,
			"expand_carrier_combos": values.ExpandCarrierCombos, "up_carrier": values.UpCarrier,
			"down_carrier": values.DownCarrier, "include_ipv6": values.IncludeIPv6, "updated_at": s.now(),
		}
		if err := tx.Model(&current).Updates(updates).Error; err != nil {
			return err
		}
		return replaceTunnelLinks(tx, &current, tunnelIDs)
	})
	if err != nil {
		return nil, err
	}
	s.enforceEntitlementBestEffort(id)
	return s.Get(id)
}

func (s *Service) Delete(id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.PortalSubscription{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
		if err := tx.Where("subscription_id = ?", id).Delete(&models.PortalSubscriptionTunnel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.PortalSubscription{}, id).Error
	})
}

func (s *Service) List() (*ListResponse, error) {
	var ids []int64
	if err := s.db.Model(&models.PortalSubscription{}).Order("id DESC").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	result := &ListResponse{Data: make([]Response, 0, len(ids)), Total: len(ids)}
	for _, id := range ids {
		item, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, *item)
	}
	return result, nil
}

func (s *Service) Get(id int64) (*Response, error) {
	if err := s.accountAndEnforce(id); err != nil {
		return nil, err
	}
	var subscription models.PortalSubscription
	if err := s.db.First(&subscription, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var tunnelIDs []int64
	if err := s.db.Model(&models.PortalSubscriptionTunnel{}).
		Where("subscription_id = ?", id).Order("id ASC").Pluck("tunnel_id", &tunnelIDs).Error; err != nil {
		return nil, err
	}
	if tunnelIDs == nil {
		tunnelIDs = []int64{}
	}
	return responseFromModel(subscription, tunnelIDs), nil
}

func (s *Service) RotateToken(id int64) (*RotateResponse, error) {
	var response RotateResponse
	err := silentDB(s.db).Transaction(func(tx *gorm.DB) error {
		var subscription models.PortalSubscription
		result := lockRows(tx).First(&subscription, id)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		token, err := generateUniqueToken(tx, id)
		if err != nil {
			return err
		}
		now := s.now()
		if err := tx.Model(&subscription).Updates(map[string]interface{}{"token": token, "updated_at": now}).Error; err != nil {
			return err
		}
		response = RotateResponse{Token: token, SubscriptionURL: subscriptionURL(token), UpdatedAt: now}
		return nil
	})
	return &response, err
}

func (s *Service) ResetTraffic(id int64) (*Response, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var subscription models.PortalSubscription
		result := lockRows(tx).First(&subscription, id)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		var links []models.PortalSubscriptionTunnel
		if err := lockRows(tx).Preload("Tunnel").Where("subscription_id = ?", id).Find(&links).Error; err != nil {
			return err
		}
		for _, link := range links {
			observed := tunnelTraffic(&link.Tunnel)
			if err := tx.Model(&link).Updates(map[string]interface{}{
				"baseline_bytes": observed, "last_observed_bytes": observed,
				"accounted_bytes": 0, "updated_at": s.now(),
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&subscription).Updates(map[string]interface{}{
			"traffic_used": 0, "over_limit": false, "updated_at": s.now(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *Service) AccountTraffic(id int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var subscription models.PortalSubscription
		result := lockRows(tx).First(&subscription, id)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		var links []models.PortalSubscriptionTunnel
		if err := lockRows(tx).Preload("Tunnel").Where("subscription_id = ?", id).Find(&links).Error; err != nil {
			return err
		}
		var deltaTotal int64
		for _, link := range links {
			observed := tunnelTraffic(&link.Tunnel)
			delta := int64(0)
			if observed > link.LastObservedBytes {
				delta = observed - link.LastObservedBytes
			}
			accounted := saturatingAdd(link.AccountedBytes, delta)
			if observed != link.LastObservedBytes || delta != 0 {
				if err := tx.Model(&link).Updates(map[string]interface{}{
					"last_observed_bytes": observed, "accounted_bytes": accounted, "updated_at": s.now(),
				}).Error; err != nil {
					return err
				}
			}
			deltaTotal = saturatingAdd(deltaTotal, delta)
		}
		used := saturatingAdd(subscription.TrafficUsed, deltaTotal)
		overLimit := isOverLimit(used, subscription.TrafficLimit)
		if used == subscription.TrafficUsed && overLimit == subscription.OverLimit {
			return nil
		}
		return tx.Model(&subscription).Updates(map[string]interface{}{
			"traffic_used": used, "over_limit": overLimit, "updated_at": s.now(),
		}).Error
	})
}

func (s *Service) accountAndEnforce(id int64) error {
	if err := s.AccountTraffic(id); err != nil {
		return err
	}
	s.enforceEntitlementBestEffort(id)
	return nil
}

func (s *Service) enforceEntitlementBestEffort(id int64) {
	if err := s.EnforceEntitlement(id); err != nil && !errors.Is(err, ErrNotFound) {
		log.Warnf("订阅 %d 授权执行失败: %v", id, err)
	}
}

func (s *Service) EnforceAllEntitlements() error {
	var ids []int64
	if err := s.db.Model(&models.PortalSubscription{}).Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := s.AccountTraffic(id); err != nil {
			errs = append(errs, fmt.Errorf("account subscription %d traffic: %w", id, err))
			continue
		}
		if err := s.EnforceEntitlement(id); err != nil && !errors.Is(err, ErrNotFound) {
			errs = append(errs, fmt.Errorf("enforce subscription %d entitlement: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) EnforceEntitlement(id int64) error {
	var subscription models.PortalSubscription
	if err := s.db.First(&subscription, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	reason := entitlementReason(&subscription, s.now())
	if reason == "" {
		return nil
	}
	return s.stopRunningSubscriptionTunnels(subscription.ID, reason)
}

func (s *Service) stopRunningSubscriptionTunnels(subscriptionID int64, reason string) error {
	var links []models.PortalSubscriptionTunnel
	err := s.db.Preload("Tunnel").
		Joins("JOIN tunnels ON tunnels.id = portal_subscription_tunnels.tunnel_id").
		Where("portal_subscription_tunnels.subscription_id = ?", subscriptionID).
		Where("tunnels.type = ? AND tunnels.status = ?", models.TunnelTypePortal, models.TunnelStatusRunning).
		Where("tunnels.instance_id IS NOT NULL AND tunnels.instance_id <> ''").
		Order("portal_subscription_tunnels.id ASC").Find(&links).Error
	if err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}

	var errs []error
	for _, link := range links {
		tunnel := link.Tunnel
		if tunnel.InstanceID == nil || strings.TrimSpace(*tunnel.InstanceID) == "" {
			continue
		}
		instanceID := strings.TrimSpace(*tunnel.InstanceID)
		result, err := s.controlInstance(tunnel.EndpointID, instanceID, "stop")
		if err != nil {
			errs = append(errs, fmt.Errorf("stop tunnel %d instance %s: %w", tunnel.ID, instanceID, err))
			continue
		}

		status := models.TunnelStatusStopped
		if strings.TrimSpace(result.Status) != "" {
			status = models.TunnelStatus(result.Status)
		}
		if err := s.db.Model(&models.Tunnel{}).Where("id = ?", tunnel.ID).Updates(map[string]interface{}{
			"status": status, "updated_at": s.now(),
		}).Error; err != nil {
			errs = append(errs, fmt.Errorf("update stopped tunnel %d: %w", tunnel.ID, err))
			continue
		}

		message := fmt.Sprintf("Subscription entitlement %s; tunnel stopped", reason)
		_ = s.db.Create(&models.TunnelOperationLog{
			TunnelID: &tunnel.ID, TunnelName: tunnel.Name,
			Action: models.OperationActionStop, Status: "success", Message: &message,
		}).Error
		log.Infof("订阅 %d 因 %s 停止隧道 %d (%s)", subscriptionID, reason, tunnel.ID, instanceID)
	}
	return errors.Join(errs...)
}

// AccountTunnels flushes the latest nonnegative counter deltas before Portal
// rows or their subscription links are deleted.
func (s *Service) AccountTunnels(tunnelIDs []int64) error {
	if len(tunnelIDs) == 0 {
		return nil
	}
	var subscriptionIDs []int64
	if err := s.db.Model(&models.PortalSubscriptionTunnel{}).
		Distinct("subscription_id").Where("tunnel_id IN ?", tunnelIDs).
		Order("subscription_id ASC").Pluck("subscription_id", &subscriptionIDs).Error; err != nil {
		return err
	}
	for _, subscriptionID := range subscriptionIDs {
		if err := s.AccountTraffic(subscriptionID); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

func (s *Service) Preview(id int64) (*PreviewResponse, error) {
	if err := s.accountAndEnforce(id); err != nil {
		return nil, err
	}
	subscription, rendered, reason, err := s.renderByID(id)
	if err != nil {
		return nil, err
	}
	available := reason == "" && rendered.PortalCount > 0
	if reason == "" && rendered.PortalCount == 0 {
		reason = "no_running_portals"
	}
	return &PreviewResponse{
		Available: available, UnavailableReason: reason, Content: rendered.Content,
		PortalCount: rendered.PortalCount, TrafficUsed: subscription.TrafficUsed, Headers: rendered.Headers,
	}, nil
}

func (s *Service) RenderPublic(token string) (*RenderedSubscription, error) {
	if !validToken(token) {
		return nil, ErrNotFound
	}
	var existing models.PortalSubscription
	if err := silentDB(s.db).Select("id").Where("token = ?", token).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.AccountTraffic(existing.ID); err != nil {
		return nil, err
	}
	s.enforceEntitlementBestEffort(existing.ID)
	subscription, rendered, reason, err := s.renderByIDAndToken(existing.ID, token)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return nil, ErrEntitlementUnavailable
	}
	if subscription == nil || rendered.PortalCount == 0 {
		return nil, ErrNotFound
	}
	return &rendered, nil
}

func (s *Service) renderByID(id int64) (*models.PortalSubscription, RenderedSubscription, string, error) {
	return s.render(id, "")
}

func (s *Service) renderByIDAndToken(id int64, token string) (*models.PortalSubscription, RenderedSubscription, string, error) {
	return s.render(id, token)
}

func (s *Service) render(id int64, token string) (*models.PortalSubscription, RenderedSubscription, string, error) {
	var subscription models.PortalSubscription
	query := s.db.Where("id = ?", id)
	if err := query.First(&subscription).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, RenderedSubscription{}, "", ErrNotFound
		}
		return nil, RenderedSubscription{}, "", err
	}
	if token != "" && subtle.ConstantTimeCompare([]byte(subscription.Token), []byte(token)) != 1 {
		return nil, RenderedSubscription{}, "", ErrNotFound
	}
	reason := entitlementReason(&subscription, s.now())
	rendered, err := s.renderContent(subscription)
	return &subscription, rendered, reason, err
}

func (s *Service) renderContent(subscription models.PortalSubscription) (RenderedSubscription, error) {
	var links []models.PortalSubscriptionTunnel
	err := s.db.Preload("Tunnel.Endpoint").
		Joins("JOIN tunnels ON tunnels.id = portal_subscription_tunnels.tunnel_id").
		Where("portal_subscription_tunnels.subscription_id = ?", subscription.ID).
		Where("tunnels.type = ? AND tunnels.status = ?", models.TunnelTypePortal, models.TunnelStatusRunning).
		Order("portal_subscription_tunnels.id ASC").Find(&links).Error
	if err != nil {
		return RenderedSubscription{}, err
	}
	preferences := preferencesFromModel(subscription)
	lines := make([]string, 0, len(links))
	portalCount := 0
	for _, link := range links {
		portalLines := renderPortal(&link.Tunnel, preferences)
		if len(portalLines) > 0 {
			portalCount++
			lines = append(lines, portalLines...)
		}
	}
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	return RenderedSubscription{
		Content: content, PortalCount: portalCount, Headers: subscriptionHeaders(subscription),
	}, nil
}

type validatedRequest struct {
	Name, ProfileTitle     string
	ExpiresAt              *time.Time
	TrafficLimit           *int64
	ExpandCarrierCombos    bool
	UpCarrier, DownCarrier string
	IncludeIPv6            bool
}

func (s *Service) validateRequest(req UpsertRequest) (validatedRequest, []int64, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]byte(name)) > 255 {
		return validatedRequest{}, nil, errors.New("name must contain 1 to 255 bytes")
	}
	profileTitle := strings.TrimSpace(req.ProfileTitle)
	if profileTitle == "" {
		profileTitle = name
	}
	if len([]byte(profileTitle)) > 255 {
		return validatedRequest{}, nil, errors.New("profileTitle must contain at most 255 bytes")
	}
	if req.TrafficLimit != nil && *req.TrafficLimit < 0 {
		return validatedRequest{}, nil, errors.New("trafficLimit must be nonnegative or null")
	}
	preferences := Preferences{ExpandCarrierCombos: true, UpCarrier: "tcp", DownCarrier: "tcp"}
	if req.Preferences != nil {
		preferences = *req.Preferences
		preferences.UpCarrier = strings.ToLower(strings.TrimSpace(preferences.UpCarrier))
		preferences.DownCarrier = strings.ToLower(strings.TrimSpace(preferences.DownCarrier))
		if preferences.UpCarrier == "" {
			preferences.UpCarrier = "tcp"
		}
		if preferences.DownCarrier == "" {
			preferences.DownCarrier = "tcp"
		}
	}
	if !validCarrier(preferences.UpCarrier) || !validCarrier(preferences.DownCarrier) {
		return validatedRequest{}, nil, errors.New("upCarrier and downCarrier must be tcp or udp")
	}
	tunnelIDs := uniquePositiveIDs(req.TunnelIDs)
	if len(tunnelIDs) != len(req.TunnelIDs) {
		return validatedRequest{}, nil, errors.New("tunnelIds must contain unique positive IDs")
	}
	if len(tunnelIDs) > 0 {
		var count int64
		if err := s.db.Model(&models.Tunnel{}).Where("id IN ? AND type = ?", tunnelIDs, models.TunnelTypePortal).Count(&count).Error; err != nil {
			return validatedRequest{}, nil, err
		}
		if count != int64(len(tunnelIDs)) {
			return validatedRequest{}, nil, errors.New("all tunnelIds must reference existing tunnels")
		}
	}
	return validatedRequest{
		Name: name, ProfileTitle: profileTitle, ExpiresAt: req.ExpiresAt,
		TrafficLimit: req.TrafficLimit, ExpandCarrierCombos: preferences.ExpandCarrierCombos,
		UpCarrier: preferences.UpCarrier, DownCarrier: preferences.DownCarrier, IncludeIPv6: preferences.IncludeIPv6,
	}, tunnelIDs, nil
}

func replaceTunnelLinks(tx *gorm.DB, subscription *models.PortalSubscription, tunnelIDs []int64) error {
	var current []models.PortalSubscriptionTunnel
	if err := tx.Where("subscription_id = ?", subscription.ID).Find(&current).Error; err != nil {
		return err
	}
	wanted := make(map[int64]struct{}, len(tunnelIDs))
	for _, id := range tunnelIDs {
		wanted[id] = struct{}{}
	}
	for _, link := range current {
		if _, keep := wanted[link.TunnelID]; keep {
			delete(wanted, link.TunnelID)
			continue
		}
		if err := tx.Delete(&link).Error; err != nil {
			return err
		}
	}
	for _, tunnelID := range tunnelIDs {
		if _, create := wanted[tunnelID]; !create {
			continue
		}
		var tunnel models.Tunnel
		if err := tx.Where("id = ? AND type = ?", tunnelID, models.TunnelTypePortal).First(&tunnel).Error; err != nil {
			return err
		}
		observed := tunnelTraffic(&tunnel)
		link := models.PortalSubscriptionTunnel{
			SubscriptionID: subscription.ID, TunnelID: tunnelID,
			BaselineBytes: observed, LastObservedBytes: observed,
		}
		if err := tx.Create(&link).Error; err != nil {
			return err
		}
	}
	return nil
}

func responseFromModel(subscription models.PortalSubscription, tunnelIDs []int64) *Response {
	return &Response{
		ID: subscription.ID, Name: subscription.Name, ProfileTitle: subscription.ProfileTitle,
		Token: subscription.Token, SubscriptionURL: subscriptionURL(subscription.Token),
		ExpiresAt: subscription.ExpiresAt, TrafficLimit: subscription.TrafficLimit,
		TrafficUsed: subscription.TrafficUsed, OverLimit: subscription.OverLimit,
		Preferences: preferencesFromModel(subscription), TunnelIDs: tunnelIDs, PortalCount: len(tunnelIDs),
		CreatedAt: subscription.CreatedAt, UpdatedAt: subscription.UpdatedAt,
	}
}

func preferencesFromModel(subscription models.PortalSubscription) Preferences {
	return Preferences{
		ExpandCarrierCombos: subscription.ExpandCarrierCombos, UpCarrier: subscription.UpCarrier,
		DownCarrier: subscription.DownCarrier, IncludeIPv6: subscription.IncludeIPv6,
	}
}

func generateUniqueToken(tx *gorm.DB, excludeID int64) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("generate subscription token: %w", err)
		}
		token := base64.RawURLEncoding.EncodeToString(bytes)
		var count int64
		query := tx.Model(&models.PortalSubscription{}).Where("token = ?", token)
		if excludeID > 0 {
			query = query.Where("id <> ?", excludeID)
		}
		if err := query.Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return token, nil
		}
	}
	return "", errors.New("could not allocate a unique subscription token")
}

func validToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	for _, character := range token {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func subscriptionURL(token string) string {
	return "/sub/portal?token=" + url.QueryEscape(token)
}

func entitlementReason(subscription *models.PortalSubscription, now time.Time) string {
	if subscription.ExpiresAt != nil && !now.Before(*subscription.ExpiresAt) {
		return "expired"
	}
	if subscription.OverLimit || isOverLimit(subscription.TrafficUsed, subscription.TrafficLimit) {
		return "over_limit"
	}
	return ""
}

func isOverLimit(used int64, limit *int64) bool {
	return limit != nil && *limit > 0 && used >= *limit
}

func tunnelTraffic(tunnel *models.Tunnel) int64 {
	total := int64(0)
	for _, value := range []int64{tunnel.TCPRx, tunnel.TCPTx, tunnel.UDPRx, tunnel.UDPTx} {
		if value > 0 {
			total = saturatingAdd(total, value)
		}
	}
	return total
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}

func uniquePositiveIDs(ids []int64) []int64 {
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func lockRows(db *gorm.DB) *gorm.DB {
	if db.Dialector.Name() == "postgres" {
		return db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return db
}

func silentDB(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)})
}

type carrierCombo struct{ up, down string }

func carrierCombos(network string, preferences Preferences) []carrierCombo {
	switch strings.ToLower(network) {
	case "tcp":
		if !preferences.ExpandCarrierCombos && (preferences.UpCarrier != "tcp" || preferences.DownCarrier != "tcp") {
			return nil
		}
		return []carrierCombo{{up: "tcp", down: "tcp"}}
	case "udp":
		if !preferences.ExpandCarrierCombos && (preferences.UpCarrier != "udp" || preferences.DownCarrier != "udp") {
			return nil
		}
		return []carrierCombo{{up: "udp", down: "udp"}}
	}
	if !preferences.ExpandCarrierCombos {
		return []carrierCombo{{up: preferences.UpCarrier, down: preferences.DownCarrier}}
	}
	return []carrierCombo{
		{up: "tcp", down: "tcp"}, {up: "tcp", down: "udp"},
		{up: "udp", down: "tcp"}, {up: "udp", down: "udp"},
	}
}

func validCarrier(carrier string) bool { return carrier == "tcp" || carrier == "udp" }

func renderPortal(tunnel *models.Tunnel, preferences Preferences) []string {
	if tunnel.SharedKey == nil || *tunnel.SharedKey == "" {
		return nil
	}
	hosts := portalHosts(tunnel, preferences.IncludeIPv6)
	if len(hosts) == 0 {
		return nil
	}
	port, err := strconv.Atoi(tunnel.ListenPort)
	if err != nil || port < 1 || port > 65535 {
		return nil
	}
	combos := carrierCombos(valueOr(tunnel.Network, "mix"), preferences)
	lines := make([]string, 0, len(hosts)*len(combos))
	for _, host := range hosts {
		for _, combo := range combos {
			name := tunnel.Name
			if isIPv6Host(host) {
				name += " | v6"
			}
			if len(combos) > 1 {
				name += " | " + strings.ToUpper(combo.up) + "/" + strings.ToUpper(combo.down)
			}
			query := "up=" + combo.up + "&down=" + combo.down
			if combo.up == "tcp" && combo.down == "tcp" {
				query += "&pool=5"
			}
			if tunnel.ALPN != nil && *tunnel.ALPN != "" {
				query += "&alpn=" + percentEncode(*tunnel.ALPN)
			}
			line := "nowhere://" + percentEncode(*tunnel.SharedKey) + "@" +
				net.JoinHostPort(host, tunnel.ListenPort) + "?" + query + "#" + percentEncode(name)
			lines = append(lines, line)
		}
	}
	return lines
}

func portalHost(tunnel *models.Tunnel, includeIPv6 bool) string {
	hosts := portalHosts(tunnel, includeIPv6)
	if len(hosts) == 0 {
		return ""
	}
	return hosts[0]
}

func portalHosts(tunnel *models.Tunnel, includeIPv6 bool) []string {
	listenHost := cleanHost(tunnel.ListenHost)
	candidates := []string{listenHost}
	if listenHost == "" || listenHost == "*" || isUnspecifiedHost(listenHost) {
		candidates = []string{tunnel.Endpoint.Hostname}
		if parsed, err := url.Parse(tunnel.Endpoint.URL); err == nil {
			candidates = append(candidates, parsed.Hostname())
		}
	}
	seen := make(map[string]struct{}, len(candidates))
	baseHost, ipv6Host := "", ""
	for _, candidate := range candidates {
		host := cleanHost(candidate)
		if host == "" || host == "*" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			if ip.IsUnspecified() {
				continue
			}
			host = ip.String()
		}
		key := strings.ToLower(host)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if isIPv6Host(host) {
			if includeIPv6 && ipv6Host == "" {
				ipv6Host = host
			}
			continue
		}
		if baseHost == "" {
			baseHost = host
		}
	}
	hosts := make([]string, 0, 2)
	if baseHost != "" {
		hosts = append(hosts, baseHost)
	}
	if includeIPv6 && ipv6Host != "" {
		hosts = append(hosts, ipv6Host)
	}
	return hosts
}

func isIPv6Host(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() == nil
}

func isUnspecifiedHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func cleanHost(host string) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" || strings.ContainsAny(host, " /\\@?#") {
		return ""
	}
	return host
}

func percentEncode(value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var result strings.Builder
	for _, character := range []byte(value) {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '.' || character == '_' || character == '~' {
			result.WriteByte(character)
			continue
		}
		result.WriteByte('%')
		result.WriteByte(hexadecimal[character>>4])
		result.WriteByte(hexadecimal[character&15])
	}
	return result.String()
}

func subscriptionHeaders(subscription models.PortalSubscription) map[string]string {
	title := base64.StdEncoding.EncodeToString([]byte(subscription.ProfileTitle))
	total := int64(-1)
	if subscription.TrafficLimit != nil {
		total = *subscription.TrafficLimit
	}
	parts := []string{
		"upload=0", "download=" + strconv.FormatInt(subscription.TrafficUsed, 10),
		"total=" + strconv.FormatInt(total, 10),
	}
	if subscription.ExpiresAt != nil {
		parts = append(parts, "expire="+strconv.FormatInt(subscription.ExpiresAt.Unix(), 10))
	}
	return map[string]string{
		"profile-title":          "base64:" + title,
		"subscription-userinfo":  strings.Join(parts, "; "),
		"cache-control":          "no-store",
		"x-content-type-options": "nosniff",
	}
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}
