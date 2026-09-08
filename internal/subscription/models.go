package subscription

import "time"

type Preferences struct {
	ExpandCarrierCombos bool   `json:"expandCarrierCombos"`
	UpCarrier           string `json:"upCarrier"`
	DownCarrier         string `json:"downCarrier"`
	IncludeIPv6         bool   `json:"includeIpv6"`
}

type NodeOrderItem struct {
	Source   string `json:"source"`
	TunnelID int64  `json:"tunnelId,omitempty"`
	URI      string `json:"uri,omitempty"`
}

type UpsertRequest struct {
	Name         string           `json:"name" binding:"required"`
	Icon         *string          `json:"icon"`
	ProfileTitle string           `json:"profileTitle"`
	ExpiresAt    *time.Time       `json:"expiresAt"`
	TrafficLimit *int64           `json:"trafficLimit"`
	Preferences  *Preferences     `json:"preferences"`
	TunnelIDs    []int64          `json:"tunnelIds"`
	TunnelNames  map[int64]string `json:"tunnelNames"`
	ExternalURIs []string         `json:"externalUris"`
	NodeOrder    []NodeOrderItem  `json:"nodeOrder"`
}

type Response struct {
	ID                int64            `json:"id"`
	Name              string           `json:"name"`
	Icon              string           `json:"icon"`
	ProfileTitle      string           `json:"profileTitle"`
	Token             string           `json:"token"`
	SubscriptionURL   string           `json:"subscriptionUrl"`
	ExpiresAt         *time.Time       `json:"expiresAt"`
	TrafficLimit      *int64           `json:"trafficLimit"`
	TrafficUsed       int64            `json:"trafficUsed"`
	OverLimit         bool             `json:"overLimit"`
	Preferences       Preferences      `json:"preferences"`
	TunnelIDs         []int64          `json:"tunnelIds"`
	TunnelNames       map[int64]string `json:"tunnelNames"`
	PortalCount       int              `json:"portalCount"`
	ExternalURIs      []string         `json:"externalUris"`
	NodeOrder         []NodeOrderItem  `json:"nodeOrder"`
	ExternalNodeCount int              `json:"externalNodeCount"`
	NodeCount         int              `json:"nodeCount"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
}

type ListResponse struct {
	Data  []Response `json:"data"`
	Total int        `json:"total"`
}

type RotateResponse struct {
	Token           string    `json:"token"`
	SubscriptionURL string    `json:"subscriptionUrl"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type PreviewResponse struct {
	Available         bool              `json:"available"`
	UnavailableReason string            `json:"unavailableReason"`
	Content           string            `json:"content"`
	PortalCount       int               `json:"portalCount"`
	ExternalNodeCount int               `json:"externalNodeCount"`
	NodeCount         int               `json:"nodeCount"`
	TrafficUsed       int64             `json:"trafficUsed"`
	Headers           map[string]string `json:"headers"`
}

type RenderedSubscription struct {
	Content           string
	PortalCount       int
	ExternalNodeCount int
	NodeCount         int
	Headers           map[string]string
}
