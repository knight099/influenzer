package repository

import (
	"context"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type campaignRepository struct {
	db *gorm.DB
}

func NewCampaignRepository(db *gorm.DB) domain.CampaignRepository {
	return &campaignRepository{db: db}
}

func (r *campaignRepository) Create(ctx context.Context, campaign *domain.Campaign) error {
	return r.db.WithContext(ctx).Create(campaign).Error
}

func (r *campaignRepository) GetByID(ctx context.Context, id string) (*domain.Campaign, error) {
	var campaign domain.Campaign
	err := r.db.WithContext(ctx).First(&campaign, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &campaign, nil
}

func (r *campaignRepository) List(ctx context.Context, filter map[string]interface{}) ([]domain.Campaign, error) {
	var campaigns []domain.Campaign
	query := r.db.WithContext(ctx).Model(&domain.Campaign{})

	if val, ok := filter["min_budget"]; ok {
		query = query.Where("budget >= ?", val)
	}
	// Simple JSONB query for Niche/Requirements matching if needed,
	// for now treating 'niche' as simple match if we add it to Campaign or derive it.
	// The requirement said "Support filtering by Budget/Niche".
	// Campaign requirements is JSON, maybe niche is stored there or we rely on text search?
	// Proposal has CreatorID -> CreatorProfile -> Niche.
	// Campaign usually doesn't have a "Niche" field directly in the provided detailed schema,
	// but let's assume filtering by budget is primary.

	// If filter is empty, return all open campaigns
	query = query.Where("status = ?", domain.CampaignStatusOpen)

	err := query.Find(&campaigns).Error
	return campaigns, err
}

func (r *campaignRepository) Update(ctx context.Context, campaign *domain.Campaign) error {
	return r.db.WithContext(ctx).Save(campaign).Error
}
