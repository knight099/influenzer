package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type ProposalHandler struct {
	service      domain.ProposalService
	notifSvc     domain.NotificationService
	campaignRepo domain.CampaignRepository
}

func NewProposalHandler(
	r *gin.Engine,
	s domain.ProposalService,
	authMiddleware gin.HandlerFunc,
	notifSvc domain.NotificationService,
	campaignRepo domain.CampaignRepository,
) {
	handler := &ProposalHandler{
		service:      s,
		notifSvc:     notifSvc,
		campaignRepo: campaignRepo,
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

	c.JSON(http.StatusOK, proposals)
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
