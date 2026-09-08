package models

import "time"

// Cursors belong to the endpoint, so deleting a Portal cannot erase its usage.
type EndpointTrafficCursor struct {
	ID         int64     `gorm:"primaryKey"`
	EndpointID int64     `gorm:"not null;uniqueIndex:idx_endpoint_traffic_cursor"`
	InstanceID string    `gorm:"not null;uniqueIndex:idx_endpoint_traffic_cursor"`
	TCPRx      int64     `gorm:"not null;default:0"`
	TCPTx      int64     `gorm:"not null;default:0"`
	UDPRx      int64     `gorm:"not null;default:0"`
	UDPTx      int64     `gorm:"not null;default:0"`
	ObservedAt time.Time `gorm:"not null"`
}
