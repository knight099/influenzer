package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	params "github.com/vaibhaw/influenzer-backend/pkg/razorpay"
	"gorm.io/gorm"
)

type PaymentService interface {
	CreateEscrow(ctx context.Context, proposalID string) (string, error)
	HandlePaymentSuccess(ctx context.Context, orderID, paymentID, signature string) error
	ReleaseFunds(ctx context.Context, proposalID string) error
	SaveBankAccount(ctx context.Context, userID string, req *domain.BankAccount) error
	GetBankAccount(ctx context.Context, userID string) (*domain.BankAccount, error)
}

type paymentService struct {
	proposalRepo  domain.ProposalRepository
	campaignRepo  domain.CampaignRepository
	userRepo      domain.AuthRepository
	rzp           params.Client
	db            *gorm.DB
	rzpAccountNum string // Razorpay X account number
}

func NewPaymentService(pRepo domain.ProposalRepository, cRepo domain.CampaignRepository, userRepo domain.AuthRepository, rzp params.Client, db *gorm.DB, rzpAccountNum string) PaymentService {
	return &paymentService{
		proposalRepo:  pRepo,
		campaignRepo:  cRepo,
		userRepo:      userRepo,
		rzp:           rzp,
		db:            db,
		rzpAccountNum: rzpAccountNum,
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

	// Get creator's bank account
	var bankAccount domain.BankAccount
	if err := s.db.Where("user_id = ?", proposal.CreatorID).First(&bankAccount).Error; err != nil {
		return errors.New("creator has not added bank account details")
	}

	// Create Razorpay contact if not already created
	if bankAccount.RazorpayContactID == "" {
		creatorProfile, err := s.userRepo.GetCreatorProfileByUserID(ctx, proposal.CreatorID.String())
		if err != nil {
			return fmt.Errorf("failed to fetch creator profile: %w", err)
		}

		contactID, err := s.rzp.CreateContact(
			bankAccount.AccountHolderName,
			creatorProfile.User.Email,
			creatorProfile.Phone,
			"employee",
		)
		if err != nil {
			return fmt.Errorf("failed to create razorpay contact: %w", err)
		}
		bankAccount.RazorpayContactID = contactID
		s.db.Save(&bankAccount)
	}

	// Create fund account if not already created
	if bankAccount.RazorpayFundAccID == "" {
		fundAccID, err := s.rzp.CreateFundAccount(
			bankAccount.RazorpayContactID,
			bankAccount.AccountHolderName,
			bankAccount.AccountNumber,
			bankAccount.IFSC,
		)
		if err != nil {
			return fmt.Errorf("failed to create razorpay fund account: %w", err)
		}
		bankAccount.RazorpayFundAccID = fundAccID
		s.db.Save(&bankAccount)
	}

	creatorShare := proposal.BidAmount * 0.90
	notes := map[string]interface{}{
		"proposal_id": proposalID,
		"creator_id":  proposal.CreatorID.String(),
	}

	_, err = s.rzp.CreatePayout(
		s.rzpAccountNum,
		bankAccount.RazorpayFundAccID,
		creatorShare,
		"INR",
		"payout",
		notes,
	)
	if err != nil {
		return fmt.Errorf("payout failed: %w", err)
	}

	proposal.Status = domain.ProposalStatusPaid
	return s.proposalRepo.Update(ctx, proposal)
}

func (s *paymentService) SaveBankAccount(ctx context.Context, userID string, req *domain.BankAccount) error {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return errors.New("invalid user id")
	}

	var existing domain.BankAccount
	result := s.db.Where("user_id = ?", uid).First(&existing)
	if result.Error != nil {
		// Create new
		req.UserID = uid
		req.RazorpayContactID = ""
		req.RazorpayFundAccID = ""
		return s.db.Create(req).Error
	}

	// Update existing — reset razorpay IDs if bank details changed
	if existing.AccountNumber != req.AccountNumber || existing.IFSC != req.IFSC {
		existing.RazorpayContactID = ""
		existing.RazorpayFundAccID = ""
	}
	existing.AccountHolderName = req.AccountHolderName
	existing.AccountNumber = req.AccountNumber
	existing.IFSC = req.IFSC
	existing.BankName = req.BankName
	return s.db.Save(&existing).Error
}

func (s *paymentService) GetBankAccount(ctx context.Context, userID string) (*domain.BankAccount, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.New("invalid user id")
	}

	var account domain.BankAccount
	if err := s.db.Where("user_id = ?", uid).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}
