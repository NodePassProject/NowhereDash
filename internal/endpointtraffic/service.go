package endpointtraffic

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	_ "time/tzdata"

	"NowhereDash/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Plan struct {
	Price                *float64 `json:"price"`
	Amount               *string  `json:"amount,omitempty"`
	Currency             string   `json:"currency"`
	MonthlyTrafficLimit  *int64   `json:"monthlyTrafficLimit"`
	TrafficResetDay      int      `json:"trafficResetDay"`
	TrafficResetTime     string   `json:"trafficResetTime"`
	TrafficResetTimezone string   `json:"trafficResetTimezone"`
	TrafficResetDate     string   `json:"trafficResetDate,omitempty"`
	AutoResetTraffic     bool     `json:"autoResetTraffic"`
}

func (p *Plan) Validate() error {
	if p.Amount != nil && len([]rune(*p.Amount)) > 200 {
		return errors.New("amount must not exceed 200 characters")
	}
	if p.TrafficResetDate != "" {
		date, err := time.Parse("2006-01-02", p.TrafficResetDate)
		if err != nil {
			return errors.New("invalid traffic renewal date")
		}
		if p.TrafficResetDay == 0 {
			p.TrafficResetDay = date.Day()
		}
		lastDay := time.Date(date.Year(), date.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if date.Day() != min(p.TrafficResetDay, lastDay) {
			return errors.New("renewal date does not match the monthly renewal day")
		}
	}
	if p.Price != nil && (math.IsNaN(*p.Price) || math.IsInf(*p.Price, 0) || *p.Price < 0 || *p.Price > 1e12) {
		return errors.New("price must be between 0 and 1000000000000")
	}
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.Currency == "" {
		p.Currency = "CNY"
	}
	if len(p.Currency) != 3 || strings.IndexFunc(p.Currency, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
		return errors.New("currency must be a three-letter currency code")
	}
	if p.MonthlyTrafficLimit != nil && (*p.MonthlyTrafficLimit <= 0 || *p.MonthlyTrafficLimit > 9007199254740991) {
		return errors.New("monthly traffic limit must be a positive safe integer")
	}
	if p.TrafficResetDay < 1 || p.TrafficResetDay > 31 {
		return errors.New("traffic reset day must be between 1 and 31")
	}
	if _, err := time.Parse("15:04", p.TrafficResetTime); err != nil {
		return errors.New("traffic reset time must use HH:MM")
	}
	if p.TrafficResetTimezone == "" || p.TrafficResetTimezone == "Local" {
		return errors.New("traffic reset timezone must be an IANA timezone")
	}
	if _, err := time.LoadLocation(p.TrafficResetTimezone); err != nil {
		return errors.New("invalid traffic reset timezone")
	}
	if p.AutoResetTraffic && p.MonthlyTrafficLimit == nil {
		return errors.New("monthly traffic limit is required for automatic renewal")
	}
	return nil
}

func NextReset(ep *models.Endpoint, after time.Time) (time.Time, error) {
	location, err := time.LoadLocation(ep.TrafficResetTimezone)
	if err != nil {
		return time.Time{}, err
	}
	clock, err := time.Parse("15:04", ep.TrafficResetTime)
	if err != nil || ep.TrafficResetDay < 1 || ep.TrafficResetDay > 31 {
		return time.Time{}, errors.New("invalid traffic reset schedule")
	}
	local := after.In(location)
	for offset := 0; offset < 2; offset++ {
		month := time.Date(local.Year(), local.Month()+time.Month(offset), 1, 0, 0, 0, 0, location)
		lastDay := month.AddDate(0, 1, -1).Day()
		day := min(ep.TrafficResetDay, lastDay)
		candidate := time.Date(month.Year(), month.Month(), day, clock.Hour(), clock.Minute(), 0, 0, location)
		if candidate.After(after) {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, errors.New("cannot determine next traffic reset")
}

func (p Plan) Apply(ep *models.Endpoint, now time.Time) error {
	changed := ep.AutoResetTraffic != p.AutoResetTraffic || ep.TrafficResetDay != p.TrafficResetDay ||
		ep.TrafficResetTime != p.TrafficResetTime || ep.TrafficResetTimezone != p.TrafficResetTimezone
	ep.Price, ep.Currency, ep.MonthlyTrafficLimit = p.Price, p.Currency, p.MonthlyTrafficLimit
	ep.Amount = p.Amount
	ep.TrafficResetDay, ep.TrafficResetTime, ep.TrafficResetTimezone = p.TrafficResetDay, p.TrafficResetTime, p.TrafficResetTimezone
	ep.TrafficResetDate = p.TrafficResetDate
	ep.AutoResetTraffic = p.AutoResetTraffic
	if !p.AutoResetTraffic {
		ep.NextTrafficResetAt = nil
	} else if changed || ep.NextTrafficResetAt == nil {
		next, err := NextReset(ep, now)
		if err != nil {
			return err
		}
		ep.NextTrafficResetAt = &next
	}
	if p.AutoResetTraffic && p.TrafficResetDate != "" {
		location, err := time.LoadLocation(p.TrafficResetTimezone)
		if err != nil {
			return err
		}
		first, err := time.ParseInLocation("2006-01-02 15:04", p.TrafficResetDate+" "+p.TrafficResetTime, location)
		if err != nil {
			return err
		}
		if first.After(now) {
			first = first.UTC()
			ep.NextTrafficResetAt = &first
		}
	}
	return nil
}

func PlanUpdates(ep *models.Endpoint) map[string]interface{} {
	return map[string]interface{}{
		"price": ep.Price, "currency": ep.Currency, "monthly_traffic_limit": ep.MonthlyTrafficLimit,
		"amount":            ep.Amount,
		"traffic_reset_day": ep.TrafficResetDay, "traffic_reset_time": ep.TrafficResetTime,
		"traffic_reset_date":     ep.TrafficResetDate,
		"traffic_reset_timezone": ep.TrafficResetTimezone, "auto_reset_traffic": ep.AutoResetTraffic,
		"next_traffic_reset_at": ep.NextTrafficResetAt,
	}
}

func lockEndpoint(tx *gorm.DB, id int64) (*models.Endpoint, error) {
	query := tx
	if tx.Dialector.Name() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	} else {
		// Acquire SQLite's write lock before reading accounting cursors.
		if err := tx.Exec("UPDATE endpoints SET traffic_total = traffic_total WHERE id = ?", id).Error; err != nil {
			return nil, err
		}
	}
	var ep models.Endpoint
	if err := query.First(&ep, id).Error; err != nil {
		return nil, err
	}
	return &ep, nil
}

func add(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

func counterDelta(current, previous int64) int64 {
	current = max(current, 0)
	if current < previous {
		return current // A reset starts a new counter; its first sample still counts.
	}
	return current - previous
}

func observe(tx *gorm.DB, ep *models.Endpoint, sample models.EndpointTrafficCursor) error {
	var previous models.EndpointTrafficCursor
	result := tx.Where("endpoint_id = ? AND instance_id = ?", ep.ID, sample.InstanceID).Limit(1).Find(&previous)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 && !sample.ObservedAt.After(previous.ObservedAt) {
		return nil
	}
	sample.TCPRx, sample.TCPTx = max(sample.TCPRx, 0), max(sample.TCPTx, 0)
	sample.UDPRx, sample.UDPTx = max(sample.UDPRx, 0), max(sample.UDPTx, 0)
	delta := add(add(counterDelta(sample.TCPRx, previous.TCPRx), counterDelta(sample.TCPTx, previous.TCPTx)),
		add(counterDelta(sample.UDPRx, previous.UDPRx), counterDelta(sample.UDPTx, previous.UDPTx)))
	sample.ID, sample.EndpointID = previous.ID, ep.ID
	if err := tx.Save(&sample).Error; err != nil {
		return err
	}
	ep.TrafficTotal = add(ep.TrafficTotal, delta)
	if ep.LastTrafficResetAt == nil || !sample.ObservedAt.Before(*ep.LastTrafficResetAt) {
		ep.TrafficUsed = add(ep.TrafficUsed, delta)
	}
	return nil
}

func syncStored(tx *gorm.DB, ep *models.Endpoint) error {
	var tunnels []models.Tunnel
	if err := tx.Where("endpoint_id = ? AND type = ?", ep.ID, models.TunnelTypePortal).Find(&tunnels).Error; err != nil {
		return err
	}
	for _, tunnel := range tunnels {
		if tunnel.InstanceID == nil || *tunnel.InstanceID == "" {
			continue
		}
		at := tunnel.UpdatedAt
		if tunnel.LastEventTime.Valid {
			at = tunnel.LastEventTime.Time
		}
		if err := observe(tx, ep, models.EndpointTrafficCursor{
			InstanceID: *tunnel.InstanceID, ObservedAt: at,
			TCPRx: tunnel.TCPRx, TCPTx: tunnel.TCPTx, UDPRx: tunnel.UDPRx, UDPTx: tunnel.UDPTx,
		}); err != nil {
			return err
		}
	}
	ep.TrafficInitialized = true
	return nil
}

func resetDue(ep *models.Endpoint, now time.Time) error {
	if !ep.AutoResetTraffic || ep.NextTrafficResetAt == nil || ep.NextTrafficResetAt.After(now) {
		return nil
	}
	next, err := NextReset(ep, now)
	if err != nil {
		return err
	}
	// Compute the most recent boundary even after several months of downtime.
	location, _ := time.LoadLocation(ep.TrafficResetTimezone)
	month := time.Date(now.In(location).Year(), now.In(location).Month(), 1, 0, 0, 0, 0, location)
	last, err := NextReset(ep, month.AddDate(0, -1, 0).Add(-time.Nanosecond))
	if err != nil {
		return err
	}
	if candidate, err := NextReset(ep, last); err == nil && !candidate.After(now) {
		last = candidate
	}
	ep.TrafficUsed, ep.LastTrafficResetAt, ep.NextTrafficResetAt = 0, &last, &next
	return nil
}

func saveUsage(tx *gorm.DB, ep *models.Endpoint) error {
	return tx.Model(&models.Endpoint{}).Where("id = ?", ep.ID).Updates(map[string]interface{}{
		"traffic_total": ep.TrafficTotal, "traffic_used": ep.TrafficUsed, "traffic_initialized": ep.TrafficInitialized,
		"last_traffic_reset_at": ep.LastTrafficResetAt, "next_traffic_reset_at": ep.NextTrafficResetAt,
	}).Error
}

// Observe persists independent counters before Portal state is overwritten or deleted.
func Observe(database *gorm.DB, sample models.EndpointTrafficCursor, now time.Time) error {
	return database.Transaction(func(tx *gorm.DB) error {
		ep, err := lockEndpoint(tx, sample.EndpointID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if !ep.TrafficInitialized {
			if err := syncStored(tx, ep); err != nil {
				return err
			}
		}
		if err := resetDue(ep, now); err != nil {
			return err
		}
		if err := observe(tx, ep, sample); err != nil {
			return err
		}
		return saveUsage(tx, ep)
	})
}

func SyncEndpoint(database *gorm.DB, id int64, now time.Time) error {
	return database.Transaction(func(tx *gorm.DB) error {
		ep, err := lockEndpoint(tx, id)
		if err != nil {
			return err
		}
		if err := resetDue(ep, now); err != nil {
			return err
		}
		if err := syncStored(tx, ep); err != nil {
			return err
		}
		return saveUsage(tx, ep)
	})
}

func SyncAll(database *gorm.DB, now time.Time) error {
	var ids []int64
	if err := database.Model(&models.Endpoint{}).Order("id").Pluck("id", &ids).Error; err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := SyncEndpoint(database, id, now); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			errs = append(errs, fmt.Errorf("endpoint %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func UpdatePlan(database *gorm.DB, id int64, plan Plan, now time.Time) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	return database.Transaction(func(tx *gorm.DB) error {
		ep, err := lockEndpoint(tx, id)
		if err != nil {
			return err
		}
		if err := resetDue(ep, now); err != nil {
			return err
		}
		if err := syncStored(tx, ep); err != nil {
			return err
		}
		if err := plan.Apply(ep, now); err != nil {
			return err
		}
		if err := saveUsage(tx, ep); err != nil {
			return err
		}
		return tx.Model(&models.Endpoint{}).Where("id = ?", id).Updates(PlanUpdates(ep)).Error
	})
}
