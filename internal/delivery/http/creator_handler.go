package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"github.com/vaibhaw/influenzer-backend/internal/service"
	"gorm.io/gorm"
)

// CreatorMediaItem is a unified media item returned for both Instagram and YouTube.
type CreatorMediaItem struct {
	Platform     string `json:"platform"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	Caption      string `json:"caption"`
	ThumbnailURL string `json:"thumbnail_url"`
	MediaURL     string `json:"media_url"`
	Permalink    string `json:"permalink"`
	MediaType    string `json:"media_type"` // VIDEO, IMAGE, CAROUSEL_ALBUM, YOUTUBE
	ViewCount    int64  `json:"view_count"`
	LikeCount    int64  `json:"like_count"`
	CommentCount int64  `json:"comment_count"`
	Duration     int64  `json:"duration"` // seconds (YouTube only)
	PublishedAt  string `json:"published_at"`
}

type CreatorHandler struct {
	db              *gorm.DB
	geminiAPIKey    string
	matchingService *service.MatchingService
}

func NewCreatorHandler(r *gin.Engine, db *gorm.DB, authMiddleware gin.HandlerFunc, geminiAPIKey string, matchingService *service.MatchingService) {
	handler := &CreatorHandler{
		db:              db,
		geminiAPIKey:    geminiAPIKey,
		matchingService: matchingService,
	}

	g := r.Group("/creators")
	g.Use(authMiddleware)
	{
		g.GET("/search", handler.Search)
		g.GET("/spotlight", handler.Spotlight)
		g.GET("/:id", handler.GetProfile)
		g.GET("/:id/media", handler.GetCreatorMedia)
	}

	// API prefix group for mobile/frontend compatibility
	apiGroup := r.Group("/api/creators")
	apiGroup.Use(authMiddleware)
	{
		apiGroup.GET("/profile", handler.GetMyProfile)
		apiGroup.POST("/refresh-stats", handler.RefreshStats)
		apiGroup.PUT("/profile", handler.UpdateProfile)
		apiGroup.GET("/profile/completion", handler.GetProfileCompletion)
	}

	// Add analytics endpoint
	g.GET("/:id/analytics", handler.GetAnalytics)
}

func (h *CreatorHandler) Search(c *gin.Context) {
	queryStr := c.Query("query")
	niche := c.Query("niche")
	platform := c.Query("platform")
	availability := c.Query("availability")
	gender := c.Query("gender")
	city := c.Query("city")
	minRate := c.Query("min_rate")
	maxRate := c.Query("max_rate")
	willingToTravel := c.Query("willing_to_travel")

	var creators []domain.CreatorProfile

	query := h.db.Preload("User").Model(&domain.CreatorProfile{})

	// Search by name in User table if query string provided
	if queryStr != "" {
		query = query.Joins("JOIN users ON users.id = creator_profiles.user_id").
			Where("users.name ILIKE ? OR creator_profiles.niche ILIKE ? OR creator_profiles.headline ILIKE ?",
				"%"+queryStr+"%", "%"+queryStr+"%", "%"+queryStr+"%")
	}

	if niche != "" {
		query = query.Where("niche ILIKE ?", "%"+niche+"%")
	}

	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	// New filters
	if availability != "" {
		query = query.Where("availability_status = ?", availability)
	}
	if gender != "" {
		query = query.Where("gender = ?", gender)
	}
	if city != "" {
		query = query.Where("city ILIKE ?", "%"+city+"%")
	}
	if minRate != "" {
		if v, err := strconv.ParseFloat(minRate, 64); err == nil {
			query = query.Where("min_budget >= ?", v)
		}
	}
	if maxRate != "" {
		if v, err := strconv.ParseFloat(maxRate, 64); err == nil {
			query = query.Where("min_budget <= ?", v)
		}
	}
	if willingToTravel == "true" {
		query = query.Where("willing_to_travel = ?", true)
	}

	if err := query.Find(&creators).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Map to enriched response with all requested fields
	var response []map[string]interface{}
	for _, cp := range creators {
		// Extract Instagram details from cached_stats
		var instagramID, instagramUsername, instagramURL string
		var instagramFollowers interface{}
		if cp.CachedStats != nil {
			if igData, ok := cp.CachedStats["instagram"].(map[string]interface{}); ok {
				if id, ok := igData["id"].(string); ok {
					instagramID = id
				}
				if username, ok := igData["username"].(string); ok {
					instagramUsername = username
					instagramURL = "https://instagram.com/" + username
				}
				if followers, ok := igData["followers_count"]; ok {
					instagramFollowers = followers
				}
			}
		}

		// Extract YouTube details from cached_stats
		var youtubeChannelID, youtubeChannelTitle, youtubeURL string
		var youtubeSubscribers interface{}
		if cp.CachedStats != nil {
			if ytData, ok := cp.CachedStats["youtube"].(map[string]interface{}); ok {
				if channelURL, ok := ytData["channel_url"].(string); ok {
					youtubeChannelID = channelURL
					youtubeURL = "https://youtube.com/" + channelURL
				}
				if title, ok := ytData["channel_title"].(string); ok {
					youtubeChannelTitle = title
				}
				if subscribers, ok := ytData["subscriber_count"]; ok {
					youtubeSubscribers = subscribers
				}
			}
		}

		// Get phone from CreatorProfile
		phone := cp.Phone

		creatorData := map[string]interface{}{
			"id":         cp.UserID,
			"name":       cp.User.Name,
			"email":      cp.User.Email,
			"phone":      phone,
			"niche":      cp.Niche,
			"min_budget": cp.MinBudget,
			"platform":   cp.Platform,
			"city":       cp.City,

			// Instagram details
			"instagram_id":        instagramID,
			"instagram_username":  instagramUsername,
			"instagram_url":       instagramURL,
			"instagram_followers": instagramFollowers,

			// YouTube details
			"youtube_channel_id":    youtubeChannelID,
			"youtube_channel_title": youtubeChannelTitle,
			"youtube_url":           youtubeURL,
			"youtube_subscribers":   youtubeSubscribers,

			// Additional info
			"avatar_url":   cp.User.AvatarURL,
			"cached_stats": cp.CachedStats,

			// Extended profile
			"bio":                cp.Bio,
			"languages":          cp.Languages,
			"years_experience":   cp.YearsExperience,
			"content_categories": cp.ContentCategories,
			"past_brands":        cp.PastBrands,
			"rate_card":          cp.RateCard,
			"social_links":       cp.SocialLinks,

			// New fields
			"headline":             cp.Headline,
			"gender":               cp.Gender,
			"availability_status":  cp.AvailabilityStatus,
			"turnaround_days":      cp.TurnaroundDays,
			"willing_to_travel":    cp.WillingToTravel,
			"profile_complete":     cp.CalculateCompletion(),
			"collaboration_prefs":  cp.CollaborationPrefs,
			"past_work":            cp.PastWork,
			"response_time":        cp.ResponseTime,
		}

		response = append(response, creatorData)
	}

	c.JSON(http.StatusOK, response)
}

// Spotlight returns up to 8 creators who have connected platforms, ordered by
// earliest sign-up — giving early adopters guaranteed visibility.
func (h *CreatorHandler) Spotlight(c *gin.Context) {
	var creators []domain.CreatorProfile

	h.db.Preload("User").
		Joins("JOIN users ON users.id = creator_profiles.user_id").
		Where("creator_profiles.niche != '' AND creator_profiles.cached_stats IS NOT NULL").
		Order("users.created_at ASC").
		Limit(8).
		Find(&creators)

	var response []map[string]interface{}
	for _, cp := range creators {
		var instagramUsername, instagramURL string
		var instagramFollowers interface{}
		if ig, ok := cp.CachedStats["instagram"].(map[string]interface{}); ok {
			if u, ok := ig["username"].(string); ok {
				instagramUsername = u
				instagramURL = "https://instagram.com/" + u
			}
			instagramFollowers = ig["followers_count"]
		}

		var youtubeChannelID, youtubeChannelTitle, youtubeURL string
		var youtubeSubscribers interface{}
		if yt, ok := cp.CachedStats["youtube"].(map[string]interface{}); ok {
			if cu, ok := yt["channel_url"].(string); ok {
				youtubeChannelID = cu
				youtubeURL = "https://youtube.com/" + cu
			}
			if t, ok := yt["channel_title"].(string); ok {
				youtubeChannelTitle = t
			}
			youtubeSubscribers = yt["subscriber_count"]
		}

		avatarURL := cp.User.AvatarURL
		if cs := cp.CachedStats; cs != nil {
			if ig, ok := cs["instagram"].(map[string]interface{}); ok {
				if pic, ok := ig["profile_picture"].(string); ok && pic != "" {
					avatarURL = pic
				}
			} else if yt, ok := cs["youtube"].(map[string]interface{}); ok {
				if thumb, ok := yt["thumbnail"].(string); ok && thumb != "" && avatarURL == "" {
					avatarURL = thumb
				}
			}
		}

		response = append(response, map[string]interface{}{
			"id":                    cp.UserID,
			"name":                  cp.User.Name,
			"niche":                 cp.Niche,
			"min_budget":            cp.MinBudget,
			"platform":              cp.Platform,
			"city":                  cp.City,
			"avatar_url":            avatarURL,
			"cached_stats":          cp.CachedStats,
			"instagram_username":    instagramUsername,
			"instagram_url":         instagramURL,
			"instagram_followers":   instagramFollowers,
			"youtube_channel_id":    youtubeChannelID,
			"youtube_channel_title": youtubeChannelTitle,
			"youtube_url":           youtubeURL,
			"youtube_subscribers":   youtubeSubscribers,
		})
	}

	if response == nil {
		response = []map[string]interface{}{}
	}
	c.JSON(http.StatusOK, response)
}

func (h *CreatorHandler) GetProfile(c *gin.Context) {
	id := c.Param("id")
	var profile domain.CreatorProfile
	if err := h.db.Where("user_id = ?", id).Preload("User").First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator not found"})
		return
	}

	// Compute campaign stats from proposals
	var totalCampaigns, completedCampaigns int64
	h.db.Model(&domain.Proposal{}).Where("creator_id = ?", profile.UserID).Count(&totalCampaigns)
	h.db.Model(&domain.Proposal{}).Where("creator_id = ? AND status IN ?", profile.UserID, []string{"COMPLETED", "PAID"}).Count(&completedCampaigns)

	c.JSON(http.StatusOK, gin.H{
		"id":         profile.UserID,
		"name":       profile.User.Name,
		"email":      profile.User.Email,
		"avatar_url": profile.User.AvatarURL,

		// Basic info
		"niche":              profile.Niche,
		"min_budget":         profile.MinBudget,
		"city":               profile.City,
		"platform":           profile.Platform,
		"bio":                profile.Bio,
		"languages":          profile.Languages,
		"years_experience":   profile.YearsExperience,
		"content_categories": profile.ContentCategories,
		"past_brands":        profile.PastBrands,
		"rate_card":          profile.RateCard,
		"social_links":       profile.SocialLinks,

		// Professional identity
		"headline":         profile.Headline,
		"gender":           profile.Gender,
		"date_of_birth":    profile.DateOfBirth,
		"profile_complete": profile.CalculateCompletion(),

		// Availability & logistics
		"availability_status": profile.AvailabilityStatus,
		"turnaround_days":     profile.TurnaroundDays,
		"location":            profile.Location,
		"pin_code":            profile.PinCode,
		"willing_to_travel":   profile.WillingToTravel,

		// Rich data
		"audience_demographics": profile.AudienceDemographics,
		"collaboration_prefs":   profile.CollaborationPrefs,
		"past_work":             profile.PastWork,
		"portfolio":             profile.Portfolio,
		"response_time":         profile.ResponseTime,

		// Performance metrics (computed)
		"total_campaigns":     totalCampaigns,
		"completed_campaigns": completedCampaigns,
		"avg_rating":          profile.AvgRating,

		// Platform stats
		"cached_stats": profile.CachedStats,
		"reviews":      []string{},
	})
}

type updateProfileRequest struct {
	// Basic info
	Bio               string                 `json:"bio"`
	Languages         string                 `json:"languages"`
	YearsExperience   int                    `json:"years_experience"`
	ContentCategories string                 `json:"content_categories"`
	PastBrands        string                 `json:"past_brands"`
	RateCard          map[string]interface{} `json:"rate_card"`
	SocialLinks       map[string]interface{} `json:"social_links"`
	Niche             string                 `json:"niche"`
	MinBudget         float64                `json:"min_budget"`
	City              string                 `json:"city"`
	Platform          string                 `json:"platform"`
	Phone             string                 `json:"phone"`

	// Professional identity
	Headline    string  `json:"headline"`
	Gender      string  `json:"gender"`
	DateOfBirth *string `json:"date_of_birth"` // ISO 8601 string, parsed server-side

	// Availability & logistics
	AvailabilityStatus string `json:"availability_status"`
	TurnaroundDays     int    `json:"turnaround_days"`
	Location           string `json:"location"`
	PinCode            string `json:"pin_code"`
	WillingToTravel    *bool  `json:"willing_to_travel"` // pointer to distinguish false from not-provided

	// Audience demographics (JSONB)
	AudienceDemographics map[string]interface{} `json:"audience_demographics"`

	// Collaboration preferences (JSONB)
	CollaborationPrefs map[string]interface{} `json:"collaboration_prefs"`

	// Structured past work (JSONB array)
	PastWork []map[string]interface{} `json:"past_work"`

	// Portfolio items (JSONB array)
	Portfolio []map[string]interface{} `json:"portfolio"`

	// Performance / self-reported
	ResponseTime string `json:"response_time"`
}

func (h *CreatorHandler) UpdateProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var profile domain.CreatorProfile
	result := h.db.Where("user_id = ?", userID).First(&profile)
	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator profile not found"})
		return
	}

	// Update only provided fields — basic info
	if req.Bio != "" { profile.Bio = req.Bio }
	if req.Languages != "" { profile.Languages = req.Languages }
	if req.YearsExperience > 0 { profile.YearsExperience = req.YearsExperience }
	if req.ContentCategories != "" { profile.ContentCategories = req.ContentCategories }
	if req.PastBrands != "" { profile.PastBrands = req.PastBrands }
	if req.RateCard != nil { profile.RateCard = req.RateCard }
	if req.SocialLinks != nil { profile.SocialLinks = req.SocialLinks }
	if req.Niche != "" { profile.Niche = req.Niche }
	if req.MinBudget > 0 { profile.MinBudget = req.MinBudget }
	if req.City != "" { profile.City = req.City }
	if req.Platform != "" { profile.Platform = req.Platform }
	if req.Phone != "" { profile.Phone = req.Phone }

	// Professional identity
	if req.Headline != "" { profile.Headline = req.Headline }
	if req.Gender != "" { profile.Gender = req.Gender }
	if req.DateOfBirth != nil {
		if t, err := time.Parse("2006-01-02", *req.DateOfBirth); err == nil {
			profile.DateOfBirth = &t
		}
	}

	// Availability & logistics
	if req.AvailabilityStatus != "" { profile.AvailabilityStatus = req.AvailabilityStatus }
	if req.TurnaroundDays > 0 { profile.TurnaroundDays = req.TurnaroundDays }
	if req.Location != "" { profile.Location = req.Location }
	if req.PinCode != "" { profile.PinCode = req.PinCode }
	if req.WillingToTravel != nil { profile.WillingToTravel = *req.WillingToTravel }

	// JSONB fields — only overwrite if provided (non-nil)
	if req.AudienceDemographics != nil { profile.AudienceDemographics = req.AudienceDemographics }
	if req.CollaborationPrefs != nil { profile.CollaborationPrefs = req.CollaborationPrefs }
	if req.PastWork != nil { profile.PastWork = req.PastWork }
	if req.Portfolio != nil { profile.Portfolio = req.Portfolio }
	if req.ResponseTime != "" { profile.ResponseTime = req.ResponseTime }

	// Recalculate profile completion
	profile.ProfileComplete = profile.CalculateCompletion()

	if err := h.db.Save(&profile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	// Asynchronously update Gemini embedding for AI matching
	if h.matchingService != nil {
		go func(uid uuid.UUID) {
			if err := h.matchingService.UpdateCreatorEmbedding(uid); err != nil {
				fmt.Printf("Failed to update creator embedding on profile update: %v\n", err)
			}
		}(profile.UserID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Profile updated", "profile_complete": profile.ProfileComplete})
}

// GetProfileCompletion returns the profile completeness percentage and missing sections
func (h *CreatorHandler) GetProfileCompletion(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var profile domain.CreatorProfile
	if err := h.db.Where("user_id = ?", userID).First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator profile not found"})
		return
	}

	missing := []map[string]interface{}{}

	if profile.Headline == "" {
		missing = append(missing, map[string]interface{}{"section": "headline", "label": "Add a professional headline", "action": "edit_about"})
	}
	if profile.Bio == "" {
		missing = append(missing, map[string]interface{}{"section": "bio", "label": "Write your bio", "action": "edit_about"})
	}
	if profile.City == "" && profile.Location == "" {
		missing = append(missing, map[string]interface{}{"section": "location", "label": "Add your location", "action": "edit_about"})
	}
	if profile.Gender == "" && profile.DateOfBirth == nil {
		missing = append(missing, map[string]interface{}{"section": "demographics", "label": "Add gender & date of birth", "action": "edit_about"})
	}
	if profile.Languages == "" {
		missing = append(missing, map[string]interface{}{"section": "languages", "label": "Add languages you speak", "action": "edit_about"})
	}
	if profile.ContentCategories == "" {
		missing = append(missing, map[string]interface{}{"section": "categories", "label": "Add content categories", "action": "edit_about"})
	}
	if len(profile.RateCard) < 3 {
		missing = append(missing, map[string]interface{}{"section": "rate_card", "label": "Complete your rate card", "action": "edit_rate_card"})
	}
	if len(profile.PastWork) == 0 && profile.PastBrands == "" {
		missing = append(missing, map[string]interface{}{"section": "past_work", "label": "Add past work & collaborations", "action": "edit_past_work"})
	}
	if len(profile.SocialLinks) == 0 {
		missing = append(missing, map[string]interface{}{"section": "social_links", "label": "Add social links", "action": "edit_about"})
	}
	if len(profile.AudienceDemographics) == 0 {
		missing = append(missing, map[string]interface{}{"section": "audience", "label": "Add audience demographics", "action": "edit_audience"})
	}

	c.JSON(http.StatusOK, gin.H{
		"profile_complete": profile.CalculateCompletion(),
		"missing_sections": missing,
	})
}

// GetAnalytics computes engagement metrics from cached stats and recent media
func (h *CreatorHandler) GetAnalytics(c *gin.Context) {
	creatorID := c.Param("id")

	var user domain.User
	if err := h.db.Preload("CreatorProfile").Where("id = ?", creatorID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator not found"})
		return
	}

	profile := user.CreatorProfile
	if profile == nil || profile.CachedStats == nil {
		c.JSON(http.StatusOK, gin.H{"available": false})
		return
	}

	analytics := map[string]interface{}{"available": true}

	// ── Instagram analytics ──────────────────────────────────────────────────
	if igData, ok := profile.CachedStats["instagram"].(map[string]interface{}); ok {
		igFollowers := toInt64(igData["followers_count"])
		igFollowing := toInt64(igData["follows_count"])
		igMediaCount := toInt64(igData["media_count"])
		igUserID, _ := igData["id"].(string)

		igAnalytics := map[string]interface{}{
			"followers":   igFollowers,
			"following":   igFollowing,
			"media_count": igMediaCount,
			"tier":        creatorTier(igFollowers),
		}

		if user.InstagramToken != "" {
			// Fetch recent 20 posts with likes, views, comments
			if items, err := h.fetchInstagramMedia(user.InstagramToken, 20); err == nil && len(items) > 0 {
				var totalViews, totalLikes, totalComments int64
				for _, item := range items {
					totalViews += item.ViewCount
					totalLikes += item.LikeCount
					totalComments += item.CommentCount
				}
				count := int64(len(items))
				avgViews := totalViews / count
				avgLikes := totalLikes / count
				avgComments := totalComments / count

				// Engagement rate = (avg_likes + avg_comments) / followers * 100
				var engRate float64
				if igFollowers > 0 {
					engRate = float64(avgLikes+avgComments) / float64(igFollowers) * 100
				}

				igAnalytics["avg_views"]       = avgViews
				igAnalytics["avg_likes"]       = avgLikes
				igAnalytics["avg_comments"]    = avgComments
				igAnalytics["engagement_rate"] = fmt.Sprintf("%.2f", engRate)
				igAnalytics["posts_analyzed"]  = count

				// Per-post insights: fetch shares, saves, reach for first 10 posts
				sampleSize := 10
				if len(items) < sampleSize {
					sampleSize = len(items)
				}
				var totalShares, totalSaves, totalReach int64
				fetched := 0
				for i := 0; i < sampleSize; i++ {
					reach, shares, saves, ok := h.fetchInstagramPostInsights(user.InstagramToken, items[i].ID)
					if ok {
						totalReach += reach
						totalShares += shares
						totalSaves += saves
						fetched++
					}
				}
				if fetched > 0 {
					n := int64(fetched)
					igAnalytics["avg_reach"]  = totalReach / n
					igAnalytics["avg_shares"] = totalShares / n
					igAnalytics["avg_saves"]  = totalSaves / n
				}
			}

			// Account-level 28-day insights: reach, impressions, profile views
			if igUserID != "" {
				if reach28d, impressions28d, profileViews28d, err := h.fetchInstagramAccountInsights(user.InstagramToken, igUserID); err == nil {
					if reach28d > 0 { igAnalytics["reach_28d"] = reach28d }
					if impressions28d > 0 { igAnalytics["impressions_28d"] = impressions28d }
					if profileViews28d > 0 { igAnalytics["profile_views_28d"] = profileViews28d }
				}
			}
		}
		analytics["instagram"] = igAnalytics
	}

	// ── YouTube analytics ────────────────────────────────────────────────────
	if ytData, ok := profile.CachedStats["youtube"].(map[string]interface{}); ok {
		ytSubs := toInt64FromStr(fmt.Sprintf("%v", ytData["subscriber_count"]))
		ytVideos := toInt64FromStr(fmt.Sprintf("%v", ytData["video_count"]))
		ytTotalViews := toInt64FromStr(fmt.Sprintf("%v", ytData["view_count"]))

		ytAnalytics := map[string]interface{}{
			"subscribers": ytSubs,
			"video_count": ytVideos,
			"total_views": ytTotalViews,
			"tier":        creatorTier(ytSubs),
		}

		// Channel metadata from cached stats
		if country, ok := ytData["country"].(string); ok && country != "" {
			ytAnalytics["country"] = country
		}
		if publishedAt, ok := ytData["published_at"].(string); ok && publishedAt != "" {
			ytAnalytics["channel_age_years"] = channelAgeYears(publishedAt)
		}

		if user.YoutubeToken != "" {
			// Per-video averages from last 20 videos
			items, err := h.fetchYouTubeVideos(user.YoutubeToken, 20)
			if err != nil && isTokenError(err.Error()) && user.YoutubeRefreshToken != "" {
				if newToken, refreshErr := h.refreshYouTubeToken(&user); refreshErr == nil {
					items, err = h.fetchYouTubeVideos(newToken, 20)
				}
			}

			if err == nil && len(items) > 0 {
				var totalViews, totalLikes, totalComments, totalDuration int64
				for _, item := range items {
					totalViews += item.ViewCount
					totalLikes += item.LikeCount
					totalComments += item.CommentCount
					totalDuration += item.Duration
				}
				count := int64(len(items))
				avgViews := totalViews / count
				avgLikes := totalLikes / count
				avgComments := totalComments / count
				avgDuration := totalDuration / count // seconds

				var engRate float64
				if ytSubs > 0 {
					engRate = float64(avgLikes+avgComments) / float64(ytSubs) * 100
				}

				ytAnalytics["avg_views"]       = avgViews
				ytAnalytics["avg_likes"]       = avgLikes
				ytAnalytics["avg_comments"]    = avgComments
				ytAnalytics["avg_duration"]    = avgDuration
				ytAnalytics["engagement_rate"] = fmt.Sprintf("%.2f", engRate)
				ytAnalytics["videos_analyzed"] = count
			}

			// YouTube Analytics API: 28-day aggregate (requires yt-analytics.readonly scope)
			analyticsData, err := h.fetchYouTubeAnalytics(user.YoutubeToken, 28)
			if err != nil && isTokenError(err.Error()) && user.YoutubeRefreshToken != "" {
				if newToken, refreshErr := h.refreshYouTubeToken(&user); refreshErr == nil {
					analyticsData, err = h.fetchYouTubeAnalytics(newToken, 28)
				}
			}
			if err == nil {
				ytAnalytics["analytics_28d"] = analyticsData
			}
		}
		analytics["youtube"] = ytAnalytics
	}

	c.JSON(http.StatusOK, analytics)
}

func creatorTier(followers int64) string {
	switch {
	case followers >= 1_000_000:
		return "Mega"
	case followers >= 100_000:
		return "Macro"
	case followers >= 10_000:
		return "Micro"
	default:
		return "Nano"
	}
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case string:
		var n int64
		fmt.Sscanf(val, "%d", &n)
		return n
	}
	return 0
}

func toInt64FromStr(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// GetMyProfile returns the authenticated user's creator profile
func (h *CreatorHandler) GetMyProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var user domain.User
	if err := h.db.Preload("CreatorProfile").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Check if user has creator role
	if user.Role != domain.RoleCreator {
		c.JSON(http.StatusForbidden, gin.H{"error": "User is not a creator"})
		return
	}

	// Build connected platforms list with reconnection status
	connectedPlatforms := []map[string]interface{}{}
	if user.InstagramToken != "" {
		needsReconnect := false
		if user.CreatorProfile != nil && user.CreatorProfile.CachedStats != nil {
			if errMsg, hasError := user.CreatorProfile.CachedStats["instagram_error"].(string); hasError {
				needsReconnect = isTokenError(errMsg)
			}
		}
		connectedPlatforms = append(connectedPlatforms, map[string]interface{}{
			"platform":           "instagram",
			"connected":          true,
			"needs_reconnection": needsReconnect,
		})
	}
	if user.YoutubeToken != "" {
		needsReconnect := false
		if user.CreatorProfile != nil && user.CreatorProfile.CachedStats != nil {
			if errMsg, hasError := user.CreatorProfile.CachedStats["youtube_error"].(string); hasError {
				needsReconnect = isTokenError(errMsg)
			}
		}
		connectedPlatforms = append(connectedPlatforms, map[string]interface{}{
			"platform":           "youtube",
			"connected":          true,
			"needs_reconnection": needsReconnect,
		})
	}

	// Build response
	response := gin.H{
		"id":                  user.ID,
		"email":               user.Email,
		"name":                user.Name,
		"avatar_url":          user.AvatarURL,
		"role":                user.Role,
		"connected_platforms": connectedPlatforms,
	}

	// Add creator profile data if exists
	if user.CreatorProfile != nil {
		cp := user.CreatorProfile
		response["niche"] = cp.Niche
		response["min_budget"] = cp.MinBudget
		response["city"] = cp.City
		response["platform"] = cp.Platform
		response["cached_stats"] = cp.CachedStats
		response["portfolio"] = cp.Portfolio

		// Extended profile as nested object for the mobile app
		response["creator_profile"] = gin.H{
			"bio":                  cp.Bio,
			"languages":            cp.Languages,
			"years_experience":     cp.YearsExperience,
			"content_categories":   cp.ContentCategories,
			"past_brands":          cp.PastBrands,
			"rate_card":            cp.RateCard,
			"social_links":        cp.SocialLinks,
			"city":                 cp.City,
			"phone":                cp.Phone,
			"min_budget":           cp.MinBudget,
			"headline":             cp.Headline,
			"gender":               cp.Gender,
			"date_of_birth":        cp.DateOfBirth,
			"availability_status":  cp.AvailabilityStatus,
			"turnaround_days":      cp.TurnaroundDays,
			"location":             cp.Location,
			"pin_code":             cp.PinCode,
			"willing_to_travel":    cp.WillingToTravel,
			"audience_demographics": cp.AudienceDemographics,
			"collaboration_prefs":  cp.CollaborationPrefs,
			"past_work":            cp.PastWork,
			"portfolio":            cp.Portfolio,
			"response_time":        cp.ResponseTime,
		}
		response["profile_complete"] = cp.CalculateCompletion()

		// Compute campaign stats
		var totalCampaigns, completedCampaigns int64
		h.db.Model(&domain.Proposal{}).Where("creator_id = ?", user.ID).Count(&totalCampaigns)
		h.db.Model(&domain.Proposal{}).Where("creator_id = ? AND status IN ?", user.ID, []string{"COMPLETED", "PAID"}).Count(&completedCampaigns)
		response["total_campaigns"] = totalCampaigns
		response["completed_campaigns"] = completedCampaigns
		response["avg_rating"] = cp.AvgRating
		response["response_time"] = cp.ResponseTime
	}

	// Check subscription status
	var subscription domain.Subscription
	subscriptionActive := false
	var subscriptionPlan *domain.SubscriptionPlan

	if err := h.db.Where("user_id = ? AND status = ?", user.ID, "active").
		Order("created_at DESC").
		First(&subscription).Error; err == nil {
		// Check if subscription is still valid
		if subscription.EndDate.IsZero() || subscription.EndDate.After(time.Now()) {
			subscriptionActive = true
			// Fetch plan details
			var plan domain.SubscriptionPlan
			if err := h.db.Where("id = ?", subscription.PlanID).First(&plan).Error; err == nil {
				subscriptionPlan = &plan
			}
		}
	}

	response["is_subscribed"] = subscriptionActive
	if subscriptionActive && subscriptionPlan != nil {
		response["subscription"] = gin.H{
			"plan_name":  subscriptionPlan.Name,
			"plan_id":    subscriptionPlan.ID,
			"start_date": subscription.StartDate,
			"end_date":   subscription.EndDate,
			"status":     subscription.Status,
		}
	}

	c.JSON(http.StatusOK, response)
}

// RefreshStats fetches latest stats from connected platforms and caches them
func (h *CreatorHandler) RefreshStats(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var user domain.User
	if err := h.db.Preload("CreatorProfile").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if user.Role != domain.RoleCreator {
		c.JSON(http.StatusForbidden, gin.H{"error": "User is not a creator"})
		return
	}

	stats := make(map[string]interface{})

	// Fetch YouTube stats if token exists
	if user.YoutubeToken != "" {
		ytStats, err := h.fetchYouTubeStats(user.YoutubeToken)
		if err != nil && isTokenError(err.Error()) && user.YoutubeRefreshToken != "" {
			if newToken, refreshErr := h.refreshYouTubeToken(&user); refreshErr == nil {
				ytStats, err = h.fetchYouTubeStats(newToken)
			}
		}

		if err != nil {
			// Log but don't fail - token might be expired
			fmt.Printf("Failed to fetch YouTube stats: %v\n", err)
			stats["youtube_error"] = err.Error()
		} else {
			stats["youtube"] = ytStats

			// Fetch recent videos and perform Gemini AI Brand Suitability / reach analysis
			recentVideos, videoErr := h.fetchYouTubeVideos(user.YoutubeToken, 10)
			if videoErr != nil && isTokenError(videoErr.Error()) && user.YoutubeRefreshToken != "" {
				if newToken, refreshErr := h.refreshYouTubeToken(&user); refreshErr == nil {
					recentVideos, videoErr = h.fetchYouTubeVideos(newToken, 10)
				}
			}

			if videoErr != nil {
				fmt.Printf("Failed to fetch recent YouTube videos (continuing with channel stats only): %v\n", videoErr)
			}

			aiAnalysis, aiErr := h.generateYouTubeAIAnalysis(ytStats, recentVideos)
			if aiErr == nil {
				stats["youtube_ai_analysis"] = aiAnalysis
			} else {
				fmt.Printf("YouTube AI analysis generation failed: %v\n", aiErr)
			}
		}
	}

	// Fetch Instagram stats if token exists
	if user.InstagramToken != "" {
		igStats, err := h.fetchInstagramStats(user.InstagramToken)
		if err != nil {
			fmt.Printf("Failed to fetch Instagram stats: %v\n", err)
			stats["instagram_error"] = err.Error()
		} else {
			stats["instagram"] = igStats

			// Fetch reels from the last 30 days (cap at 15 to bound API calls)
			reels, reelsErr := h.fetchInstagramReels(user.InstagramToken, 30, 15)
			if reelsErr != nil {
				fmt.Printf("Failed to fetch Instagram reels: %v\n", reelsErr)
			} else if len(reels) > 0 {
				stats["instagram_reels"] = reels
				stats["instagram_reels_aggregates"] = computeInstagramReelAggregates(reels, toInt64(igStats["followers_count"]))
			}

			aiAnalysis, aiErr := h.generateInstagramAIAnalysis(igStats, reels)
			if aiErr == nil {
				stats["instagram_ai_analysis"] = aiAnalysis
			} else {
				fmt.Printf("Instagram AI analysis generation failed: %v\n", aiErr)
			}
		}
	}

	// Update or create creator profile with cached stats
	if user.CreatorProfile == nil {
		user.CreatorProfile = &domain.CreatorProfile{
			UserID:      user.ID,
			CachedStats: stats,
		}
		h.db.Create(user.CreatorProfile)
	} else {
		user.CreatorProfile.CachedStats = stats
		h.db.Save(user.CreatorProfile)
	}

	// Sync best available avatar into User.AvatarURL so chat list and other
	// places that use User.AvatarURL show the same picture as the creator profile.
	if igData, ok := stats["instagram"].(map[string]interface{}); ok {
		if pic, ok := igData["profile_picture"].(string); ok && pic != "" {
			user.AvatarURL = pic
			h.db.Model(&user).Update("avatar_url", pic)
		}
	} else if ytData, ok := stats["youtube"].(map[string]interface{}); ok {
		if thumb, ok := ytData["thumbnail"].(string); ok && thumb != "" {
			user.AvatarURL = thumb
			h.db.Model(&user).Update("avatar_url", thumb)
		}
	}

	// Check if reconnection is needed for any platform
	instagramNeedsReconnect := false
	youtubeNeedsReconnect := false

	if errMsg, hasError := stats["instagram_error"].(string); hasError {
		instagramNeedsReconnect = isTokenError(errMsg)
	}
	if errMsg, hasError := stats["youtube_error"].(string); hasError {
		youtubeNeedsReconnect = isTokenError(errMsg)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":                      true,
		"stats":                        stats,
		"instagram_needs_reconnection": instagramNeedsReconnect,
		"youtube_needs_reconnection":   youtubeNeedsReconnect,
	})
}

// isTokenError checks if an error message indicates a token/authentication issue
func isTokenError(errMsg string) bool {
	// Check for common token error patterns
	errorPatterns := []string{
		"expired",
		"invalid authentication",
		"invalid token",
		"invalid access token",
		"authentication credentials",
		"Session has expired",
	}

	for _, pattern := range errorPatterns {
		if len(errMsg) > 0 && contains(errMsg, pattern) {
			return true
		}
	}
	return false
}

// contains checks if a string contains a substring (case-insensitive)
func contains(str, substr string) bool {
	return strings.Contains(strings.ToLower(str), strings.ToLower(substr))
}

// YouTubeChannelResponse represents YouTube API response
type YouTubeChannelResponse struct {
	Items []struct {
		Statistics struct {
			SubscriberCount string `json:"subscriberCount"`
			ViewCount       string `json:"viewCount"`
			VideoCount      string `json:"videoCount"`
		} `json:"statistics"`
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			CustomURL   string `json:"customUrl"`
			PublishedAt string `json:"publishedAt"`
			Country     string `json:"country"`
			Thumbnails  struct {
				Default struct {
					URL string `json:"url"`
				} `json:"default"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (h *CreatorHandler) fetchYouTubeStats(accessToken string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/youtube/v3/channels?part=statistics,snippet&mine=true", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ytResp YouTubeChannelResponse
	if err := json.Unmarshal(body, &ytResp); err != nil {
		return nil, err
	}

	if ytResp.Error != nil {
		return nil, fmt.Errorf("YouTube API error: %s", ytResp.Error.Message)
	}

	if len(ytResp.Items) == 0 {
		return nil, fmt.Errorf("no YouTube channel found")
	}

	channel := ytResp.Items[0]
	return map[string]interface{}{
		"subscriber_count": channel.Statistics.SubscriberCount,
		"view_count":       channel.Statistics.ViewCount,
		"video_count":      channel.Statistics.VideoCount,
		"channel_title":    channel.Snippet.Title,
		"channel_url":      channel.Snippet.CustomURL,
		"thumbnail":        channel.Snippet.Thumbnails.Default.URL,
		"published_at":     channel.Snippet.PublishedAt,
		"country":          channel.Snippet.Country,
	}, nil
}

func (h *CreatorHandler) refreshYouTubeToken(user *domain.User) (string, error) {
	if user.YoutubeRefreshToken == "" {
		return "", fmt.Errorf("no refresh token available")
	}

	clientID := os.Getenv("GOOGLE_WEB_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_CLIENT_ID")
	}
	clientSecret := os.Getenv("GOOGLE_WEB_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	}

	if clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("google client credentials not configured")
	}

	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("client_secret", clientSecret)
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", user.YoutubeRefreshToken)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", values)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}

	if res.AccessToken == "" {
		return "", fmt.Errorf("no access token returned in response")
	}

	// Update GORM user model
	user.YoutubeToken = res.AccessToken
	if res.RefreshToken != "" {
		user.YoutubeRefreshToken = res.RefreshToken
	}

	// Persist to database
	if err := h.db.Model(user).Updates(map[string]interface{}{
		"youtube_token":         user.YoutubeToken,
		"youtube_refresh_token": user.YoutubeRefreshToken,
	}).Error; err != nil {
		return "", err
	}

	return res.AccessToken, nil
}

