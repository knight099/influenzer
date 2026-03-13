package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type ProposalHandler struct {
	service      domain.ProposalService
	notifSvc     domain.NotificationService
	campaignRepo domain.CampaignRepository
	db           *gorm.DB
}

func NewProposalHandler(
	r *gin.Engine,
	s domain.ProposalService,
	authMiddleware gin.HandlerFunc,
	notifSvc domain.NotificationService,
	campaignRepo domain.CampaignRepository,
	db *gorm.DB,
) {
	handler := &ProposalHandler{
		service:      s,
		notifSvc:     notifSvc,
		campaignRepo: campaignRepo,
		db:           db,
	}

	g := r.Group("/proposals")
	g.Use(authMiddleware)
	{
		g.POST("", handler.Create)
		g.GET("/campaign/:campaignId", handler.GetByCampaignID)
		g.PATCH("/:id/status", handler.UpdateStatus)
	}
}

type createProposalRequest struct {
	CampaignID    string  `json:"campaign_id" binding:"required"`
	BidAmount     float64 `json:"bid_amount" binding:"required"`
	CoverNote     string  `json:"cover_note"`
	TrialVideoURL string  `json:"trial_video_url"`
}

func (h *ProposalHandler) Create(c *gin.Context) {
	var req createProposalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDVal, _ := c.Get("userID")
	creatorID, err := uuid.Parse(userIDVal.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid User ID"})
		return
	}

	campaignID, err := uuid.Parse(req.CampaignID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Campaign ID"})
		return
	}

	// Block duplicate proposal
	var existing domain.Proposal
	if h.db.Where("campaign_id = ? AND creator_id = ?", campaignID, creatorID).First(&existing).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "You have already submitted a proposal for this campaign"})
		return
	}

	proposal := &domain.Proposal{
		CampaignID:    campaignID,
		CreatorID:     creatorID,
		BidAmount:     req.BidAmount,
		CoverNote:     req.CoverNote,
		TrialVideoURL: req.TrialVideoURL,
		Status:        domain.ProposalStatusApplied,
	}

	if err := h.service.CreateProposal(c.Request.Context(), proposal); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify the brand that owns this campaign
	go func() {
		campaign, err := h.campaignRepo.GetByID(context.Background(), req.CampaignID)
		if err != nil {
			return
		}
		h.notifSvc.Notify(
			campaign.BrandID,
			domain.NotifNewProposal,
			"New Proposal Received",
			fmt.Sprintf("A creator submitted a proposal for \"%s\" with a bid of ₹%.0f", campaign.Title, req.BidAmount),
			proposal.ID.String(),
		)
	}()

	c.JSON(http.StatusCreated, proposal)
}

func (h *ProposalHandler) GetByCampaignID(c *gin.Context) {
	campaignID := c.Param("campaignId")

	proposals, err := h.service.GetProposalsByCampaignID(c.Request.Context(), campaignID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type enrichedProposal struct {
		domain.Proposal
		CreatorName      string  `json:"creator_name"`
		CreatorFollowers int64   `json:"creator_followers"`
		CreatorAvatar    string  `json:"creator_avatar"`
	}

	result := make([]enrichedProposal, 0, len(proposals))
	for _, p := range proposals {
		ep := enrichedProposal{Proposal: p}

		// Fetch creator user + profile
		var creator domain.User
		if h.db.Preload("CreatorProfile").First(&creator, "id = ?", p.CreatorID).Error == nil {
			ep.CreatorName = creator.Name
			ep.CreatorAvatar = creator.AvatarURL
			if creator.CreatorProfile != nil {
				stats := creator.CreatorProfile.CachedStats
				if ig, ok := stats["instagram"].(map[string]interface{}); ok {
					if f, ok := ig["followers_count"]; ok {
						switch v := f.(type) {
						case float64:
							ep.CreatorFollowers = int64(v)
						case int64:
							ep.CreatorFollowers = v
						}
					}
				}
				if ep.CreatorFollowers == 0 {
					if yt, ok := stats["youtube"].(map[string]interface{}); ok {
						if s, ok := yt["subscriber_count"]; ok {
							switch v := s.(type) {
							case float64:
								ep.CreatorFollowers = int64(v)
							case int64:
								ep.CreatorFollowers = v
							}
						}
					}
				}
			}
		}
		result = append(result, ep)
	}

	c.JSON(http.StatusOK, result)
}

type updateStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

func (h *ProposalHandler) UpdateStatus(c *gin.Context) {
	id := c.Param("id")
	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := domain.ProposalStatus(req.Status)

	// Fetch proposal before update to get creatorID
	proposal, err := h.service.GetProposalByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proposal not found"})
		return
	}

	if err := h.service.UpdateProposalStatus(c.Request.Context(), id, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify creator about status change
	go func() {
		switch status {
		case domain.ProposalStatusApproved:
			h.notifSvc.Notify(
				proposal.CreatorID,
				domain.NotifProposalAccepted,
				"Proposal Accepted! 🎉",
				fmt.Sprintf("Your proposal for \"%s\" has been accepted. Check your messages.", proposal.Campaign.Title),
				proposal.ID.String(),
			)
		case domain.ProposalStatus("REJECTED"):
			h.notifSvc.Notify(
				proposal.CreatorID,
				domain.NotifProposalRejected,
				"Proposal Update",
				fmt.Sprintf("Your proposal for \"%s\" was not selected this time.", proposal.Campaign.Title),
				proposal.ID.String(),
			)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}
