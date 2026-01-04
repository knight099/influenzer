package service

import (
	"context"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type proposalService struct {
	repo domain.ProposalRepository
}

func NewProposalService(repo domain.ProposalRepository) domain.ProposalService {
	return &proposalService{repo: repo}
}

func (s *proposalService) CreateProposal(ctx context.Context, proposal *domain.Proposal) error {
	// Logic to check if user already applied?
	return s.repo.Create(ctx, proposal)
}

func (s *proposalService) UpdateProposalStatus(ctx context.Context, id string, status domain.ProposalStatus) error {
	proposal, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	proposal.Status = status
	return s.repo.Update(ctx, proposal)
}

func (s *proposalService) GetProposalByID(ctx context.Context, id string) (*domain.Proposal, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *proposalService) GetProposalsByCampaignID(ctx context.Context, campaignID string) ([]domain.Proposal, error) {
	return s.repo.ListByCampaignID(ctx, campaignID)
}
