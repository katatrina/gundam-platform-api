package worker

import (
	"github.com/hibiken/asynq"
)

type TaskInspector interface {
	DeleteTask(queue, taskID string) error
	GetTaskInfo(queue, taskID string) (*asynq.TaskInfo, error)
}

type RedisTaskInspector struct {
	inspector *asynq.Inspector
}

func NewTaskInspector(redisOpt asynq.RedisClientOpt) TaskInspector {
	return &RedisTaskInspector{
		inspector: asynq.NewInspector(redisOpt),
	}
}

func (i *RedisTaskInspector) DeleteTask(queue, taskID string) error {
	return i.inspector.DeleteTask(queue, taskID)
}

func (i *RedisTaskInspector) GetTaskInfo(queue, taskID string) (*asynq.TaskInfo, error) {
	return i.inspector.GetTaskInfo(queue, taskID)
}
