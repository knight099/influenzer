package service

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"github.com/vaibhaw/influenzer-backend/pkg/embeddings"
	"gorm.io/gorm"
)

type MatchingService struct {
	db           *gorm.DB
	geminiClient *embeddings.Client
}

type MatchResult struct {
	CreatorID          uuid.UUID `json:"creator_id"`
	Name               string    `json:"name"`
	AvatarURL          string    `json:"avatar_url"`
	Headline           string    `json:"headline"`
	Niche              string    `json:"niche"`
	MatchScore         float32   `json:"match_score"`
	Platform           string    `json:"platform"`
	City               string    `json:"city"`
	MinBudget          float64   `json:"min_budget"`
	InstagramFollowers int64     `json:"instagram_followers"`
	EngagementRate     float32   `json:"engagement_rate"`
	MatchReasons       []string  `json:"match_reasons"`
}

func NewMatchingService(db *gorm.DB, apiKey string) *MatchingService {
	return &MatchingService{
		db:           db,
		geminiClient: embeddings.NewClient(apiKey),
	}
}

// BuildCreatorProfileText constructs a rich natural language profile from all available creator metadata
func (s *MatchingService) BuildCreatorProfileText(profile *domain.CreatorProfile, userName string) string {
	var doc strings.Builder

	doc.WriteString(fmt.Sprintf("Creator Name: %s\n", userName))
	if profile.Headline != "" {
		doc.WriteString(fmt.Sprintf("Headline: %s\n", profile.Headline))
	}
	if profile.Bio != "" {
		doc.WriteString(fmt.Sprintf("Bio: %s\n", profile.Bio))
	}
	if profile.Niche != "" {
		doc.WriteString(fmt.Sprintf("Niches and Categories: %s\n", profile.Niche))
	}
	if profile.ContentCategories != "" {
		doc.WriteString(fmt.Sprintf("Content Style Categories: %s\n", profile.ContentCategories))
	}
	if profile.Platform != "" {
		doc.WriteString(fmt.Sprintf("Primary Social Media Platforms: %s\n", profile.Platform))
	}
	if profile.City != "" || profile.Location != "" {
		location := profile.City
		if location == "" {
			location = profile.Location
		}
		doc.WriteString(fmt.Sprintf("Location: %s\n", location))
	}
	if profile.Languages != "" {
		doc.WriteString(fmt.Sprintf("Languages Spoken: %s\n", profile.Languages))
	}
	if profile.Gender != "" {
		doc.WriteString(fmt.Sprintf("Gender: %s\n", profile.Gender))
	}
	if profile.YearsExperience > 0 {
		doc.WriteString(fmt.Sprintf("Experience: %d years\n", profile.YearsExperience))
	}
	if profile.AvailabilityStatus != "" {
		doc.WriteString(fmt.Sprintf("Availability Status: %s\n", profile.AvailabilityStatus))
	}
	if profile.TurnaroundDays > 0 {
		doc.WriteString(fmt.Sprintf("Content Delivery Turnaround: %d days\n", profile.TurnaroundDays))
	}
	if profile.WillingToTravel {
		doc.WriteString("Willing to travel for campaigns: Yes\n")
	}

	// Audience demographics
	if len(profile.AudienceDemographics) > 0 {
		doc.WriteString("Audience Demographics: ")
		if age, ok := profile.AudienceDemographics["age_split"]; ok {
			doc.WriteString(fmt.Sprintf("Age distribution: %v. ", age))
		}
		if gender, ok := profile.AudienceDemographics["gender_split"]; ok {
			doc.WriteString(fmt.Sprintf("Gender distribution: %v. ", gender))
		}
		if cities, ok := profile.AudienceDemographics["top_cities"]; ok {
			doc.WriteString(fmt.Sprintf("Top audience cities: %v. ", cities))
		}
		if countries, ok := profile.AudienceDemographics["top_countries"]; ok {
			doc.WriteString(fmt.Sprintf("Top audience countries: %v. ", countries))
		}
		doc.WriteString("\n")
	}

	// Rates
	if profile.MinBudget > 0 {
		doc.WriteString(fmt.Sprintf("Minimum Campaign Budget Requirement: INR %f\n", profile.MinBudget))
	}
	if len(profile.RateCard) > 0 {
		doc.WriteString("Pricing Rate Card: ")
		for k, v := range profile.RateCard {
			doc.WriteString(fmt.Sprintf("%s: INR %v, ", k, v))
		}
		doc.WriteString("\n")
	}

	// Past Brands and Work
	if profile.PastBrands != "" {
		doc.WriteString(fmt.Sprintf("Collaborations with Past Brands: %s\n", profile.PastBrands))
	}
	if len(profile.PastWork) > 0 {
		doc.WriteString("Featured Past Work: ")
		for _, work := range profile.PastWork {
			if brand, ok := work["brand_name"]; ok {
				doc.WriteString(fmt.Sprintf("%v (Platform: %v, Deliverable: %v), ", brand, work["platform"], work["deliverable_type"]))
			}
		}
		doc.WriteString("\n")
	}

	return doc.String()
}

