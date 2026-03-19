package notifications

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const TypeNotificationDeliver = "notification_deliver"
const QueueNotifications = "notifications"

// NotificationDeliverPayload is the asynq task payload for outbound delivery.
type NotificationDeliverPayload struct {
	Host             string                 `json:"host,omitempty"`
	DestinationID    string                 `json:"destination_id"`
	RouteID          string                 `json:"route_id"`
	ChannelType      string                 `json:"channel_type"`
	EventType        string                 `json:"event_type"`
	Severity         string                 `json:"severity"`
	Title            string                 `json:"title"`
	Message          string                 `json:"message"`
	ReferenceType    string                 `json:"reference_type"`
	ReferenceID      string                 `json:"reference_id"`
	EventFingerprint string                 `json:"event_fingerprint"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// NewNotificationDeliverTask builds the asynq task.
func NewNotificationDeliverTask(p NotificationDeliverPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeNotificationDeliver, b), nil
}
