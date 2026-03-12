package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type notificationService struct {
	repo       domain.NotificationRepository
	fcmSender  *fcmSender
}

func NewNotificationService(repo domain.NotificationRepository) domain.NotificationService {
	sender := newFCMSender(os.Getenv("FCM_SERVER_KEY"), os.Getenv("FCM_PROJECT_ID"))
	return &notificationService{repo: repo, fcmSender: sender}
}

// Notify creates a DB notification and fires a push — fire-and-forget.
func (s *notificationService) Notify(userID uuid.UUID, nType domain.NotificationType, title, body, resourceID string) {
	n := &domain.Notification{
		UserID:     userID,
		Type:       nType,
		Title:      title,
		Body:       body,
		ResourceID: resourceID,
		CreatedAt:  time.Now(),
	}
	if err := s.repo.Create(n); err != nil {
		log.Printf("[notify] db error: %v", err)
	}

	// Send FCM push (best-effort)
	go func() {
		tokens, err := s.repo.GetDeviceTokens(userID)
		if err != nil || len(tokens) == 0 {
			return
		}
		s.fcmSender.Send(tokens, title, body, map[string]string{
			"type":        string(nType),
			"resource_id": resourceID,
		})
	}()
}

func (s *notificationService) List(userID uuid.UUID) ([]domain.Notification, error) {
	return s.repo.ListByUserID(userID, 50)
}

func (s *notificationService) MarkRead(id string, userID uuid.UUID) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid notification id")
	}
	return s.repo.MarkRead(parsed, userID)
}

func (s *notificationService) MarkAllRead(userID uuid.UUID) error {
	return s.repo.MarkAllRead(userID)
}

func (s *notificationService) UnreadCount(userID uuid.UUID) (int64, error) {
	return s.repo.UnreadCount(userID)
}

func (s *notificationService) RegisterDevice(userID uuid.UUID, token, platform string) error {
	dt := &domain.DeviceToken{
		UserID:   userID,
		Token:    token,
		Platform: platform,
	}
	return s.repo.SaveDeviceToken(dt)
}

func (s *notificationService) UnregisterDevice(userID uuid.UUID, token string) error {
	return s.repo.DeleteDeviceToken(userID, token)
}

// ── FCM sender (HTTP v1 API) ──────────────────────────────────────────────────

type fcmSender struct {
	serverKey string // legacy FCM server key  (FCM_SERVER_KEY env var)
	projectID string // Firebase project ID    (FCM_PROJECT_ID env var)
	client    *http.Client
}

func newFCMSender(serverKey, projectID string) *fcmSender {
	return &fcmSender{
		serverKey: serverKey,
		projectID: projectID,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Send uses FCM legacy HTTP API (simple, no Firebase Admin SDK required).
// Set FCM_SERVER_KEY env var from Firebase console → Project Settings → Cloud Messaging.
func (f *fcmSender) Send(tokens []string, title, body string, data map[string]string) {
	if f.serverKey == "" {
		return // FCM not configured, skip silently
	}

	payload := map[string]interface{}{
		"registration_ids": tokens,
		"notification": map[string]string{
			"title": title,
			"body":  body,
			"sound": "default",
		},
		"data":     data,
		"priority": "high",
	}

	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"https://fcm.googleapis.com/fcm/send",
		bytes.NewBuffer(b),
	)
	if err != nil {
		log.Printf("[fcm] build request error: %v", err)
		return
	}
	req.Header.Set("Authorization", "key="+f.serverKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		log.Printf("[fcm] send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[fcm] unexpected status: %d", resp.StatusCode)
	}
}
