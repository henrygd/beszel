package notifications

import (
	"encoding/json"
	"os"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	webpush "github.com/SherClockHolmes/webpush-go"
)

// PushNotification represents a push notification message
type PushNotification struct {
	Title              string            `json:"title"`
	Body               string            `json:"body"`
	Icon               string            `json:"icon,omitempty"`
	Badge              string            `json:"badge,omitempty"`
	Image              string            `json:"image,omitempty"`
	Tag                string            `json:"tag,omitempty"`
	Data               map[string]string `json:"data,omitempty"`
	Actions            []Action          `json:"actions,omitempty"`
	RequireInteraction bool              `json:"requireInteraction,omitempty"`
}

// Action represents a notification action button
type Action struct {
	Action string `json:"action"`
	Title  string `json:"title"`
	Icon   string `json:"icon,omitempty"`
}

// PushSubscription represents a browser push subscription
type PushSubscription struct {
	ID       string    `json:"id" db:"id"`
	UserID   string    `json:"user" db:"user"`
	Endpoint string    `json:"endpoint" db:"endpoint"`
	P256dh   string    `json:"p256dh" db:"p256dh"`
	Auth     string    `json:"auth" db:"auth"`
	Created  time.Time `json:"created" db:"created"`
}

// PushService handles push notifications
type PushService struct {
	app       core.App
	vapidPriv string
	vapidPub  string
}

// NewPushService creates a new push notification service
func NewPushService(app core.App) *PushService {
	// Generate or load VAPID keys
	// In production, load from BESZEL_VAPID_PRIVATE_KEY env var
	privKey, pubKey := generateVAPIDKeys()

	return &PushService{
		app:       app,
		vapidPriv: privKey,
		vapidPub:  pubKey,
	}
}

// RegisterSubscription registers a push subscription for a user
func (s *PushService) RegisterSubscription(userID string, sub *webpush.Subscription) error {
	collection, err := s.app.FindCollectionByNameOrId("push_subscriptions")
	if err != nil {
		return err
	}

	// Check if subscription already exists
	existing, _ := s.app.FindFirstRecordByFilter("push_subscriptions",
		"user = {:user} && endpoint = {:endpoint}",
		map[string]interface{}{"user": userID, "endpoint": sub.Endpoint})

	if existing != nil {
		// Update existing
		existing.Set("p256dh", sub.Keys.P256dh)
		existing.Set("auth", sub.Keys.Auth)
		return s.app.Save(existing)
	}

	// Create new subscription
	record := core.NewRecord(collection)
	record.Set("user", userID)
	record.Set("endpoint", sub.Endpoint)
	record.Set("p256dh", sub.Keys.P256dh)
	record.Set("auth", sub.Keys.Auth)
	record.Set("created", time.Now())

	return s.app.Save(record)
}

// UnregisterSubscription removes a push subscription
func (s *PushService) UnregisterSubscription(userID string, endpoint string) error {
	record, err := s.app.FindFirstRecordByFilter("push_subscriptions",
		"user = {:user} && endpoint = {:endpoint}",
		map[string]interface{}{"user": userID, "endpoint": endpoint})
	if err != nil {
		return err
	}

	return s.app.Delete(record)
}

// SendNotification sends a push notification to a user
func (s *PushService) SendNotification(userID string, notification *PushNotification) error {
	// Get all subscriptions for user
	records, err := s.app.FindAllRecords("push_subscriptions",
		dbx.NewExp("user = {:user}", dbx.Params{"user": userID}),
	)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	for _, record := range records {
		sub := &webpush.Subscription{
			Endpoint: record.GetString("endpoint"),
			Keys: webpush.Keys{
				P256dh: record.GetString("p256dh"),
				Auth:   record.GetString("auth"),
			},
		}

		resp, err := webpush.SendNotification(payload, sub, &webpush.Options{
			Subscriber:      "beszel@localhost",
			VAPIDPublicKey:  s.vapidPub,
			VAPIDPrivateKey: s.vapidPriv,
			TTL:             30,
		})
		if err != nil {
			// Log error but continue trying other subscriptions
			continue
		}
		resp.Body.Close()
	}

	return nil
}

// BroadcastNotification sends notification to all users
func (s *PushService) BroadcastNotification(notification *PushNotification) error {
	records, err := s.app.FindAllRecords("push_subscriptions")
	if err != nil {
		return err
	}

	payload, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	for _, record := range records {
		sub := &webpush.Subscription{
			Endpoint: record.GetString("endpoint"),
			Keys: webpush.Keys{
				P256dh: record.GetString("p256dh"),
				Auth:   record.GetString("auth"),
			},
		}

		resp, err := webpush.SendNotification(payload, sub, &webpush.Options{
			Subscriber:      "beszel@localhost",
			VAPIDPublicKey:  s.vapidPub,
			VAPIDPrivateKey: s.vapidPriv,
			TTL:             30,
		})
		if err != nil {
			continue
		}
		resp.Body.Close()
	}

	return nil
}

// generateVAPIDKeys generates or loads VAPID keys for web push
func generateVAPIDKeys() (privateKey, publicKey string) {
	// Check for environment variable first
	if envKey := os.Getenv("BESZEL_VAPID_PRIVATE_KEY"); envKey != "" {
		// If private key provided, we need to derive public key
		// For now, return empty public key - will be handled by webpush lib
		return envKey, ""
	}

	// Generate new VAPID key pair
	privKey, pubKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		// Return empty keys if generation fails
		return "", ""
	}

	return privKey, pubKey
}

// GetVAPIDPublicKey returns the VAPID public key for client subscription
func (s *PushService) GetVAPIDPublicKey() string {
	return s.vapidPub
}
