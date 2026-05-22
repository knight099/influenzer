package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/service"
	"github.com/vaibhaw/influenzer-backend/pkg/utils"
)

type MatchingHandler struct {
	matchingService *service.MatchingService
}

func NewMatchingHandler(r *gin.Engine, matchingService *service.MatchingService, authMiddleware gin.HandlerFunc) {
	handler := &MatchingHandler{matchingService: matchingService}

	// Brand matching operations
	matchingGroup := r.Group("/api/match")
	matchingGroup.Use(authMiddleware)
	{
		matchingGroup.POST("/search", handler.SearchMatches)
		matchingGroup.GET("/campaign/:id", handler.MatchCampaign)
	}

	// Administration
	adminGroup := r.Group("/api/admin/embeddings")
	adminGroup.Use(authMiddleware)
	{
		adminGroup.POST("/refresh", handler.BulkRefreshEmbeddings)
	}
}

type SearchRequest struct {
	Query     string  `json:"query" binding:"required"`
	Platform  string  `json:"platform"`
	MinBudget float64 `json:"min_budget"`
	Limit     int     `json:"limit"`
}

// SearchMatches handles creator searching with natural language queries combined with filters
func (h *MatchingHandler) SearchMatches(c *gin.Context) {
	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	results, err := h.matchingService.FindMatchingCreators(req.Query, req.Platform, req.MinBudget, req.Limit)
	if err != nil {
		utils.Logger.Error("SearchMatches Error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search matches: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"matches":       results,
		"query_used":    req.Query,
		"total_matches": len(results),
	})
}

// MatchCampaign handles auto-matching creators to a campaign's profile
func (h *MatchingHandler) MatchCampaign(c *gin.Context) {
	campaignIDStr := c.Param("id")
	campaignID, err := uuid.Parse(campaignIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid campaign ID"})
		return
	}

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	results, err := h.matchingService.MatchCampaignToCreators(campaignID, limit)
	if err != nil {
		utils.Logger.Error("MatchCampaign Error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to match campaign: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"matches":       results,
		"total_matches": len(results),
	})
}

// BulkRefreshEmbeddings triggers regeneration of outstanding creator profile embeddings
func (h *MatchingHandler) BulkRefreshEmbeddings(c *gin.Context) {
	count, err := h.matchingService.BulkUpdateEmbeddings()
	if err != nil {
		utils.Logger.Error("BulkRefreshEmbeddings Error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to refresh embeddings: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"embeddings_built": count,
	})
}
