package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type CampaignHandler struct {
	service domain.CampaignService
}

func NewCampaignHandler(r *gin.Engine, s domain.CampaignService, authMiddleware gin.HandlerFunc) {
	handler := &CampaignHandler{service: s}

	g := r.Group("/campaigns")
	g.Use(authMiddleware) // Protect these routes
	{
		g.POST("", handler.Create)
		g.GET("", handler.List)
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

	// Response format per spec: { "id": "123", ... }
	c.JSON(http.StatusCreated, campaign)
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
