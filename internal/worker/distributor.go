package worker

import (
	"github.com/hibiken/asynq"
)

/*
This file will contain the codes to create tasks and distributes them to the Redis queue.
*/

type RedisTaskDistributor struct {
	client *asynq.Client // client sends tasks to redis queue.
}

func NewRedisTaskDistributor(redisOpt asynq.RedisClientOpt) *RedisTaskDistributor {
	client := asynq.NewClient(redisOpt)
	
	return &RedisTaskDistributor{
		client: client,
	}
}
