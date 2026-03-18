package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
	params "github.com/vaibhaw/influenzer-backend/pkg/razorpay"
)

type PaymentService interface {
	CreateEscrow(ctx context.Context, proposalID string) (string, error)
	HandlePaymentSuccess(ctx context.Context, orderID, paymentID, signature string) error
	ReleaseFunds(ctx context.Context, proposalID string) error
}

type paymentService struct {
	proposalRepo domain.ProposalRepository
	campaignRepo domain.CampaignRepository
	userRepo     domain.AuthRepository
	rzp          params.Client
}

func NewPaymentService(pRepo domain.ProposalRepository, cRepo domain.CampaignRepository, userRepo domain.AuthRepository, rzp params.Client) PaymentService {
	return &paymentService{
		proposalRepo: pRepo,
		campaignRepo: cRepo,
		userRepo:     userRepo,
		rzp:          rzp,
	}
}

func (s *paymentService) CreateEscrow(ctx context.Context, proposalID string) (string, error) {
	proposal, err := s.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return "", err
	}

	if proposal.Status != domain.ProposalStatusApproved {
		return "", errors.New("proposal must be APPROVED to create escrow")
	}

	orderID, err := s.rzp.CreateOrder(proposal.BidAmount, "INR", proposal.ID.String(), nil)
	if err != nil {
		return "", err
	}

	proposal.RazorpayOrderID = orderID
	if err := s.proposalRepo.Update(ctx, proposal); err != nil {
		return "", err
	}

	return orderID, nil
}

func (s *paymentService) HandlePaymentSuccess(ctx context.Context, orderID, paymentID, signature string) error {
	if err := s.rzp.VerifyPaymentSignature(orderID, paymentID, signature); err != nil {
		return errors.New("invalid signature")
	}

	proposal, err := s.proposalRepo.GetByRazorpayOrderID(ctx, orderID)
	if err != nil {
		return err
	}

	if proposal.Status != domain.ProposalStatusApproved {
		if proposal.Status == domain.ProposalStatusFunded {
			return nil
		}
		return fmt.Errorf("proposal in invalid state state for funding: %s", proposal.Status)
	}

	proposal.RazorpayPaymentID = paymentID
	proposal.Status = domain.ProposalStatusFunded

	return s.proposalRepo.Update(ctx, proposal)
}

func (s *paymentService) ReleaseFunds(ctx context.Context, proposalID string) error {
	proposal, err := s.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return err
	}

	if proposal.Status != domain.ProposalStatusCompleted {
		return errors.New("proposal must be COMPLETED to release funds")
	}

	creatorProfile, err := s.userRepo.GetCreatorProfileByUserID(ctx, proposal.CreatorID.String())
	if err != nil {
		return fmt.Errorf("failed to fetch creator profile: %w", err)
	}

	if creatorProfile.RazorpayAccountID == "" {
		return errors.New("creator has no Razorpay account linked")
	}

	totalAmount := proposal.BidAmount
	creatorShare := totalAmount * 0.90

	_, err = s.rzp.TransferFunds(creatorProfile.RazorpayAccountID, creatorShare, "INR", nil)
	if err != nil {
		return err
	}

	proposal.Status = domain.ProposalStatusPaid
	return s.proposalRepo.Update(ctx, proposal)
}
