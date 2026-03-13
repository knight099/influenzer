package repository

import (
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type notificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) domain.NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) Create(n *domain.Notification) error {
	return r.db.Create(n).Error
}

func (r *notificationRepository) ListByUserID(userID uuid.UUID, limit int) ([]domain.Notification, error) {
	var notifications []domain.Notification
	err := r.db.
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&notifications).Error
	return notifications, err
}

func (r *notificationRepository) MarkRead(id uuid.UUID, userID uuid.UUID) error {
	return r.db.Model(&domain.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true).Error
}

func (r *notificationRepository) MarkAllRead(userID uuid.UUID) error {
	return r.db.Model(&domain.Notification{}).
		Where("user_id = ? AND is_read = false", userID).
		Update("is_read", true).Error
}

func (r *notificationRepository) UnreadCount(userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.Model(&domain.Notification{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&count).Error
	return count, err
}

func (r *notificationRepository) GetDeviceTokens(userID uuid.UUID) ([]string, error) {
	var tokens []domain.DeviceToken
	err := r.db.Where("user_id = ?", userID).Find(&tokens).Error
	if err != nil {
		return nil, err
	}
	result := make([]string, len(tokens))
	for i, t := range tokens {
		result[i] = t.Token
	}
	return result, nil
}

func (r *notificationRepository) SaveDeviceToken(token *domain.DeviceToken) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "token"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_id", "platform", "updated_at"}),
	}).Create(token).Error
}

func (r *notificationRepository) DeleteDeviceToken(userID uuid.UUID, token string) error {
	return r.db.Where("user_id = ? AND token = ?", userID, token).
		Delete(&domain.DeviceToken{}).Error
}

func (r *notificationRepository) GetAllCreatorIDs() ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.Model(&domain.User{}).
		Where("role = ?", domain.RoleCreator).
		Pluck("id", &ids).Error
	return ids, err
}
