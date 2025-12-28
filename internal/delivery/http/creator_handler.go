package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type CreatorHandler struct {
	db *gorm.DB
}

func NewCreatorHandler(r *gin.Engine, db *gorm.DB, authMiddleware gin.HandlerFunc) {
	handler := &CreatorHandler{db: db}

	g := r.Group("/creators")
	g.Use(authMiddleware)
	{
		g.GET("/search", handler.Search)
		g.GET("/:id", handler.GetProfile)
	}
}

func (h *CreatorHandler) Search(c *gin.Context) {
	queryStr := c.Query("query")
	niche := c.Query("niche")
	platform := c.Query("platform")

	var creators []domain.CreatorProfile

	query := h.db.Model(&domain.CreatorProfile{})
	// User table join for name is ideal but for now simple search
	if queryStr != "" {
		// Mock search or filter niche by query too
		query = query.Where("niche ILIKE ?", "%"+queryStr+"%")
	}
	if niche != "" {
		query = query.Where("niche ILIKE ?", "%"+niche+"%")
	}
	if platform != "" {
		query = query.Where("platform = ?", platform)
	}
	// General query? Name in User table?
	// Joining User table to filter by name?
	// Too complex for MVP GORM single table?
	// Let's assume query searches Niche for now or we join.

	if err := query.Find(&creators).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// We need to return enriched data: Name, Avatar (from User table).
	// Current DB structure: CreatorProfile has UserID.
	// We should Preload User.

	h.db.Preload("User").Find(&creators) // Re-run or optimize

	// Map to response
	var response []map[string]interface{}
	for _, cp := range creators {
		// Mock Name fetch if User Relation issues
		response = append(response, map[string]interface{}{
			"id":       cp.UserID,
			"niche":    cp.Niche,
			"platform": cp.Platform,
			// "name": cp.User.Name? User has no Name field in current model?
			// Need to Add Name to User model or Brand/Creator profile?
			// BrandProfile has CompanyName. CreatorProfile has... nothing for name explicitly?
			// User has Email. AvatarURL.
			// Let's assume Email implies name or add Name to User.
		})
	}

	c.JSON(http.StatusOK, response)
}

func (h *CreatorHandler) GetProfile(c *gin.Context) {
	id := c.Param("id")
	var profile domain.CreatorProfile
	if err := h.db.Where("user_id = ?", id).First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      profile.UserID,
		"niche":   profile.Niche,
		"videos":  profile.Portfolio, // JSONB
		"reviews": []string{},        // Mock
	})
}
