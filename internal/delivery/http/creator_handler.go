package http

import (
	"encoding/json"
	"fmt"
	"io"
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

	// Build connected platforms list
	connectedPlatforms := []map[string]interface{}{}
	if user.InstagramToken != "" {
		connectedPlatforms = append(connectedPlatforms, map[string]interface{}{
			"platform":  "instagram",
			"connected": true,
		})
	}
	if user.YoutubeToken != "" {
		connectedPlatforms = append(connectedPlatforms, map[string]interface{}{
			"platform":  "youtube",
			"connected": true,
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

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
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
