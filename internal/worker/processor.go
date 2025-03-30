package worker

import (
	"context"
	
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/storage"
	"github.com/rs/zerolog/log"
)

/*
 This file contains code that will pick up the tasks from the Redis queue and process them.
*/

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

type TaskProcessor interface {
	Start() error
}

type RedisTaskProcessor struct {
	server    *asynq.Server
	dbStore   db.Store
	fileStore storage.FileStore
}

func NewRedisTaskProcessor(redisOpt asynq.RedisClientOpt, dbStore db.Store, fileStore storage.FileStore) TaskProcessor {
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
		server:  server,
		dbStore: dbStore,
	}
}

// Start registers the task handlers for the mux, attaches the mux to the asynq server, and starts the server.
func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()
	
	return processor.server.Start(mux)
}
