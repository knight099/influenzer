package repository

import (
	"context"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type proposalRepository struct {
	db *gorm.DB
}

func NewProposalRepository(db *gorm.DB) domain.ProposalRepository {
	return &proposalRepository{db: db}
}

func (r *proposalRepository) Create(ctx context.Context, proposal *domain.Proposal) error {
	return r.db.WithContext(ctx).Create(proposal).Error
}

func (r *proposalRepository) GetByID(ctx context.Context, id string) (*domain.Proposal, error) {
	var proposal domain.Proposal
	err := r.db.WithContext(ctx).First(&proposal, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &proposal, nil
}

func (r *proposalRepository) GetByRazorpayOrderID(ctx context.Context, orderID string) (*domain.Proposal, error) {
	var proposal domain.Proposal
	err := r.db.WithContext(ctx).First(&proposal, "razorpay_order_id = ?", orderID).Error
	if err != nil {
		return nil, err
	}
	return &proposal, nil
}

func (r *proposalRepository) ListByCampaignID(ctx context.Context, campaignID string) ([]domain.Proposal, error) {
	var proposals []domain.Proposal
	err := r.db.WithContext(ctx).Where("campaign_id = ?", campaignID).Find(&proposals).Error
	return proposals, err
}

func (r *proposalRepository) Update(ctx context.Context, proposal *domain.Proposal) error {
	return r.db.WithContext(ctx).Save(proposal).Error
}