// InstagramUserResponse represents Instagram Graph API response
type InstagramUserResponse struct {
	ID             string `json:"id"`
	Username       string `json:"username"`
	Name           string `json:"name"`
	Biography      string `json:"biography"`
	FollowersCount int    `json:"followers_count"`
	FollowsCount   int    `json:"follows_count"`
	MediaCount     int    `json:"media_count"`
	ProfilePicture string `json:"profile_picture_url"`
	Error          *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (h *CreatorHandler) fetchInstagramStats(accessToken string) (map[string]interface{}, error) {
	// Instagram Graph API - fetch user info with business account fields
	url := fmt.Sprintf("https://graph.instagram.com/me?fields=id,username,name,biography,followers_count,follows_count,media_count,profile_picture_url&access_token=%s", accessToken)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var igResp InstagramUserResponse
	if err := json.Unmarshal(body, &igResp); err != nil {
		return nil, err
	}

	if igResp.Error != nil {
		return nil, fmt.Errorf("Instagram API error: %s", igResp.Error.Message)
	}

	return map[string]interface{}{
		"id":              igResp.ID,
		"username":        igResp.Username,
		"name":            igResp.Name,
		"biography":       igResp.Biography,
		"followers_count": igResp.FollowersCount,
		"follows_count":   igResp.FollowsCount,
		"media_count":     igResp.MediaCount,
		"profile_picture": igResp.ProfilePicture,
	}, nil
}

// GetCreatorMedia godoc
// @Summary Get creator media
// @Description Returns recent Instagram posts and/or YouTube videos for a creator with view counts.
// @Tags creators
// @Produce json
// @Param id path string true "Creator user ID"
// @Param platform query string false "Filter by platform: instagram | youtube (omit for both)"
// @Param limit query int false "Max items per platform (default 12)"
// @Success 200 {object} map[string]interface{} "instagram and/or youtube arrays of CreatorMediaItem"
// @Failure 404 {object} map[string]interface{} "Creator not found"
// @Security BearerAuth
// @Router /creators/{id}/media [get]
func (h *CreatorHandler) GetCreatorMedia(c *gin.Context) {
	creatorID := c.Param("id")
	platform := strings.ToLower(c.Query("platform")) // "instagram", "youtube", or ""
	limit := 12

	// Load creator's user record (tokens live on User)
	var user domain.User
	if err := h.db.Where("id = ?", creatorID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator not found"})
		return
	}

	response := gin.H{}

	if platform == "" || platform == "instagram" {
		if user.InstagramToken != "" {
			items, err := h.fetchInstagramMedia(user.InstagramToken, limit)
			if err != nil {
				response["instagram_error"] = err.Error()
			} else {
				response["instagram"] = items
			}
		} else {
			response["instagram"] = []CreatorMediaItem{}
		}
	}

	if platform == "" || platform == "youtube" {
		if user.YoutubeToken != "" {
			items, err := h.fetchYouTubeVideos(user.YoutubeToken, limit)
			if err != nil && isTokenError(err.Error()) && user.YoutubeRefreshToken != "" {
				if newToken, refreshErr := h.refreshYouTubeToken(&user); refreshErr == nil {
					items, err = h.fetchYouTubeVideos(newToken, limit)
				}
			}

			if err != nil {
				response["youtube_error"] = err.Error()
			} else {
				response["youtube"] = items
			}
		} else {
			response["youtube"] = []CreatorMediaItem{}
		}
	}

	c.JSON(http.StatusOK, response)
}

// ── Instagram ────────────────────────────────────────────────────────────────

type igMediaListResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Caption       string `json:"caption"`
		MediaType     string `json:"media_type"`
		MediaURL      string `json:"media_url"`
		ThumbnailURL  string `json:"thumbnail_url"`
		Permalink     string `json:"permalink"`
		Timestamp     string `json:"timestamp"`
		VideoViews    int64  `json:"video_views"`
		LikeCount     int64  `json:"like_count"`
		CommentsCount int64  `json:"comments_count"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type igInsightsResponse struct {
	Data []struct {
		Name   string `json:"name"`
		Period string `json:"period"`
		Values []struct {
			Value int64 `json:"value"`
		} `json:"values"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// fetchInstagramAccountInsights fetches 28-day reach, impressions, profile_views
func (h *CreatorHandler) fetchInstagramAccountInsights(accessToken, userID string) (reach, impressions, profileViews int64, err error) {
	url := fmt.Sprintf(
		"https://graph.instagram.com/%s/insights?metric=reach,impressions,profile_views&period=days_28&access_token=%s",
		userID, accessToken,
	)
	resp, e := http.Get(url) //nolint:noctx
	if e != nil {
		return 0, 0, 0, e
	}
	defer resp.Body.Close()
	body, e := io.ReadAll(resp.Body)
	if e != nil {
		return 0, 0, 0, e
	}
	var ins igInsightsResponse
	if e := json.Unmarshal(body, &ins); e != nil {
		return 0, 0, 0, e
	}
	if ins.Error != nil {
		return 0, 0, 0, fmt.Errorf("Instagram insights error: %s", ins.Error.Message)
	}
	for _, d := range ins.Data {
		if len(d.Values) == 0 {
			continue
		}
		v := d.Values[0].Value
		switch d.Name {
		case "reach":
			reach = v
		case "impressions":
			impressions = v
		case "profile_views":
			profileViews = v
		}
	}
	return
}

// fetchInstagramPostInsights fetches per-post reach, shares, saved for a single media item.
// Returns (reach, shares, saves, ok). ok=false if the API call fails (e.g. not a business account).
func (h *CreatorHandler) fetchInstagramPostInsights(accessToken, mediaID string) (reach, shares, saves int64, ok bool) {
	url := fmt.Sprintf(
		"https://graph.instagram.com/%s/insights?metric=reach,shares,saved&access_token=%s",
		mediaID, accessToken,
	)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	var ins igInsightsResponse
	if err := json.Unmarshal(body, &ins); err != nil {
		return
	}
	if ins.Error != nil {
		return
	}
	for _, d := range ins.Data {
		if len(d.Values) == 0 {
			continue
		}
		v := d.Values[0].Value
		switch d.Name {
		case "reach":
			reach = v
		case "shares":
			shares = v
		case "saved":
			saves = v
		}
	}
	ok = true
	return
}

func (h *CreatorHandler) fetchInstagramMedia(accessToken string, limit int) ([]CreatorMediaItem, error) {
	fields := "id,caption,media_type,media_url,thumbnail_url,permalink,timestamp,video_views,like_count,comments_count"
	url := fmt.Sprintf(
		"https://graph.instagram.com/me/media?fields=%s&limit=%d&access_token=%s",
		fields, limit, accessToken,
	)

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to reach Instagram API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var igResp igMediaListResponse
	if err := json.Unmarshal(body, &igResp); err != nil {
		return nil, err
	}
	if igResp.Error != nil {
		return nil, fmt.Errorf("Instagram API error: %s", igResp.Error.Message)
	}

	items := make([]CreatorMediaItem, 0, len(igResp.Data))
	for _, m := range igResp.Data {
		// For images/carousels, view_count is 0 — that's expected
		thumbURL := m.ThumbnailURL
		if thumbURL == "" && m.MediaType == "IMAGE" {
			thumbURL = m.MediaURL
		}
		// First line of caption as title
		title := m.Caption
		if idx := strings.IndexByte(m.Caption, '\n'); idx != -1 {
			title = m.Caption[:idx]
		}
		if len(title) > 100 {
			title = title[:100]
		}
		items = append(items, CreatorMediaItem{
			Platform:     "instagram",
			ID:           m.ID,
			Title:        title,
			Caption:      m.Caption,
			ThumbnailURL: thumbURL,
			MediaURL:     m.MediaURL,
			Permalink:    m.Permalink,
			MediaType:    m.MediaType,
			ViewCount:    m.VideoViews,
			LikeCount:    m.LikeCount,
			CommentCount: m.CommentsCount,
			PublishedAt:  m.Timestamp,
		})
	}
	return items, nil
}

// InstagramReel represents a single reel enriched with insight metrics.
type InstagramReel struct {
	ID                string `json:"id"`
	Caption           string `json:"caption"`
	Permalink         string `json:"permalink"`
	ThumbnailURL      string `json:"thumbnail_url"`
	MediaURL          string `json:"media_url"`
	Timestamp         string `json:"timestamp"`
	Views             int64  `json:"views"`
	Likes             int64  `json:"likes"`
	Comments          int64  `json:"comments"`
	Reach             int64  `json:"reach"`
	Shares            int64  `json:"shares"`
	Saves             int64  `json:"saves"`
	TotalInteractions int64  `json:"total_interactions"`
}

type igReelsMediaResponse struct {
	Data []struct {
		ID               string `json:"id"`
		Caption          string `json:"caption"`
		MediaType        string `json:"media_type"`
		MediaProductType string `json:"media_product_type"`
		MediaURL         string `json:"media_url"`
		ThumbnailURL     string `json:"thumbnail_url"`
		Permalink        string `json:"permalink"`
		Timestamp        string `json:"timestamp"`
		VideoViews       int64  `json:"video_views"`
		LikeCount        int64  `json:"like_count"`
		CommentsCount    int64  `json:"comments_count"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// fetchInstagramReels fetches reels (media_product_type=REELS) published within the
// last `sinceDays` days, up to `maxFetch` items. Each reel is enriched with insight
// metrics via fetchInstagramReelInsights when permission allows.
func (h *CreatorHandler) fetchInstagramReels(accessToken string, sinceDays, maxFetch int) ([]InstagramReel, error) {
	fields := "id,caption,media_type,media_product_type,media_url,thumbnail_url,permalink,timestamp,video_views,like_count,comments_count"
	url := fmt.Sprintf(
		"https://graph.instagram.com/me/media?fields=%s&limit=50&access_token=%s",
		fields, accessToken,
	)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to reach Instagram API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var igResp igReelsMediaResponse
	if err := json.Unmarshal(body, &igResp); err != nil {
		return nil, err
	}
	if igResp.Error != nil {
		return nil, fmt.Errorf("Instagram API error: %s", igResp.Error.Message)
	}

	cutoff := time.Now().AddDate(0, 0, -sinceDays)
	reels := make([]InstagramReel, 0)
	for _, m := range igResp.Data {
		// media_product_type=REELS is the canonical Graph-API marker. On Basic
		// Display tokens (or older scopes) the field comes back empty — fall back
		// to MediaType=VIDEO so reels still get picked up.
		isReel := m.MediaProductType == "REELS" ||
			(m.MediaProductType == "" && m.MediaType == "VIDEO")
		if !isReel {
			continue
		}
		ts, err := time.Parse(time.RFC3339, m.Timestamp)
		if err == nil && ts.Before(cutoff) {
			continue
		}
		thumb := m.ThumbnailURL
		if thumb == "" {
			thumb = m.MediaURL
		}
		reels = append(reels, InstagramReel{
			ID:           m.ID,
			Caption:      m.Caption,
			Permalink:    m.Permalink,
			ThumbnailURL: thumb,
			MediaURL:     m.MediaURL,
			Timestamp:    m.Timestamp,
			Views:        m.VideoViews,
			Likes:        m.LikeCount,
			Comments:     m.CommentsCount,
		})
		if len(reels) >= maxFetch {
			break
		}
	}

	for i := range reels {
		if ins, ok := h.fetchInstagramReelInsights(accessToken, reels[i].ID); ok {
			if reels[i].Views == 0 {
				reels[i].Views = ins["plays"]
			}
			reels[i].Reach = ins["reach"]
			reels[i].Shares = ins["shares"]
			reels[i].Saves = ins["saved"]
			reels[i].TotalInteractions = ins["total_interactions"]
		}
	}
	return reels, nil
}

// fetchInstagramReelInsights returns metric -> value for a single reel.
// Returns ok=false if the call failed (e.g. token lacks instagram_manage_insights
// or the account is not a Business/Creator account).
func (h *CreatorHandler) fetchInstagramReelInsights(accessToken, mediaID string) (map[string]int64, bool) {
	url := fmt.Sprintf(
		"https://graph.instagram.com/%s/insights?metric=plays,reach,likes,comments,shares,saved,total_interactions&access_token=%s",
		mediaID, accessToken,
	)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	var ins igInsightsResponse
	if err := json.Unmarshal(body, &ins); err != nil {
		return nil, false
	}
	if ins.Error != nil {
		return nil, false
	}
	out := make(map[string]int64, len(ins.Data))
	for _, d := range ins.Data {
		if len(d.Values) == 0 {
			continue
		}
		out[d.Name] = d.Values[0].Value
	}
	return out, true
}

// computeInstagramReelAggregates returns {avg_views, avg_likes, avg_comments,
// avg_reach, avg_engagement, total_views, total_likes, total_reach, count,
// engagement_rate} for a slice of reels. engagement_rate is (avg_likes+avg_comments+avg_shares+avg_saves)/followers*100.
func computeInstagramReelAggregates(reels []InstagramReel, followers int64) map[string]interface{} {
	out := map[string]interface{}{"count": len(reels)}
	if len(reels) == 0 {
		return out
	}
	var totalViews, totalLikes, totalComments, totalReach, totalShares, totalSaves int64
	for _, r := range reels {
		totalViews += r.Views
		totalLikes += r.Likes
		totalComments += r.Comments
		totalReach += r.Reach
		totalShares += r.Shares
		totalSaves += r.Saves
	}
	n := int64(len(reels))
	avgLikes := totalLikes / n
	avgComments := totalComments / n
	avgShares := totalShares / n
	avgSaves := totalSaves / n
	out["avg_views"] = totalViews / n
	out["avg_likes"] = avgLikes
	out["avg_comments"] = avgComments
	out["avg_reach"] = totalReach / n
	out["avg_shares"] = avgShares
	out["avg_saves"] = avgSaves
	out["total_views"] = totalViews
	out["total_likes"] = totalLikes
	out["total_reach"] = totalReach
	if followers > 0 {
		engRate := float64(avgLikes+avgComments+avgShares+avgSaves) / float64(followers) * 100
		out["engagement_rate"] = fmt.Sprintf("%.2f", engRate)
	}
	return out
}

// generateInstagramAIAnalysis runs Gemini over the IG profile stats + recent reels
// to produce a brand-suitability JSON, mirroring the YouTube AI analysis schema.
func (h *CreatorHandler) generateInstagramAIAnalysis(igStats map[string]interface{}, recentReels []InstagramReel) (map[string]interface{}, error) {
	if h.geminiAPIKey == "" {
		return nil, fmt.Errorf("gemini api key is empty")
	}

	reelsSummary := make([]string, 0, len(recentReels))
	for _, r := range recentReels {
		caption := r.Caption
		if len(caption) > 200 {
			caption = caption[:200]
		}
		reelsSummary = append(reelsSummary, fmt.Sprintf("- Caption: %s | Views: %d | Likes: %d | Comments: %d | Reach: %d", caption, r.Views, r.Likes, r.Comments, r.Reach))
	}

	prompt := fmt.Sprintf(`You are an expert AI brand safety and content intelligence analyzer.
Analyze the following Instagram creator's profile and recent reels to produce high-value suitability insights for brands.

Profile Statistics:
%+v

Recent Reels (last 30 days):
%s

You MUST return a JSON object with the following schema:
{
  "channel_niche": "Primary content focus, e.g. Fashion & Lifestyle",
  "content_style": "High-level creative style, e.g. Aesthetic Vlog / Comedy Skits",
  "estimated_reach_score": 85,
  "estimated_reach_description": "Explanation of reach consistency, view-to-follower ratio, and growth potential",
  "audience_interests": ["Fashion", "Travel", "Beauty"],
  "brand_safety_rating": "Safe",
  "brand_safety_reasons": "Detailed explanation of any risks or a confirmation of safe content",
  "recommended_campaign_categories": ["DTC Apparel", "Beauty Brands", "Travel Apps"],
  "key_insights_for_brands": [
    "Strong reel-to-follower view ratio (>30%%)",
    "Consistent posting cadence",
    "Highly engaged Gen-Z audience"
  ]
}

brand_safety_rating must be exactly one of: "Safe", "Moderate", "Caution".
audience_interests, recommended_campaign_categories, key_insights_for_brands must each have at most 4 items.
Return ONLY the raw JSON object. Do not include markdown wraps or anything else.`, igStats, strings.Join(reelsSummary, "\n"))

	type localPart struct {
		Text string `json:"text"`
	}
	type localContent struct {
		Parts []localPart `json:"parts"`
	}
	type localReqConfig struct {
		ResponseMimeType string `json:"responseMimeType"`
	}
	type localRequest struct {
		Contents         []localContent `json:"contents"`
		GenerationConfig localReqConfig `json:"generationConfig"`
	}

	reqPayload := localRequest{
		Contents: []localContent{{Parts: []localPart{{Text: prompt}}}},
		GenerationConfig: localReqConfig{ResponseMimeType: "application/json"},
	}
	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=%s", h.geminiAPIKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini API call failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read Gemini response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API non-200: %s", string(respBody))
	}

	var gResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini wrapper: %w", err)
	}
	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("Gemini response had no candidates")
	}
	raw := gResp.Candidates[0].Content.Parts[0].Text

	var analysis map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse Gemini AI JSON: %w (body=%s)", err, raw)
	}
	return analysis, nil
}

// ── YouTube ──────────────────────────────────────────────────────────────────

type ytChannelContentResponse struct {
	Items []struct {
		ContentDetails struct {
			RelatedPlaylists struct {
				Uploads string `json:"uploads"`
			} `json:"relatedPlaylists"`
		} `json:"contentDetails"`
	} `json:"items"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type ytPlaylistItemsResponse struct {
	Items []struct {
		Snippet struct {
			PublishedAt string `json:"publishedAt"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Thumbnails  struct {
				High struct {
					URL string `json:"url"`
				} `json:"high"`
				Default struct {
					URL string `json:"url"`
				} `json:"default"`
			} `json:"thumbnails"`
			ResourceID struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	} `json:"items"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type ytVideosResponse struct {
	Items []struct {
		ID         string `json:"id"`
		Statistics struct {
			ViewCount    string `json:"viewCount"`
			LikeCount    string `json:"likeCount"`
			CommentCount string `json:"commentCount"`
		} `json:"statistics"`
		ContentDetails struct {
			Duration string `json:"duration"` // ISO 8601: PT4M23S
		} `json:"contentDetails"`
	} `json:"items"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (h *CreatorHandler) fetchYouTubeVideos(accessToken string, limit int) ([]CreatorMediaItem, error) {
	doGet := func(url string) ([]byte, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := (&http.Client{}).Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	// Step 1: get uploads playlist ID
	body, err := doGet("https://www.googleapis.com/youtube/v3/channels?part=contentDetails&mine=true")
	if err != nil {
		return nil, fmt.Errorf("YouTube channels API failed: %w", err)
	}
	var chResp ytChannelContentResponse
	if err := json.Unmarshal(body, &chResp); err != nil {
		return nil, err
	}
	if chResp.Error != nil {
		return nil, fmt.Errorf("YouTube API error: %s", chResp.Error.Message)
	}
	if len(chResp.Items) == 0 {
		return nil, fmt.Errorf("no YouTube channel found")
	}
	uploadsPlaylistID := chResp.Items[0].ContentDetails.RelatedPlaylists.Uploads
	if uploadsPlaylistID == "" {
		return nil, fmt.Errorf("YouTube uploads playlist not found")
	}

	// Step 2: list recent uploads
	plURL := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/playlistItems?part=snippet&playlistId=%s&maxResults=%d",
		uploadsPlaylistID, limit,
	)
	body, err = doGet(plURL)
	if err != nil {
		return nil, fmt.Errorf("YouTube playlistItems API failed: %w", err)
	}
	var plResp ytPlaylistItemsResponse
	if err := json.Unmarshal(body, &plResp); err != nil {
		return nil, err
	}
	if plResp.Error != nil {
		return nil, fmt.Errorf("YouTube API error: %s", plResp.Error.Message)
	}
	if len(plResp.Items) == 0 {
		return []CreatorMediaItem{}, nil
	}

	// Collect video IDs and build a lookup map for snippet data
	type snippetData struct {
		title       string
		description string
		thumbnail   string
		publishedAt string
	}
	snippetMap := make(map[string]snippetData, len(plResp.Items))
	videoIDs := make([]string, 0, len(plResp.Items))
	for _, item := range plResp.Items {
		vid := item.Snippet.ResourceID.VideoID
		if vid == "" {
			continue
		}
		videoIDs = append(videoIDs, vid)
		thumb := item.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Default.URL
		}
		snippetMap[vid] = snippetData{
			title:       item.Snippet.Title,
			description: item.Snippet.Description,
			thumbnail:   thumb,
			publishedAt: item.Snippet.PublishedAt,
		}
	}

	// Step 3: fetch statistics + content details for all videos in one request
	vidStatsURL := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/videos?part=statistics,contentDetails&id=%s",
		strings.Join(videoIDs, ","),
	)
	body, err = doGet(vidStatsURL)
	if err != nil {
		return nil, fmt.Errorf("YouTube videos API failed: %w", err)
	}
	var vidResp ytVideosResponse
	if err := json.Unmarshal(body, &vidResp); err != nil {
		return nil, err
	}
	if vidResp.Error != nil {
		return nil, fmt.Errorf("YouTube API error: %s", vidResp.Error.Message)
	}

	// index statistics by video ID
	type videoStats struct {
		viewCount    string
		likeCount    string
		commentCount string
		duration     string // ISO 8601
	}
	statsByID := make(map[string]videoStats, len(vidResp.Items))
	for _, v := range vidResp.Items {
		statsByID[v.ID] = videoStats{
			viewCount:    v.Statistics.ViewCount,
			likeCount:    v.Statistics.LikeCount,
			commentCount: v.Statistics.CommentCount,
			duration:     v.ContentDetails.Duration,
		}
	}

	// Build response in upload order
	items := make([]CreatorMediaItem, 0, len(videoIDs))
	for _, vid := range videoIDs {
		sn := snippetMap[vid]
		st := statsByID[vid]
		items = append(items, CreatorMediaItem{
			Platform:     "youtube",
			ID:           vid,
			Title:        sn.title,
			Caption:      sn.description,
			ThumbnailURL: sn.thumbnail,
			MediaURL:     "https://www.youtube.com/watch?v=" + vid,
			Permalink:    "https://www.youtube.com/watch?v=" + vid,
			MediaType:    "YOUTUBE",
			ViewCount:    parseCount(st.viewCount),
			LikeCount:    parseCount(st.likeCount),
			CommentCount: parseCount(st.commentCount),
			Duration:     parseDuration(st.duration),
			PublishedAt:  sn.publishedAt,
		})
	}
	return items, nil
}

// parseCount converts a string count from the YouTube API to int64.
func parseCount(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// parseDuration converts an ISO 8601 duration string to seconds. e.g. "PT4M23S" → 263
func parseDuration(iso string) int64 {
	iso = strings.TrimPrefix(iso, "PT")
	var total int64
	if i := strings.Index(iso, "H"); i != -1 {
		h, _ := strconv.ParseInt(iso[:i], 10, 64)
		total += h * 3600
		iso = iso[i+1:]
	}
	if i := strings.Index(iso, "M"); i != -1 {
		m, _ := strconv.ParseInt(iso[:i], 10, 64)
		total += m * 60
		iso = iso[i+1:]
	}
	if i := strings.Index(iso, "S"); i != -1 {
		s, _ := strconv.ParseInt(iso[:i], 10, 64)
		total += s
	}
	return total
}

// channelAgeYears returns the channel age in years (1 decimal) from an RFC3339 publishedAt string.
func channelAgeYears(publishedAt string) float64 {
	t, err := time.Parse(time.RFC3339, publishedAt)
	if err != nil {
		return 0
	}
	years := time.Since(t).Hours() / 8760
	return math.Round(years*10) / 10
}

// ── YouTube Analytics API ─────────────────────────────────────────────────────

type ytAnalyticsResponse struct {
	ColumnHeaders []struct {
		Name string `json:"name"`
	} `json:"columnHeaders"`
	Rows  [][]interface{} `json:"rows"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// fetchYouTubeAnalytics calls the YouTube Analytics API for a rolling window of `days`.
// Requires the yt-analytics.readonly OAuth scope. Gracefully returns an error if the
// token lacks that scope so callers can omit the data rather than fail.
func (h *CreatorHandler) fetchYouTubeAnalytics(accessToken string, days int) (map[string]interface{}, error) {
	endDate := time.Now().Format("2006-01-02")
	startDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	metrics := strings.Join([]string{
		"views",
		"estimatedMinutesWatched",
		"averageViewDuration",
		"averageViewPercentage",
		"subscribersGained",
		"subscribersLost",
		"likes",
		"shares",
		"comments",
		"impressions",
		"impressionClickThroughRate",
	}, ",")

	apiURL := fmt.Sprintf(
		"https://youtubeanalytics.googleapis.com/v2/reports?ids=channel==MINE&startDate=%s&endDate=%s&metrics=%s",
		startDate, endDate, metrics,
	)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ytResp ytAnalyticsResponse
	if err := json.Unmarshal(body, &ytResp); err != nil {
		return nil, err
	}
	if ytResp.Error != nil {
		return nil, fmt.Errorf("YouTube Analytics API error (%d): %s", ytResp.Error.Code, ytResp.Error.Message)
	}
	if len(ytResp.Rows) == 0 || len(ytResp.ColumnHeaders) == 0 {
		return nil, fmt.Errorf("no analytics data")
	}

	result := make(map[string]interface{}, len(ytResp.ColumnHeaders))
	row := ytResp.Rows[0]
	for i, h := range ytResp.ColumnHeaders {
		if i < len(row) {
			result[h.Name] = row[i]
		}
	}
	return result, nil
}

func (h *CreatorHandler) generateYouTubeAIAnalysis(ytStats map[string]interface{}, recentVideos []CreatorMediaItem) (map[string]interface{}, error) {
	if h.geminiAPIKey == "" {
		return nil, fmt.Errorf("gemini api key is empty")
	}

	// Prepare data to send to Gemini
	videosSummary := make([]string, 0, len(recentVideos))
	for _, v := range recentVideos {
		videosSummary = append(videosSummary, fmt.Sprintf("- Title: %s | Description/Caption: %s | Views: %d", v.Title, v.Caption, v.ViewCount))
	}

	prompt := fmt.Sprintf(`You are an expert AI brand safety and channel intelligence analyzer.
Analyze the following YouTube creator's channel statistics and recent video catalog to produce high-value suitability insights for brands.

Channel Statistics:
%+v

Recent Videos:
%s

You MUST return a JSON object with the following schema:
{
  "channel_niche": "Primary content focus, e.g. Tech & Lifestyle",
  "content_style": "High-level creative style, e.g. Informative/Vlog-style",
  "estimated_reach_score": 85, // Integer 1-100 representing the consistency of engagement and reach
  "estimated_reach_description": "Explanation of reach consistency, views, and growth potential",
  "audience_interests": ["Tech Gadgets", "Productivity", "Travel"], // Max 4 strings
  "brand_safety_rating": "Safe", // Must be exactly one of: "Safe", "Moderate", "Caution"
  "brand_safety_reasons": "Detailed explanation of any risks, language use, controversies, or a confirmation of safe content",
  "recommended_campaign_categories": ["SaaS Apps", "Tech Accessories", "E-learning Platforms"], // Max 4 strings
  "key_insights_for_brands": [
    "High view retention on video tutorials",
    "Strong tech-savvy demographic alignment",
    "Polished, professional brand integration potential"
  ] // Bulleted benefits, max 4 items
}

Return ONLY the raw JSON object. Do not include markdown wraps or anything else.`, ytStats, strings.Join(videosSummary, "\n"))

	// We define Part here because we're inside Delivery/Http, but actually Part/Content are already imported if they are in package,
	// wait, are Part/Content already in pkg/embeddings/gemini.go? Yes, but here we can define them locally or use generic/inline structs.
	// Let's define them with unique local names or inline to avoid conflicts.
	type localPart struct {
		Text string `json:"text"`
	}
	type localContent struct {
		Parts []localPart `json:"parts"`
	}
	type localReqConfig struct {
		ResponseMimeType string `json:"responseMimeType"`
	}
	type localRequest struct {
		Contents         []localContent `json:"contents"`
		GenerationConfig localReqConfig `json:"generationConfig"`
	}

	reqPayload := localRequest{
		Contents: []localContent{
			{
				Parts: []localPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: localReqConfig{
			ResponseMimeType: "application/json",
		},
	}

	jsonBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Gemini request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=%s", h.geminiAPIKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	responseText := geminiResp.Candidates[0].Content.Parts[0].Text

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI analysis json: %w (raw response: %s)", err, responseText)
	}

	return result, nil
}

