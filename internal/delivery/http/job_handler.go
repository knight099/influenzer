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

func (h *JobHandler) GetFeed(c *gin.Context) {
	niche := c.Query("niche")
	var campaigns []domain.Campaign

	query := h.db.Where("status = ?", domain.CampaignStatusOpen)
	// Filter by niche if we add Niche to Campaign or derived
	// For now just return all
	if niche != "" {
		// Mock filter
	}

	query.Find(&campaigns)

	// Map to simplified response
	var response []map[string]interface{}
	for _, camp := range campaigns {
		response = append(response, map[string]interface{}{
			"id":       camp.ID,
			"title":    camp.Title,
			"budget":   camp.Budget,
			"platform": camp.Platform,
		})
	}

	c.JSON(http.StatusOK, response)
}

type applyRequest struct {
	BidAmount   float64 `json:"bid_amount"`
	CoverLetter string  `json:"cover_letter"`
	VideoURL    string  `json:"video_url"`
}

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

func (h *JobHandler) MyApplications(c *gin.Context) {
	userIDVal, _ := c.Get("userID")

	var proposals []domain.Proposal
	h.db.Where("creator_id = ?", userIDVal).Find(&proposals)

	c.JSON(http.StatusOK, proposals)
}

func (h *JobHandler) GetPresignedURL(c *gin.Context) {
	// Mock Presigned URL
	// Request: filename, file_type

	c.JSON(http.StatusOK, gin.H{
		"upload_url": "https://s3.aws.com/bucket/mock-upload-url",
		"public_url": "https://s3.aws.com/bucket/mock-file.mp4",
	})
}
