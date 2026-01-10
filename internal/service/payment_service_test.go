package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"github.com/vaibhaw/influenzer-backend/internal/service"
)

// Mocks
type MockProposalRepo struct {
	mock.Mock
}

func (m *MockProposalRepo) Create(ctx context.Context, p *domain.Proposal) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}
func (m *MockProposalRepo) GetByID(ctx context.Context, id string) (*domain.Proposal, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Proposal), args.Error(1)
}
func (m *MockProposalRepo) GetByRazorpayOrderID(ctx context.Context, orderID string) (*domain.Proposal, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Proposal), args.Error(1)
}
func (m *MockProposalRepo) ListByCampaignID(ctx context.Context, cID string) ([]domain.Proposal, error) {
	args := m.Called(ctx, cID)
	return args.Get(0).([]domain.Proposal), args.Error(1)
}
func (m *MockProposalRepo) Update(ctx context.Context, p *domain.Proposal) error {
	args := m.Called(ctx, p)
	return args.Error(0)
}

type MockClient struct {
	mock.Mock
}

func (m *MockClient) CreateOrder(amt float64, cur, rec string, notes map[string]interface{}) (string, error) {
	args := m.Called(amt, cur, rec, notes)
	return args.String(0), args.Error(1)
}
func (m *MockClient) VerifyPaymentSignature(oid, pid, sig string) error {
	args := m.Called(oid, pid, sig)
	return args.Error(0)
}
func (m *MockClient) TransferFunds(acc string, amt float64, cur string, notes map[string]interface{}) (string, error) {
	args := m.Called(acc, amt, cur, notes)
	return args.String(0), args.Error(1)
}
func (m *MockClient) CreateSubscription(planID string, totalCount int, customerNotify int, notes map[string]interface{}) (string, string, error) {
	args := m.Called(planID, totalCount, customerNotify, notes)
	return args.String(0), args.String(1), args.Error(2)
}

func TestCreateEscrow(t *testing.T) {
	repo := new(MockProposalRepo)
	rzp := new(MockClient)
	svc := service.NewPaymentService(repo, nil, rzp) // CampaignRepo not needed for CreateEscrow

	proposalID := uuid.New()
	proposal := &domain.Proposal{
		ID:        proposalID,
		BidAmount: 1000,
		Status:    domain.ProposalStatusApproved,
	}

	repo.On("GetByID", mock.Anything, proposalID.String()).Return(proposal, nil)
	rzp.On("CreateOrder", 1000.0, "INR", proposalID.String(), mock.Anything).Return("order_123", nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(p *domain.Proposal) bool {
		return p.RazorpayOrderID == "order_123"
	})).Return(nil)

	orderID, err := svc.CreateEscrow(context.Background(), proposalID.String())

	assert.NoError(t, err)
	assert.Equal(t, "order_123", orderID)
	repo.AssertExpectations(t)
	rzp.AssertExpectations(t)
}

func TestCreateEscrow_InvalidStatus(t *testing.T) {
	repo := new(MockProposalRepo)
	svc := service.NewPaymentService(repo, nil, nil)

	proposalID := uuid.New()
	proposal := &domain.Proposal{
		ID:     proposalID,
		Status: domain.ProposalStatusApplied, // Not Approved
	}

	repo.On("GetByID", mock.Anything, proposalID.String()).Return(proposal, nil)

	_, err := svc.CreateEscrow(context.Background(), proposalID.String())

	assert.Error(t, err)
	assert.Equal(t, "proposal must be APPROVED to create escrow", err.Error())
}

func TestReleaseFunds(t *testing.T) {
	repo := new(MockProposalRepo)
	rzp := new(MockClient)
	svc := service.NewPaymentService(repo, nil, rzp)

	proposalID := uuid.New()
	proposal := &domain.Proposal{
		ID:        proposalID,
		BidAmount: 1000,
		Status:    domain.ProposalStatusCompleted,
	}

	repo.On("GetByID", mock.Anything, proposalID.String()).Return(proposal, nil)
	// 900 to creator
	rzp.On("TransferFunds", "acc_test_123", 900.0, "INR", mock.Anything).Return("transfer_123", nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(p *domain.Proposal) bool {
		return p.Status == domain.ProposalStatusPaid
	})).Return(nil)

	err := svc.ReleaseFunds(context.Background(), proposalID.String())

	assert.NoError(t, err)
	repo.AssertExpectations(t)
}
