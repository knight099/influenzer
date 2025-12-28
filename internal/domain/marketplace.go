package domain

import (
	"context"
)

type CampaignRepository interface {
	Create(ctx context.Context, campaign *Campaign) error
	GetByID(ctx context.Context, id string) (*Campaign, error)
	List(ctx context.Context, filter map[string]interface{}) ([]Campaign, error)
	Update(ctx context.Context, campaign *Campaign) error
}

type ProposalRepository interface {
	Create(ctx context.Context, proposal *Proposal) error
	GetByID(ctx context.Context, id string) (*Proposal, error)
	GetByRazorpayOrderID(ctx context.Context, orderID string) (*Proposal, error)
	ListByCampaignID(ctx context.Context, campaignID string) ([]Proposal, error)
	Update(ctx context.Context, proposal *Proposal) error
}

type CampaignService interface {
	CreateCampaign(ctx context.Context, campaign *Campaign) error
	ListCampaigns(ctx context.Context, minBudget float64, niche string) ([]Campaign, error)
}

type ProposalService interface {
	CreateProposal(ctx context.Context, proposal *Proposal) error
	UpdateProposalStatus(ctx context.Context, id string, status ProposalStatus) error
	GetProposalByID(ctx context.Context, id string) (*Proposal, error)
}
