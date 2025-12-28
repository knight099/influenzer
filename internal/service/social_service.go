package service

import (
	"context"
	"errors"
	"strings"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type SocialService interface {
	SubmitProof(ctx context.Context, proposalID string, instagramPostURL string) error
}

type socialService struct {
	proposalRepo domain.ProposalRepository
	campaignRepo domain.CampaignRepository
	// authRepo needed to get user tokens if they are not in Context?
	// actually tokens are on User struct. But Proposal has CreatorID.
	// We need to fetch Creator to get the token.
	authRepo domain.AuthRepository
}

func NewSocialService(pRepo domain.ProposalRepository, cRepo domain.CampaignRepository, aRepo domain.AuthRepository) SocialService {
	return &socialService{
		proposalRepo: pRepo,
		campaignRepo: cRepo,
		authRepo:     aRepo,
	}
}

func (s *socialService) SubmitProof(ctx context.Context, proposalID string, instagramPostURL string) error {
	proposal, err := s.proposalRepo.GetByID(ctx, proposalID)
	if err != nil {
		return err
	}

	if proposal.Status != domain.ProposalStatusFunded {
		return errors.New("proposal must be FUNDED before submitting proof")
	}

	// 1. Fetch Campaign to know requirements (e.g. Hashtag)
	campaign, err := s.campaignRepo.GetByID(ctx, proposal.CampaignID.String())
	if err != nil {
		return err
	}

	// Assume requirement is just "hashtags" key in JSON
	requiredHashtag := ""
	if tags, ok := campaign.Requirements["hashtags"].(string); ok {
		requiredHashtag = tags
	}

	// 2. Fetch Creator to get Instagram Token
	// real implementation would use decrypted token
	creatorUser, err := s.authRepo.GetBaseUserByID(ctx, proposal.CreatorID.String())
	if err != nil {
		return err
	}

	// 3. Verify via Graph API
	// Extract Post ID from URL (naive implementation)
	// https://www.instagram.com/p/Cush.../
	// Mock verification
	if err := s.verifyInstagramPost(instagramPostURL, creatorUser.InstagramToken, requiredHashtag); err != nil {
		return err
	}

	proposal.ProofURL = instagramPostURL
	proposal.Status = domain.ProposalStatusSubmitted
	return s.proposalRepo.Update(ctx, proposal)
}

func (s *socialService) verifyInstagramPost(url, token, hashtag string) error {
	// Mock Logic
	// In real world: Call Graph API /media endpoint
	if token == "" {
		// return errors.New("creator has not linked instagram")
		// For testing allow empty token
	}

	// If hashtag matches "fail", simulate failure
	if strings.Contains(url, "fail") {
		return errors.New("verification failed: caption does not contain required tags")
	}

	return nil
}
