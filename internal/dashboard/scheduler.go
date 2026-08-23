package dashboard

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// TrafficScheduler 流量数据聚合调度器
type TrafficScheduler struct {
	db                 *gorm.DB
	trafficService     *TrafficService
	cleanupService     *CleanupService
	ownsCleanupService bool
	ctx                context.Context
	cancel             context.CancelFunc
}

// NewTrafficScheduler 创建流量调度器
func NewTrafficScheduler(db *gorm.DB, cleanupServices ...*CleanupService) *TrafficScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	cleanupService := (*CleanupService)(nil)
	if len(cleanupServices) > 0 {
		cleanupService = cleanupServices[0]
	}
	ownsCleanupService := cleanupService == nil
	if cleanupService == nil {
		cleanupService = NewCleanupService(db, DefaultCleanupConfig())
	}

	return &TrafficScheduler{
		db:                 db,
		trafficService:     NewTrafficService(db),
		cleanupService:     cleanupService,
		ownsCleanupService: ownsCleanupService,
		ctx:                ctx,
		cancel:             cancel,
	}
}

// Start 启动调度器
func (s *TrafficScheduler) Start() {
	log.Println("[流量调度器] 启动定时任务...")

	// 立即执行一次初始化，汇总最近24小时的数据
	go func() {
		// 启动后稍作延迟，减少与 SSE 初始写入的锁竞争
		time.Sleep(10 * time.Second)

		log.Println("[流量调度器] 开始初始化最近24小时流量汇总数据...")
		start := time.Now()

		// 对 SQLite locked 做有限重试
		var err error
		for attempt := 1; attempt <= 5; attempt++ {
			err = s.trafficService.InitializeRecentTrafficData()
			if err == nil {
				break
			}
			log.Printf("[流量调度器] 初始化24小时汇总数据失败(第%d次): %v", attempt, err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		if err != nil {
			log.Printf("[流量调度器] 初始化24小时汇总数据最终失败: %v", err)
		} else {
			duration := time.Since(start)
			log.Printf("[流量调度器] 初始化24小时汇总数据完成，耗时: %v", duration)
		}

		// 然后执行上一小时的常规聚合（如果有遗漏）
		log.Println("[流量调度器] 执行启动时常规数据聚合...")
		for attempt := 1; attempt <= 5; attempt++ {
			err = s.trafficService.AggregateTrafficData()
			if err == nil {
				break
			}
			log.Printf("[流量调度器] 启动时常规数据聚合失败(第%d次): %v", attempt, err)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		if err != nil {
			log.Printf("[流量调度器] 启动时常规数据聚合最终失败: %v", err)
		} else {
			log.Println("[流量调度器] 启动时常规数据聚合完成")
		}
	}()

	// 启动定时任务
	go s.runAligned()

	// 启动数据清理任务（每天凌晨3:15执行）
	go s.runCleanupTask()

	log.Println("[流量调度器] 定时任务已启动")
}

// Stop 停止调度器
func (s *TrafficScheduler) Stop() {
	log.Println("[流量调度器] 停止定时任务...")

	s.cancel()
	if s.ownsCleanupService {
		s.cleanupService.Close()
	}
	log.Println("[流量调度器] 定时任务已停止")
}

// runAligned 在每个整点执行上一完整小时的聚合。
// 整点后两分钟再校准一次，纳入可能延迟落库的最后一分钟数据。
func (s *TrafficScheduler) runAligned() {
	now := time.Now()
	nextRun := now.Truncate(time.Hour).Add(1 * time.Hour)
	timer := time.NewTimer(time.Until(nextRun))
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			s.executeAggregation()
			s.scheduleReconciliation()

			nextRun = nextRun.Add(1 * time.Hour)
			timer.Reset(time.Until(nextRun))
		}
	}
}

func (s *TrafficScheduler) scheduleReconciliation() {
	go func() {
		timer := time.NewTimer(2 * time.Minute)
		defer timer.Stop()

		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			log.Println("[流量调度器] 开始执行整点流量校准...")
			s.executeAggregation()
		}
	}()
}

// executeAggregation 执行数据聚合
func (s *TrafficScheduler) executeAggregation() {
	start := time.Now()
	log.Println("[流量调度器] 开始执行小时流量数据聚合...")

	err := s.trafficService.AggregateTrafficData()
	if err != nil {
		log.Printf("[流量调度器] 数据聚合失败: %v", err)
		return
	}

	duration := time.Since(start)
	log.Printf("[流量调度器] 数据聚合完成，耗时: %v", duration)
}

// runCleanupTask 运行数据清理任务
func (s *TrafficScheduler) runCleanupTask() {
	timer := time.NewTimer(time.Until(nextCleanupRun(time.Now())))
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			s.executeCleanup()
			timer.Reset(time.Until(nextCleanupRun(time.Now())))
		}
	}
}

func nextCleanupRun(now time.Time) time.Time {
	nextRun := time.Date(now.Year(), now.Month(), now.Day(), 3, 15, 0, 0, now.Location())
	if !nextRun.After(now) {
		nextRun = nextRun.AddDate(0, 0, 1)
	}
	return nextRun
}

// executeCleanup 执行数据清理
func (s *TrafficScheduler) executeCleanup() {
	if !s.cleanupService.ConfigSnapshot().AutoCleanupEnabled {
		log.Println("[流量调度器] 自动数据清理已禁用，跳过本次任务")
		return
	}

	start := time.Now()
	log.Println("[流量调度器] 开始执行数据清理任务...")

	results, err := s.cleanupService.ExecuteFullCleanupContext(s.ctx)

	// 统计清理结果
	totalDeleted := int64(0)
	for _, result := range results {
		totalDeleted += result.DeletedCount
		if result.Error != nil {
			log.Printf("[流量调度器] %s 清理出现错误: %v", result.TableName, result.Error)
		}
	}
	if err != nil {
		log.Printf("[流量调度器] 数据清理未完全成功: %v", err)
	}

	duration := time.Since(start)
	log.Printf("[流量调度器] 数据清理完成，总共删除 %d 条记录，耗时: %v", totalDeleted, duration)
}

// GetTrafficService 获取流量服务实例
func (s *TrafficScheduler) GetTrafficService() *TrafficService {
	return s.trafficService
}
