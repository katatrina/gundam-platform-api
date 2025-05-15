package worker

import (
	"context"
	
	"github.com/hibiken/asynq"
)

type TaskInspector interface {
	DeleteTask(ctx context.Context, queue, taskID string) error
	GetTaskInfo(ctx context.Context, queue, taskID string) (*asynq.TaskInfo, error)
}

type RedisTaskInspector struct {
	inspector *asynq.Inspector
}

func NewTaskInspector(redisOpt asynq.RedisClientOpt) TaskInspector {
	return &RedisTaskInspector{
		inspector: asynq.NewInspector(redisOpt),
	}
}

func (i *RedisTaskInspector) DeleteTask(ctx context.Context, queue, taskID string) error {
	return i.inspector.DeleteTask(queue, taskID)
}

func (i *RedisTaskInspector) GetTaskInfo(ctx context.Context, queue, taskID string) (*asynq.TaskInfo, error) {
	return i.inspector.GetTaskInfo(queue, taskID)
}
