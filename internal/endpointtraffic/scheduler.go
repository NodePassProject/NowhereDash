package endpointtraffic

import (
	"context"
	"sync"
	"time"

	log "NowhereDash/internal/log"
	"gorm.io/gorm"
)

// StartScheduler reconciles immediately after startup and once per minute.
func StartScheduler(database *gorm.DB) func() {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := SyncAll(database.WithContext(ctx), time.Now()); err != nil && ctx.Err() == nil {
				log.Errorf("Endpoint traffic renewal failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}
