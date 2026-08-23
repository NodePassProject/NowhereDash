package tunnel

import "NowhereDash/internal/models"

type Tunnel = models.Tunnel
type TunnelStatus = models.TunnelStatus
type TunnelType = models.TunnelType
type TLSMode = models.TLSMode
type LogLevel = models.LogLevel

const (
	StatusRunning = models.TunnelStatusRunning
	StatusStopped = models.TunnelStatusStopped
	StatusError   = models.TunnelStatusError
	StatusOffline = models.TunnelStatusOffline
)

type TunnelQueryParams struct {
	Search     string `json:"search"`
	Status     string `json:"status"`
	EndpointID string `json:"endpoint_id"`
	PortFilter string `json:"port_filter"`
	GroupID    string `json:"group_id"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
	SortBy     string `json:"sort_by"`
	SortOrder  string `json:"sort_order"`
}

type TunnelWithStats struct {
	models.Tunnel
	TotalRx         int64  `json:"totalRx"`
	TotalTx         int64  `json:"totalTx"`
	EndpointName    string `json:"endpoint"`
	EndpointVersion string `json:"version,omitempty"`
	PortalHost      string `json:"portalHost"`
	VectorURL       string `json:"vectorUrl,omitempty"`
}

type TunnelListResult struct {
	Data       []TunnelWithStats `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

// PortalRequest is the complete editable Nowhere Portal configuration.
// Metadata belongs to OpenCtrl and is intentionally kept outside the URL.
type PortalRequest struct {
	Name        string             `json:"name" binding:"required"`
	EndpointID  int64              `json:"endpointId" binding:"required"`
	ListenHost  string             `json:"listenHost"`
	ListenPort  string             `json:"listenPort" binding:"required"`
	SharedKey   string             `json:"sharedKey" binding:"required"`
	Network     string             `json:"network"`
	TLSMode     TLSMode            `json:"tlsMode"`
	CertPath    string             `json:"certPath"`
	KeyPath     string             `json:"keyPath"`
	ALPN        string             `json:"alpn"`
	Rate        *int64             `json:"rate"`
	Etar        *int64             `json:"etar"`
	Dial        string             `json:"dial"`
	Socks       string             `json:"socks"`
	Next        string             `json:"next"`
	Up          string             `json:"up"`
	Down        string             `json:"down"`
	PoolSize    *int64             `json:"poolSize"`
	Sni         string             `json:"sni"`
	Pin         string             `json:"pin"`
	LogLevel    LogLevel           `json:"logLevel"`
	Restart     bool               `json:"restart"`
	Tags        *map[string]string `json:"tags"`
	Peer        *models.Peer       `json:"peer"`
	EnableStore bool               `json:"enableLogStore"`
}

type UpdatePortalRequest struct {
	PortalRequest
	ID int64 `json:"id"`
}

type TunnelActionRequest struct {
	InstanceID string `json:"instanceId" binding:"required"`
	Action     string `json:"action" binding:"required,oneof=start stop restart"`
}

type TunnelResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Error   string      `json:"error,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Tunnel  interface{} `json:"tunnel,omitempty"`
}

type TunnelSortItem struct {
	ID    int64 `json:"id" binding:"required"`
	Sorts int64 `json:"sorts"`
}

type UpdateTunnelsSortsRequest struct {
	Tunnels []TunnelSortItem `json:"tunnels" binding:"required,min=1"`
}
