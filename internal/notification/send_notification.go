package notification

import (
	"context"
	"time"
	
	"github.com/rs/zerolog/log"
)

func (s *NotificationService) SendNotification(ctx context.Context, notification *Notification) error {
	// Create a new document in the Firestore collection
	_, _, err := s.client.Collection("notifications").Add(ctx, map[string]interface{}{
		"recipientID": notification.RecipientID,
		"title":       notification.Title,
		"message":     notification.Message,
		"type":        notification.Type,
		"referenceID": notification.ReferenceID,
		"isRead":      notification.IsRead,
		"createdAt":   time.Now(),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to send notification")
		return err
	}
	
	log.Info().Msg("notification sent successfully")
	return nil
}
