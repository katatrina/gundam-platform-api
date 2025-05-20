package worker

import (
	"context"
	
	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

/*
 This file contains code that will pick up the tasks from the Redis queue and process them.
*/

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

type RedisTaskProcessor struct {
	server          *asynq.Server     // server will process tasks from the Redis queue.
	store           db.Store          // Tương tác với db
	firestoreClient *firestore.Client // Dùng để gửi thông báo đến cho người dùng thông qua Firestore
	distributor     TaskDistributor   // Dùng để phân phối task đến Redis queue
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, store db.Store, firebaseApp *firebase.App, distributor TaskDistributor) *RedisTaskProcessor {
	// Initialize Firestore client
	firestoreClient, err := firebaseApp.Firestore(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create firestore client 😣")
		return nil
	}
	
	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Queues: map[string]int{
				QueueCritical: 10,
				QueueDefault:  5,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Error().Err(err).Str("type", task.Type()).
					Bytes("payload", task.Payload()).Msg("process task failed")
			}),
			Logger: NewLogger(),
		},
	)
	
	return &RedisTaskProcessor{
		server:          server,
		store:           store,
		firestoreClient: firestoreClient,
		distributor:     distributor,
	}
}

// Start registers the task handlers for the mux, attaches the mux to the asynq server, and starts the server.
func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()
	
	mux.HandleFunc(TaskSendNotification, processor.ProcessTaskSendNotification)
	mux.HandleFunc(TaskStartAuction, processor.ProcessTaskStartAuction)
	mux.HandleFunc(TaskEndAuction, processor.ProcessTaskEndAuction)
	mux.HandleFunc(TaskCheckAuctionPayment, processor.ProcessTaskCheckAuctionPayment)
	mux.HandleFunc(TaskPaymentReminder, processor.ProcessTaskPaymentReminder)
	
	return processor.server.Start(mux)
}
