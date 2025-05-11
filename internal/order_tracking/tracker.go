package ordertracking

import (
	"time"
	
	"github.com/go-co-op/gocron/v2"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/delivery"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

// OrderTracker là một struct để theo dõi trạng thái đơn hàng trên GHN.
type OrderTracker struct {
	store           db.Store
	taskDistributor worker.TaskDistributor
	ghnService      delivery.IDeliveryProvider
	scheduler       gocron.Scheduler
}

// NewOrderTracker tạo một tracker mới để theo dõi trạng thái đơn hàng trên GHN.
func NewOrderTracker(store db.Store, deliveryService delivery.IDeliveryProvider, taskDistributor worker.TaskDistributor) (*OrderTracker, error) {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	
	return &OrderTracker{
		store:           store,
		taskDistributor: taskDistributor,
		ghnService:      deliveryService,
		scheduler:       scheduler,
	}, nil
}

// Start bắt đầu chạy cronjob theo dõi trạng thái đơn hàng.
func (t *OrderTracker) Start() error {
	// Tạo job kiểm tra trạng thái đơn hàng (mỗi 10 giây)
	_, err := t.scheduler.NewJob(
		gocron.DurationJob(10*time.Second),
		gocron.NewTask(
			func() {
				t.checkOrderStatus()
			},
		),
	)
	
	if err != nil {
		return err
	}
	
	// Thêm job tự động hoàn tất đơn hàng sau 7 ngày
	// Chạy mỗi 1 giờ để không tạo quá nhiều tải
	_, err = t.scheduler.NewJob(
		gocron.DurationJob(1*time.Hour),
		gocron.NewTask(
			func() {
				log.Info().
					Str("job", "auto_complete_orders").
					Time("start_time", time.Now()).
					Msg("Starting auto-complete orders job")
				
				t.autoCompleteDeliveredOrders()
			},
		),
	)
	
	if err != nil {
		return err
	}
	
	// Bắt đầu scheduler
	t.scheduler.Start()
	return nil
}

// Stop dừng cronjob theo dõi trạng thái đơn hàng
func (t *OrderTracker) Stop() error {
	return t.scheduler.Shutdown()
}
