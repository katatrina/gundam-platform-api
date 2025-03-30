package notification

import (
	"time"
)

type Notification struct {
	RecipientID string
	Title       string
	Message     string
	Type        string
	ReferenceID string
	IsRead      bool
	CreatedAt   time.Time
}
