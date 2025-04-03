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
	server          *asynq.Server
	store           db.Store
	firestoreClient *firestore.Client
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, store db.Store, firebaseApp *firebase.App) *RedisTaskProcessor {
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
	}
}

// Start registers the task handlers for the mux, attaches the mux to the asynq server, and starts the server.
func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()
	
	mux.HandleFunc(TaskSendNotification, processor.ProcessTaskSendNotification)
	
	return processor.server.Start(mux)
}
