package service

import (
	"context"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type campaignService struct {
	repo domain.CampaignRepository
}

func NewCampaignService(repo domain.CampaignRepository) domain.CampaignService {
	return &campaignService{repo: repo}
}

func (s *campaignService) CreateCampaign(ctx context.Context, campaign *domain.Campaign) error {
	// Add validation logic here
	if campaign.Budget <= 0 {
		// return error
	}
	return s.repo.Create(ctx, campaign)
}

func (s *campaignService) ListCampaigns(ctx context.Context, minBudget float64, niche string) ([]domain.Campaign, error) {
	filter := make(map[string]interface{})
	if minBudget > 0 {
		filter["min_budget"] = minBudget
	}
	// Niche filter logic if implemented
	return s.repo.List(ctx, filter)
}