// UpdateCreatorEmbedding builds the creator profile text document, fetches its Gemini vector embedding, and stores it in the database
func (s *MatchingService) UpdateCreatorEmbedding(userID uuid.UUID) error {
	var user domain.User
	if err := s.db.Preload("CreatorProfile").First(&user, "id = ?", userID).Error; err != nil {
		return fmt.Errorf("failed to fetch user and profile: %w", err)
	}

	if user.CreatorProfile == nil {
		return fmt.Errorf("user is not registered as a creator")
	}

	profileText := s.BuildCreatorProfileText(user.CreatorProfile, user.Name)

	embeddingValues, err := s.geminiClient.GetEmbedding(profileText)
	if err != nil {
		return fmt.Errorf("failed to generate gemini embedding: %w", err)
	}

	now := time.Now()
	err = s.db.Model(user.CreatorProfile).Updates(map[string]interface{}{
		"embedding":              pgvector.NewVector(embeddingValues),
		"embedding_last_updated": &now,
	}).Error

	if err != nil {
		return fmt.Errorf("failed to save embedding to database: %w", err)
	}

	log.Printf("Successfully updated embedding for creator %s (%s)", user.Name, userID)
	return nil
}

// BulkUpdateEmbeddings runs embedding generation for all creator profiles that have outdated or missing embeddings
func (s *MatchingService) BulkUpdateEmbeddings() (int, error) {
	var profiles []domain.CreatorProfile
	// Find all profiles where embedding is NULL or embedding_last_updated is older than profile's updated_at
	err := s.db.Where("embedding IS NULL OR embedding_last_updated IS NULL OR embedding_last_updated < updated_at").Find(&profiles).Error
	if err != nil {
		return 0, fmt.Errorf("failed to query outstanding profiles: %w", err)
	}

	successCount := 0
	for _, profile := range profiles {
		if err := s.UpdateCreatorEmbedding(profile.UserID); err != nil {
			log.Printf("Error updating embedding for creator %s: %v", profile.UserID, err)
			continue
		}
		successCount++
		// Add small delay to stay well within Gemini API free-tier rate limits
		time.Sleep(100 * time.Millisecond)
	}

	return successCount, nil
}

