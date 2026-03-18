package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type SocialService interface {
	SubmitProof(ctx context.Context, proposalID string, instagramPostURL string) error
}

type socialService struct {
	proposalRepo domain.ProposalRepository
	campaignRepo domain.CampaignRepository
	authRepo     domain.AuthRepository
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

func (s *socialService) verifyInstagramPost(postURL, token, requiredHashtag string) error {
	if token == "" {
		// Can't verify without a token; allow submission
		return nil
	}

	shortcode := extractInstagramShortcode(postURL)
	if shortcode == "" {
		return errors.New("invalid Instagram post URL")
	}

	// Fetch recent media to find the post and check its caption
	apiURL := fmt.Sprintf("https://graph.instagram.com/me/media?fields=id,caption,permalink&access_token=%s", token)
	resp, err := http.Get(apiURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("failed to reach Instagram API: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID        string `json:"id"`
			Caption   string `json:"caption"`
			Permalink string `json:"permalink"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse Instagram response: %w", err)
	}
	if result.Error != nil {
		return fmt.Errorf("Instagram API error: %s", result.Error.Message)
	}

	for _, media := range result.Data {
		if strings.Contains(media.Permalink, shortcode) {
			if requiredHashtag != "" && !strings.Contains(media.Caption, requiredHashtag) {
				return fmt.Errorf("post caption does not contain required hashtag: %s", requiredHashtag)
			}
			return nil
		}
	}

	return errors.New("Instagram post not found in creator's recent media")
}

func extractInstagramShortcode(postURL string) string {
	// Handle https://www.instagram.com/p/{shortcode}/
	parts := strings.Split(strings.TrimRight(postURL, "/"), "/")
	for i, part := range parts {
		if part == "p" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
