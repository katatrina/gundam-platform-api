package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
)

// PayloadSendNotification contain all data of the task that we want to store in Redis.
type PayloadSendNotification struct {
	RecipientID string
	Title       string
	Message     string
	Type        string
	ReferenceID string
}

func (distributor *RedisTaskDistributor) DistributeTaskSendNotification(
	ctx context.Context,
	payload *PayloadSendNotification,
	opts ...asynq.Option,
) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	
	task := asynq.NewTask(TaskSendNotification, jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	
	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).Str("queue", info.Queue).Int("max_retry", info.MaxRetry).Msg("task enqueued")
	
	return nil
}

func (processor *RedisTaskProcessor) ProcessTaskSendNotification(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadSendNotification
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}
	
	// Create a new document in the Firestore collection
	_, _, err := processor.firestoreClient.Collection("notifications").Add(ctx, map[string]interface{}{
		"recipientID": payload.RecipientID,
		"title":       payload.Title,
		"message":     payload.Message,
		"type":        payload.Type,
		"referenceID": payload.ReferenceID,
		"isRead":      false,
		"createdAt":   time.Now(),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to send notification")
		return err
	}
	
	log.Info().Str("type", task.Type()).Bytes("payload", task.Payload()).
		Str("referenceID", payload.ReferenceID).Msg("task processed")
	
	return nil
}
