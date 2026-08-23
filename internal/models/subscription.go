package models

import "time"

// PortalSubscription is an administrator-managed, token-authenticated view of
// a selected set of Nowhere Portal instances.
type PortalSubscription struct {
	ID                  int64                      `json:"id" gorm:"primaryKey;autoIncrement;column:id"`
	Name                string                     `json:"name" gorm:"type:text;not null;column:name"`
	Icon                []byte                     `json:"-" gorm:"column:icon"`
	ProfileTitle        string                     `json:"profileTitle" gorm:"type:text;not null;column:profile_title"`
	Token               string                     `json:"token" gorm:"type:text;not null;uniqueIndex;column:token"`
	ExpiresAt           *time.Time                 `json:"expiresAt,omitempty" gorm:"index;column:expires_at"`
	TrafficLimit        *int64                     `json:"trafficLimit,omitempty" gorm:"column:traffic_limit"`
	TrafficUsed         int64                      `json:"trafficUsed" gorm:"not null;default:0;column:traffic_used"`
	OverLimit           bool                       `json:"overLimit" gorm:"not null;default:false;column:over_limit"`
	ExpandCarrierCombos bool                       `json:"expandCarrierCombos" gorm:"not null;column:expand_carrier_combos"`
	UpCarrier           string                     `json:"upCarrier" gorm:"type:text;not null;default:'tcp';column:up_carrier"`
	DownCarrier         string                     `json:"downCarrier" gorm:"type:text;not null;default:'tcp';column:down_carrier"`
	IncludeIPv6         bool                       `json:"includeIpv6" gorm:"not null;default:false;column:include_ipv6"`
	CreatedAt           time.Time                  `json:"createdAt" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt           time.Time                  `json:"updatedAt" gorm:"autoUpdateTime;column:updated_at"`
	SubscriptionTunnels []PortalSubscriptionTunnel `json:"-" gorm:"foreignKey:SubscriptionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (PortalSubscription) TableName() string { return "portal_subscriptions" }

// PortalSubscriptionTunnel stores the accounting cursor for one selected
// Portal. BaselineBytes excludes traffic generated before the association was
// created. AccountedBytes never decreases when the Portal counter is reset.
type PortalSubscriptionTunnel struct {
	ID                int64     `json:"id" gorm:"primaryKey;autoIncrement;column:id"`
	SubscriptionID    int64     `json:"subscriptionId" gorm:"not null;index;uniqueIndex:idx_subscription_tunnel;column:subscription_id"`
	TunnelID          int64     `json:"tunnelId" gorm:"not null;index;uniqueIndex:idx_subscription_tunnel;column:tunnel_id"`
	BaselineBytes     int64     `json:"baselineBytes" gorm:"not null;default:0;column:baseline_bytes"`
	LastObservedBytes int64     `json:"lastObservedBytes" gorm:"not null;default:0;column:last_observed_bytes"`
	AccountedBytes    int64     `json:"accountedBytes" gorm:"not null;default:0;column:accounted_bytes"`
	CreatedAt         time.Time `json:"createdAt" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt         time.Time `json:"updatedAt" gorm:"autoUpdateTime;column:updated_at"`

	Subscription PortalSubscription `json:"-" gorm:"foreignKey:SubscriptionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	Tunnel       Tunnel             `json:"-" gorm:"foreignKey:TunnelID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (PortalSubscriptionTunnel) TableName() string { return "portal_subscription_tunnels" }
