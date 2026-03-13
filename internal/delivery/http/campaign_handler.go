package http

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type CampaignHandler struct {
	service      domain.CampaignService
	notifService domain.NotificationService
	db           *gorm.DB
}

func NewCampaignHandler(r *gin.Engine, s domain.CampaignService, authMiddleware gin.HandlerFunc, notifService domain.NotificationService, db *gorm.DB) {
	handler := &CampaignHandler{service: s, notifService: notifService, db: db}

	g := r.Group("/campaigns")
	g.Use(authMiddleware) // Protect these routes
	{
		g.POST("", handler.Create)
		g.GET("", handler.List)
		g.GET("/my", handler.ListMy)
		g.POST("/:id/invite", handler.InviteCreator)
	}
}

type createCampaignRequest struct {
	Title       string  `json:"title" binding:"required"`
	Description string  `json:"description"`
	Budget      float64 `json:"budget" binding:"required"`
	Platform    string  `json:"platform"`
}

func (h *CampaignHandler) Create(c *gin.Context) {
	var req createCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userIDStr := userIDVal.(string)
	brandID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid User ID"})
		return
	}

	campaign := &domain.Campaign{
		BrandID:     brandID,
		Title:       req.Title,
		Description: req.Description,
		Budget:      req.Budget,
		Platform:    req.Platform,
		Status:      domain.CampaignStatusOpen,
	}

	if err := h.service.CreateCampaign(c.Request.Context(), campaign); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify all creators about the new campaign
	var brandProfile domain.BrandProfile
	brandName := "A brand"
	if err := h.db.Where("user_id = ?", brandID).First(&brandProfile).Error; err == nil && brandProfile.CompanyName != "" {
		brandName = brandProfile.CompanyName
	}
	h.notifService.NotifyAllCreators(
		domain.NotifCampaignCreated,
		"New Campaign Posted",
		fmt.Sprintf("%s posted a new campaign", brandName),
		campaign.ID.String(),
	)

	// Response format per spec: { "id": "123", ... }
	c.JSON(http.StatusCreated, campaign)
}

// ListMy returns campaigns owned by the authenticated brand.
func (h *CampaignHandler) ListMy(c *gin.Context) {
	brandID := mustUserID(c)

	var campaigns []domain.Campaign
	if err := h.db.Where("brand_id = ?", brandID).Order("created_at DESC").Find(&campaigns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if campaigns == nil {
		campaigns = []domain.Campaign{}
	}
	c.JSON(http.StatusOK, campaigns)
}

type inviteCreatorRequest struct {
	CreatorID string `json:"creator_id" binding:"required"`
}

// InviteCreator sends a campaign invitation notification to a creator.
func (h *CampaignHandler) InviteCreator(c *gin.Context) {
	brandID := mustUserID(c)

	campaignID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	var req inviteCreatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	creatorID, err := uuid.Parse(req.CreatorID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid creator_id"})
		return
	}

	// Validate the campaign belongs to this brand
	var campaign domain.Campaign
	if err := h.db.Where("id = ? AND brand_id = ?", campaignID, brandID).First(&campaign).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Campaign not found"})
		return
	}

	// Validate the creator exists
	var creator domain.User
	if err := h.db.Where("id = ? AND role = ?", creatorID, domain.RoleCreator).First(&creator).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator not found"})
		return
	}

	// Get brand name for notification body
	var brandProfile domain.BrandProfile
	brandName := "A brand"
	if err := h.db.Where("user_id = ?", brandID).First(&brandProfile).Error; err == nil && brandProfile.CompanyName != "" {
		brandName = brandProfile.CompanyName
	}

	h.notifService.Notify(
		creatorID,
		domain.NotifCampaignInvite,
		"Campaign Invitation",
		fmt.Sprintf("%s has invited you to join their campaign: %s", brandName, campaign.Title),
		campaign.ID.String(),
	)

	c.JSON(http.StatusOK, gin.H{"message": "Invitation sent"})
}

func (h *CampaignHandler) List(c *gin.Context) {
	minBudgetStr := c.Query("min_budget")
	var minBudget float64
	if minBudgetStr != "" {
		minBudget, _ = strconv.ParseFloat(minBudgetStr, 64)
	}

	campaigns, err := h.service.ListCampaigns(c.Request.Context(), minBudget, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, campaigns)
}
