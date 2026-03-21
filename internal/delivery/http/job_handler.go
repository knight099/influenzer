package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type JobHandler struct {
	db *gorm.DB // Direct DB for speed, ideally Service
}

func NewJobHandler(r *gin.Engine, db *gorm.DB, authMiddleware gin.HandlerFunc) {
	handler := &JobHandler{db: db}

	g := r.Group("/jobs") // also /upload
	g.Use(authMiddleware)
	{
		g.GET("/feed", handler.GetFeed)
		g.POST("/:id/apply", handler.Apply)
		g.GET("/my-applications", handler.MyApplications)
	}

	r.POST("/upload/presigned", authMiddleware, handler.GetPresignedURL)
}

// GetFeed godoc
// @Summary Get Job Feed
// @Description Retrieve a list of open campaigns (job feed)
// @Tags jobs
// @Produce json
// @Param niche query string false "Filter by niche"
// @Success 200 {array} map[string]interface{} "List of campaigns"
// @Security BearerAuth
// @Router /jobs/feed [get]
func (h *JobHandler) GetFeed(c *gin.Context) {
	niche := c.Query("niche")
	userIDVal, exists := c.Get("userID")

	var campaigns []domain.Campaign

	query := h.db.Where("status = ?", domain.CampaignStatusOpen)
	if niche != "" {
		query = query.Where("niche ILIKE ?", "%"+niche+"%")
	}

	query.Find(&campaigns)

	// Collect brand IDs to batch-fetch brand profiles
	brandIDSet := make(map[uuid.UUID]bool)
	for _, camp := range campaigns {
		brandIDSet[camp.BrandID] = true
	}
	brandIDs := make([]uuid.UUID, 0, len(brandIDSet))
	for id := range brandIDSet {
		brandIDs = append(brandIDs, id)
	}
	var brandProfiles []domain.BrandProfile
	h.db.Where("user_id IN ?", brandIDs).Find(&brandProfiles)
	brandMap := make(map[uuid.UUID]domain.BrandProfile, len(brandProfiles))
	for _, bp := range brandProfiles {
		brandMap[bp.UserID] = bp
	}

	// Get applied campaign IDs for this user
	appliedCampaigns := make(map[uuid.UUID]bool)
	if exists {
		var proposalCampaignIDs []uuid.UUID
		h.db.Model(&domain.Proposal{}).
			Where("creator_id = ?", userIDVal).
			Pluck("campaign_id", &proposalCampaignIDs)
		for _, id := range proposalCampaignIDs {
			appliedCampaigns[id] = true
		}
	}

	// Map to response
	var response []map[string]interface{}
	for _, camp := range campaigns {
		bp := brandMap[camp.BrandID]
		brandName := bp.CompanyName
		if brandName == "" {
			brandName = "Unknown Brand"
		}
		response = append(response, map[string]interface{}{
			"id":           camp.ID,
			"title":        camp.Title,
			"description":  camp.Description,
			"budget":       camp.Budget,
			"niche":        camp.Niche,
			"platform":     camp.Platform,
			"requirements": camp.Requirements,
			"brand_id":     camp.BrandID,
			"brand_name":   brandName,
			"brand_logo":   bp.LogoURL,
			"applied":      appliedCampaigns[camp.ID],
			"created_at":   camp.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, response)
}

type applyRequest struct {
	BidAmount   float64 `json:"bid_amount"`
	CoverLetter string  `json:"cover_letter"`
	VideoURL    string  `json:"video_url"`
}

// Apply godoc
// @Summary Apply to a Job
// @Description Submit a proposal to a campaign
// @Tags jobs
// @Accept json
// @Produce json
// @Param id path string true "Campaign ID"
// @Param request body applyRequest true "Application Details"
// @Success 200 {object} map[string]interface{} "proposal_id"
// @Failure 400 {object} map[string]interface{} "error"
// @Failure 500 {object} map[string]interface{} "error"
// @Security BearerAuth
// @Router /jobs/{id}/apply [post]
func (h *JobHandler) Apply(c *gin.Context) {
	campaignIDStr := c.Param("id")
	campaignID, _ := uuid.Parse(campaignIDStr)

	var req applyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// User ID
	userIDVal, _ := c.Get("userID")
	creatorID, _ := uuid.Parse(userIDVal.(string))

	proposal := domain.Proposal{
		CampaignID:    campaignID,
		CreatorID:     creatorID,
		BidAmount:     req.BidAmount,
		CoverNote:     req.CoverLetter,
		TrialVideoURL: req.VideoURL,
		Status:        domain.ProposalStatusApplied,
	}

	if err := h.db.Create(&proposal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"proposal_id": proposal.ID})
}

// MyApplications godoc
// @Summary Get My Applications
// @Description Retrieve a list of proposals submitted by the authenticated creator
// @Tags jobs
// @Produce json
// @Success 200 {array} domain.Proposal "List of proposals"
// @Security BearerAuth
// @Router /jobs/my-applications [get]
func (h *JobHandler) MyApplications(c *gin.Context) {
	userIDVal, _ := c.Get("userID")

	var proposals []domain.Proposal
	h.db.Preload("Campaign").Where("creator_id = ?", userIDVal).Find(&proposals)

	c.JSON(http.StatusOK, proposals)
}

// GetPresignedURL godoc
// @Summary Get Presigned URL
// @Description Get a mock presigned URL for file uploads
// @Tags upload
// @Produce json
// @Success 200 {object} map[string]interface{} "upload_url and public_url"
// @Security BearerAuth
// @Router /upload/presigned [post]
func (h *JobHandler) GetPresignedURL(c *gin.Context) {
	// Mock Presigned URL
	// Request: filename, file_type

	c.JSON(http.StatusOK, gin.H{
		"upload_url": "https://s3.aws.com/bucket/mock-upload-url",
		"public_url": "https://s3.aws.com/bucket/mock-file.mp4",
	})
}
