package http

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"github.com/vaibhaw/influenzer-backend/pkg/utils"
	"gorm.io/gorm"
)

// logoURLFromWebsite derives a Clearbit logo URL from a website string.
// Returns empty string if website is empty or unparseable.
func logoURLFromWebsite(website string) string {
	if website == "" {
		return ""
	}
	if !strings.HasPrefix(website, "http://") && !strings.HasPrefix(website, "https://") {
		website = "https://" + website
	}
	u, err := url.Parse(website)
	if err != nil || u.Host == "" {
		return ""
	}
	domain := strings.TrimPrefix(u.Host, "www.")
	return fmt.Sprintf("https://img.logo.dev/%s?token=pk_LsUq4TdcQ3mx-6UhuUlFoQ", domain)
}

type BrandHandler struct {
	db *gorm.DB
}

func NewBrandHandler(r *gin.Engine, db *gorm.DB, authMiddleware gin.HandlerFunc) {
	handler := &BrandHandler{db: db}

	// API prefix group for mobile/frontend compatibility
	apiGroup := r.Group("/api/brands")
	apiGroup.Use(authMiddleware)
	{
		apiGroup.PUT("/profile", handler.UpdateProfile)
		apiGroup.GET("/profile", handler.GetProfile)
	}
}

type updateBrandProfileRequest struct {
	BrandName     string `json:"brand_name"`
	ContactName   string `json:"contact_name"`
	Phone         string `json:"phone"`
	RoleInCompany string `json:"role_in_company"`
	Website       string `json:"website"`
}

// UpdateProfile godoc
// @Summary Update Brand Profile
// @Description Create or update the brand profile for the authenticated user
// @Tags brands
// @Accept json
// @Produce json
// @Param request body updateBrandProfileRequest true "Brand Profile Details"
// @Success 200 {object} map[string]interface{} "success and profile"
// @Failure 400 {object} map[string]interface{} "error"
// @Failure 401 {object} map[string]interface{} "error"
// @Failure 403 {object} map[string]interface{} "error"
// @Failure 404 {object} map[string]interface{} "error"
// @Failure 500 {object} map[string]interface{} "error"
// @Security BearerAuth
// @Router /api/brands/profile [put]
// UpdateProfile updates or creates the brand profile for the authenticated user
func (h *BrandHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req updateBrandProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Logger.Error("UpdateProfile Bind Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Check if user is a brand
	var user domain.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.Role != domain.RoleBrand {
		c.JSON(http.StatusForbidden, gin.H{"error": "User is not a brand"})
		return
	}

	// Check if profile exists
	var profile domain.BrandProfile
	err := h.db.Where("user_id = ?", userID).First(&profile).Error

	logoURL := logoURLFromWebsite(req.Website)

	if err == gorm.ErrRecordNotFound {
		// Create new profile
		profile = domain.BrandProfile{
			UserID:        user.ID,
			CompanyName:   req.BrandName,
			ContactName:   req.ContactName,
			Phone:         req.Phone,
			RoleInCompany: req.RoleInCompany,
			Website:       req.Website,
			LogoURL:       logoURL,
		}
		if err := h.db.Create(&profile).Error; err != nil {
			utils.Logger.Error("Failed to create brand profile: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create profile"})
			return
		}
	} else if err != nil {
		utils.Logger.Error("Database error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	} else {
		// Update existing profile
		profile.CompanyName = req.BrandName
		profile.ContactName = req.ContactName
		profile.Phone = req.Phone
		profile.RoleInCompany = req.RoleInCompany
		profile.Website = req.Website
		profile.LogoURL = logoURL

		if err := h.db.Save(&profile).Error; err != nil {
			utils.Logger.Error("Failed to update brand profile: " + err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"profile": profile,
	})
}

// GetProfile godoc
// @Summary Get Brand Profile
// @Description Retrieve the brand profile for the authenticated user
// @Tags brands
// @Produce json
// @Success 200 {object} map[string]interface{} "profile details"
// @Failure 401 {object} map[string]interface{} "error"
// @Failure 404 {object} map[string]interface{} "error"
// @Failure 500 {object} map[string]interface{} "error"
// @Security BearerAuth
// @Router /api/brands/profile [get]
// GetProfile retrieves the brand profile for the authenticated user
func (h *BrandHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var profile domain.BrandProfile
	if err := h.db.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Profile not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Fetch subscription status
	var subscription domain.Subscription
	hasSubscription := false
	isActive := false
	var planDetails *domain.SubscriptionPlan

	err := h.db.Where("user_id = ? AND status = ?", userID, "active").
		Order("created_at DESC").
		First(&subscription).Error

	if err == nil {
		hasSubscription = true
		// Status is already filtered to "active", just check date validity
		isActive = subscription.EndDate.IsZero() || subscription.EndDate.After(time.Now())

		// Fetch plan details
		var plan domain.SubscriptionPlan
		if err := h.db.Where("id = ?", subscription.PlanID).First(&plan).Error; err == nil {
			planDetails = &plan
		}
	}

	// Response with profile and subscription info
	c.JSON(http.StatusOK, gin.H{
		"user_id":          profile.UserID,
		"company_name":     profile.CompanyName,
		"contact_name":     profile.ContactName,
		"phone":            profile.Phone,
		"role_in_company":  profile.RoleInCompany,
		"gst_number":       profile.GSTNumber,
		"website":          profile.Website,
		"logo_url":         profile.LogoURL,
		"wallet_balance":   profile.WalletBalance,
		"updated_at":       profile.UpdatedAt,
		"has_subscription": hasSubscription,
		"is_subscribed":    isActive,
		"subscription":     &subscription,
		"plan":             planDetails,
	})
}