// FindMatchingCreators searches for the best fit creators based on query semantic similarity and standard filtering
func (s *MatchingService) FindMatchingCreators(query string, platform string, minBudget float64, limit int) ([]MatchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// 1. Generate query embedding
	queryEmbedding, err := s.geminiClient.GetEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate search embedding: %w", err)
	}

	// 2. Query Neon pgvector using cosine distance
	type DBResult struct {
		UserID             uuid.UUID `gorm:"column:user_id"`
		Name               string    `gorm:"column:name"`
		AvatarURL          string    `gorm:"column:avatar_url"`
		Headline           string    `gorm:"column:headline"`
		Niche              string    `gorm:"column:niche"`
		Platform           string    `gorm:"column:platform"`
		City               string    `gorm:"column:city"`
		MinBudget          float64   `gorm:"column:min_budget"`
		CachedStats        []byte    `gorm:"column:cached_stats"`
		AvailabilityStatus string    `gorm:"column:availability_status"`
		MatchScore         float32   `gorm:"column:match_score"`
	}

	var rawResults []DBResult

	// Cosine distance <=> operator: smaller distance means higher similarity.
	// Cosine similarity is 1 - Cosine distance.
	sqlQuery := s.db.Table("creator_profiles cp").
		Select("cp.user_id, u.name, u.avatar_url, cp.headline, cp.niche, cp.platform, cp.city, cp.min_budget, cp.cached_stats, cp.availability_status, 1 - (cp.embedding <=> ?) AS match_score", pgvector.NewVector(queryEmbedding)).
		Joins("JOIN users u ON u.id = cp.user_id").
		Where("cp.embedding IS NOT NULL AND cp.availability_status = 'available'")

	// Apply hard filters
	if platform != "" {
		sqlQuery = sqlQuery.Where("cp.platform ILIKE ?", "%"+platform+"%")
	}
	if minBudget > 0 {
		sqlQuery = sqlQuery.Where("cp.min_budget <= ?", minBudget)
	}

	// Order by proximity (ascending cosine distance <=> descending match_score)
	err = sqlQuery.Order("match_score DESC").Limit(limit).Find(&rawResults).Error
	if err != nil {
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	results := make([]MatchResult, 0, len(rawResults))
	for _, item := range rawResults {
		var stats map[string]interface{}
		var followers int64 = 0
		var engagement float32 = 0.0

		// Parse CachedStats to fetch follower count/engagement dynamically
		if len(item.CachedStats) > 0 {
			if err := json.Unmarshal(item.CachedStats, &stats); err == nil {
				// Instagram Stats parser fallback
				if ig, ok := stats["instagram"].(map[string]interface{}); ok {
					if f, ok := ig["followers"].(float64); ok {
						followers = int64(f)
					}
					if e, ok := ig["engagement_rate"].(float64); ok {
						engagement = float32(e)
					}
				} else {
					// Fallback to direct followers key if flat structure
					if f, ok := stats["followers"].(float64); ok {
						followers = int64(f)
					}
					if e, ok := stats["engagement_rate"].(float64); ok {
						engagement = float32(e)
					}
				}
			}
		}

		reasons := s.generateMatchReasons(item.Niche, item.Platform, item.MinBudget, query)

		results = append(results, MatchResult{
			CreatorID:          item.UserID,
			Name:               item.Name,
			AvatarURL:          item.AvatarURL,
			Headline:           item.Headline,
			Niche:              item.Niche,
			MatchScore:         item.MatchScore,
			Platform:           item.Platform,
			City:               item.City,
			MinBudget:          item.MinBudget,
			InstagramFollowers: followers,
			EngagementRate:     engagement,
			MatchReasons:       reasons,
		})
	}

	return results, nil
}

// MatchCampaignToCreators matches creators against details extracted from a campaign profile
func (s *MatchingService) MatchCampaignToCreators(campaignID uuid.UUID, limit int) ([]MatchResult, error) {
	var campaign domain.Campaign
	if err := s.db.First(&campaign, "id = ?", campaignID).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch campaign: %w", err)
	}

	// Create campaign context query text
	query := fmt.Sprintf("Title: %s. Description: %s. Niche/Category: %s. Platform: %s. Required Budget: %f.",
		campaign.Title, campaign.Description, campaign.Niche, campaign.Platform, campaign.Budget)

	return s.FindMatchingCreators(query, campaign.Platform, campaign.Budget, limit)
}

// generateMatchReasons yields short human-readable explanations of why the creator was matched
func (s *MatchingService) generateMatchReasons(niche, platform string, rate float64, query string) []string {
	reasons := make([]string, 0)
	queryLower := strings.ToLower(query)

	// Reason 1: Niche overlap
	if niche != "" {
		niches := strings.Split(niche, ",")
		for _, n := range niches {
			nClean := strings.TrimSpace(strings.ToLower(n))
			if nClean != "" && strings.Contains(queryLower, nClean) {
				reasons = append(reasons, fmt.Sprintf("Matches niche target: '%s'", n))
				break
			}
		}
	}

	// Reason 2: Platform check
	if platform != "" {
		platforms := strings.Split(strings.ToLower(platform), ",")
		for _, p := range platforms {
			pClean := strings.TrimSpace(p)
			if pClean != "" && strings.Contains(queryLower, pClean) {
				reasons = append(reasons, fmt.Sprintf("Active on required platform: %s", strings.Title(pClean)))
				break
			}
		}
	}

	// Reason 3: Budget check
	if rate > 0 {
		reasons = append(reasons, fmt.Sprintf("Creator minimum budget (INR %v) is budget-friendly", rate))
	}

	// Reason 4: Default generic semantic similarity fallback
	if len(reasons) == 0 {
		reasons = append(reasons, "High semantic alignment with campaign description")
	}

	return reasons
}
