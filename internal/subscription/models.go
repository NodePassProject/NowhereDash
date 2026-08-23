package subscription

import "time"

type Preferences struct {
	ExpandCarrierCombos bool   `json:"expandCarrierCombos"`
	UpCarrier           string `json:"upCarrier"`
	DownCarrier         string `json:"downCarrier"`
	IncludeIPv6         bool   `json:"includeIpv6"`
}

type UpsertRequest struct {
	Name         string       `json:"name" binding:"required"`
	ProfileTitle string       `json:"profileTitle"`
	ExpiresAt    *time.Time   `json:"expiresAt"`
	TrafficLimit *int64       `json:"trafficLimit"`
	Preferences  *Preferences `json:"preferences"`
	TunnelIDs    []int64      `json:"tunnelIds"`
}

type Response struct {
	ID              int64       `json:"id"`
	Name            string      `json:"name"`
	ProfileTitle    string      `json:"profileTitle"`
	Token           string      `json:"token"`
	SubscriptionURL string      `json:"subscriptionUrl"`
	ExpiresAt       *time.Time  `json:"expiresAt"`
	TrafficLimit    *int64      `json:"trafficLimit"`
	TrafficUsed     int64       `json:"trafficUsed"`
	OverLimit       bool        `json:"overLimit"`
	Preferences     Preferences `json:"preferences"`
	TunnelIDs       []int64     `json:"tunnelIds"`
	PortalCount     int         `json:"portalCount"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
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
	TrafficUsed       int64             `json:"trafficUsed"`
	Headers           map[string]string `json:"headers"`
}

type RenderedSubscription struct {
	Content     string
	PortalCount int
	Headers     map[string]string
}
