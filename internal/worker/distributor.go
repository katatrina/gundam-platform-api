package worker

import (
	"context"
	
	"github.com/hibiken/asynq"
)

const (
	TaskSendNotification    = "notification:send"
	TaskStartAuction        = "auction:start"
	TaskEndAuction          = "auction:end"
	TaskCheckAuctionPayment = "auction:check_payment"
	TaskPaymentReminder     = "auction:payment_reminder"
)

/*
This file will contain the codes to create tasks and distributes them to the Redis queue.
*/

type TaskDistributor interface {
	DistributeTaskSendNotification(ctx context.Context, payload *PayloadSendNotification, opts ...asynq.Option) error
	DistributeTaskStartAuction(ctx context.Context, payload *PayloadStartAuction, opts ...asynq.Option) error
	DistributeTaskEndAuction(ctx context.Context, payload *PayloadEndAuction, opts ...asynq.Option) error
	DistributeTaskCheckAuctionPayment(ctx context.Context, payload *PayloadCheckAuctionPayment, opts ...asynq.Option) error
	DistributeTaskPaymentReminder(ctx context.Context, payload *PayloadPaymentReminder, opts ...asynq.Option) error
}

type RedisTaskDistributor struct {
	client *asynq.Client // client sends tasks to redis queue.
}

func NewTaskDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	client := asynq.NewClient(redisOpt)
	
	return &RedisTaskDistributor{
		client: client,
	}
}
