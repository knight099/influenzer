package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

// CreatorMediaItem is a unified media item returned for both Instagram and YouTube.
type CreatorMediaItem struct {
	Platform    string `json:"platform"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Caption     string `json:"caption"`
	ThumbnailURL string `json:"thumbnail_url"`
	MediaURL    string `json:"media_url"`
	Permalink   string `json:"permalink"`
	MediaType   string `json:"media_type"` // VIDEO, IMAGE, CAROUSEL_ALBUM, YOUTUBE
	ViewCount   int64  `json:"view_count"`
	LikeCount   int64  `json:"like_count"`
	PublishedAt string `json:"published_at"`
}

type CreatorHandler struct {
	db *gorm.DB
}

func NewCreatorHandler(r *gin.Engine, db *gorm.DB, authMiddleware gin.HandlerFunc) {
	handler := &CreatorHandler{db: db}

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
	}
}

func (h *CreatorHandler) Search(c *gin.Context) {
	queryStr := c.Query("query")
	niche := c.Query("niche")
	platform := c.Query("platform")

	var creators []domain.CreatorProfile

	query := h.db.Preload("User").Model(&domain.CreatorProfile{})

	// Search by name in User table if query string provided
	if queryStr != "" {
		query = query.Joins("JOIN users ON users.id = creator_profiles.user_id").
			Where("users.name ILIKE ? OR creator_profiles.niche ILIKE ?", "%"+queryStr+"%", "%"+queryStr+"%")
	}

	if niche != "" {
		query = query.Where("niche ILIKE ?", "%"+niche+"%")
	}

	if platform != "" {
		query = query.Where("platform = ?", platform)
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
	if err := h.db.Where("user_id = ?", id).First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Creator not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          profile.UserID,
		"niche":       profile.Niche,
		"min_budget":  profile.MinBudget,
		"city":        profile.City,
		"platform":    profile.Platform,
		"portfolio":   profile.Portfolio,
		"cached_stats": profile.CachedStats,
		"reviews":     []string{},
	})
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
		response["niche"] = user.CreatorProfile.Niche
		response["min_budget"] = user.CreatorProfile.MinBudget
		response["city"] = user.CreatorProfile.City
		response["platform"] = user.CreatorProfile.Platform
		response["cached_stats"] = user.CreatorProfile.CachedStats
		response["portfolio"] = user.CreatorProfile.Portfolio
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
		if err != nil {
			// Log but don't fail - token might be expired
			fmt.Printf("Failed to fetch YouTube stats: %v\n", err)
			stats["youtube_error"] = err.Error()
		} else {
			stats["youtube"] = ytStats
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
	}, nil
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
		ID           string `json:"id"`
		Caption      string `json:"caption"`
		MediaType    string `json:"media_type"`
		MediaURL     string `json:"media_url"`
		ThumbnailURL string `json:"thumbnail_url"`
		Permalink    string `json:"permalink"`
		Timestamp    string `json:"timestamp"`
		VideoViews   int64  `json:"video_views"`
		LikeCount    int64  `json:"like_count"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (h *CreatorHandler) fetchInstagramMedia(accessToken string, limit int) ([]CreatorMediaItem, error) {
	fields := "id,caption,media_type,media_url,thumbnail_url,permalink,timestamp,video_views,like_count"
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
			PublishedAt:  m.Timestamp,
		})
	}
	return items, nil
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
		ID      string `json:"id"`
		Statistics struct {
			ViewCount string `json:"viewCount"`
			LikeCount string `json:"likeCount"`
		} `json:"statistics"`
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

	// Step 3: fetch statistics for all videos in one request
	vidStatsURL := fmt.Sprintf(
		"https://www.googleapis.com/youtube/v3/videos?part=statistics&id=%s",
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
		viewCount string
		likeCount string
	}
	statsByID := make(map[string]videoStats, len(vidResp.Items))
	for _, v := range vidResp.Items {
		statsByID[v.ID] = videoStats{
			viewCount: v.Statistics.ViewCount,
			likeCount: v.Statistics.LikeCount,
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
