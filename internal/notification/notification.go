package notification

import (
	"context"
	
	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/rs/zerolog/log"
)

type NotificationService struct {
	client *firestore.Client
}

func NewNotificationService(ctx context.Context, firebaseApp *firebase.App) (*NotificationService, error) {
	// Initialize Firestore client
	firestoreClient, err := firebaseApp.Firestore(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create firestore client 😣")
		return nil, err
	}
	defer firestoreClient.Close()
	
	return &NotificationService{
		client: firestoreClient,
	}, nil
}
