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

	// Public brand profile — accessible to creators
	r.Group("/api/brands").Use(authMiddleware).GET("/:id", handler.GetPublicProfile)
}

type updateBrandProfileRequest struct {
	BrandName         string `json:"brand_name"`
	ContactName       string `json:"contact_name"`
	Phone             string `json:"phone"`
	RoleInCompany     string `json:"role_in_company"`
	Website           string `json:"website"`
	Industry          string `json:"industry"`
	Description       string `json:"description"`
	FoundedYear       int    `json:"founded_year"`
	CompanySize       string `json:"company_size"`
	Headquarters      string `json:"headquarters"`
	InstagramURL      string `json:"instagram_url"`
	TwitterURL        string `json:"twitter_url"`
	LinkedinURL       string `json:"linkedin_url"`
	ProductCategories string `json:"product_categories"`
	TargetAudience    string `json:"target_audience"`
	CampaignTypes     string `json:"campaign_types"`
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
			UserID:            user.ID,
			CompanyName:       req.BrandName,
			ContactName:       req.ContactName,
			Phone:             req.Phone,
			RoleInCompany:     req.RoleInCompany,
			Website:           req.Website,
			LogoURL:           logoURL,
			Industry:          req.Industry,
			Description:       req.Description,
			FoundedYear:       req.FoundedYear,
			CompanySize:       req.CompanySize,
			Headquarters:      req.Headquarters,
			InstagramURL:      req.InstagramURL,
			TwitterURL:        req.TwitterURL,
			LinkedinURL:       req.LinkedinURL,
			ProductCategories: req.ProductCategories,
			TargetAudience:    req.TargetAudience,
			CampaignTypes:     req.CampaignTypes,
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
		if req.Industry != "" { profile.Industry = req.Industry }
		if req.Description != "" { profile.Description = req.Description }
		if req.FoundedYear > 0 { profile.FoundedYear = req.FoundedYear }
		if req.CompanySize != "" { profile.CompanySize = req.CompanySize }
		if req.Headquarters != "" { profile.Headquarters = req.Headquarters }
		if req.InstagramURL != "" { profile.InstagramURL = req.InstagramURL }
		if req.TwitterURL != "" { profile.TwitterURL = req.TwitterURL }
		if req.LinkedinURL != "" { profile.LinkedinURL = req.LinkedinURL }
		if req.ProductCategories != "" { profile.ProductCategories = req.ProductCategories }
		if req.TargetAudience != "" { profile.TargetAudience = req.TargetAudience }
		if req.CampaignTypes != "" { profile.CampaignTypes = req.CampaignTypes }

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
		"user_id":            profile.UserID,
		"company_name":       profile.CompanyName,
		"contact_name":       profile.ContactName,
		"phone":              profile.Phone,
		"role_in_company":    profile.RoleInCompany,
		"gst_number":         profile.GSTNumber,
		"website":            profile.Website,
		"logo_url":           profile.LogoURL,
		"industry":           profile.Industry,
		"description":        profile.Description,
		"founded_year":       profile.FoundedYear,
		"company_size":       profile.CompanySize,
		"headquarters":       profile.Headquarters,
		"instagram_url":      profile.InstagramURL,
		"twitter_url":        profile.TwitterURL,
		"linkedin_url":       profile.LinkedinURL,
		"product_categories": profile.ProductCategories,
		"target_audience":    profile.TargetAudience,
		"campaign_types":     profile.CampaignTypes,
		"wallet_balance":     profile.WalletBalance,
		"updated_at":         profile.UpdatedAt,
		"has_subscription":   hasSubscription,
		"is_subscribed":      isActive,
		"subscription":       &subscription,
		"plan":               planDetails,
	})
}

// GetPublicProfile returns public brand info accessible to any authenticated user
func (h *BrandHandler) GetPublicProfile(c *gin.Context) {
	brandID := c.Param("id")
	var profile domain.BrandProfile
	if err := h.db.Where("user_id = ?", brandID).First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Brand not found"})
		return
	}
	var user domain.User
	h.db.Where("id = ?", brandID).First(&user)

	c.JSON(http.StatusOK, gin.H{
		"user_id":            profile.UserID,
		"company_name":       profile.CompanyName,
		"contact_name":       profile.ContactName,
		"logo_url":           profile.LogoURL,
		"website":            profile.Website,
		"industry":           profile.Industry,
		"description":        profile.Description,
		"founded_year":       profile.FoundedYear,
		"company_size":       profile.CompanySize,
		"headquarters":       profile.Headquarters,
		"instagram_url":      profile.InstagramURL,
		"twitter_url":        profile.TwitterURL,
		"linkedin_url":       profile.LinkedinURL,
		"product_categories": profile.ProductCategories,
		"target_audience":    profile.TargetAudience,
		"campaign_types":     profile.CampaignTypes,
	})
}
