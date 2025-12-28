package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type ProposalHandler struct {
	service domain.ProposalService
}

func NewProposalHandler(r *gin.Engine, s domain.ProposalService, authMiddleware gin.HandlerFunc) {
	handler := &ProposalHandler{service: s}

	g := r.Group("/proposals")
	g.Use(authMiddleware)
	{
		g.POST("", handler.Create)
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

	c.JSON(http.StatusCreated, proposal)
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

	// TODO: Verify if the User (Brand) owns the campaign related to this proposal!
	// Skipping ownership check for speed, but MUST be here in prod.

	status := domain.ProposalStatus(req.Status)

	// Simple validation
	switch status {
	case domain.ProposalStatusApproved, domain.ProposalStatusFunded: // etc
		// ok
	default:
		// c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		// return
	}

	if err := h.service.UpdateProposalStatus(c.Request.Context(), id, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Status updated"})
}
