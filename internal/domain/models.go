package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

type Role string
type CampaignStatus string
type ProposalStatus string

const (
	RoleBrand   Role = "BRAND"
	RoleCreator Role = "CREATOR"

	CampaignStatusOpen   CampaignStatus = "OPEN"
	CampaignStatusClosed CampaignStatus = "CLOSED"

	ProposalStatusApplied   ProposalStatus = "APPLIED"
	ProposalStatusApproved  ProposalStatus = "APPROVED"
	ProposalStatusFunded    ProposalStatus = "FUNDED"
	ProposalStatusSubmitted ProposalStatus = "SUBMITTED"
	ProposalStatusCompleted ProposalStatus = "COMPLETED"
	ProposalStatusPaid      ProposalStatus = "PAID"
)

type User struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Email         string         `gorm:"uniqueIndex;not null" json:"email"`
	Name          string         `json:"name"`
	Password      string         `json:"-"` // Hashed password
	Role          Role           `gorm:"type:varchar(20);not null" json:"role"`
	GoogleID      string         `gorm:"uniqueIndex" json:"google_id"`
	AvatarURL     string         `json:"avatar_url"`
	EpisoddLinkID *string        `json:"episodd_link_id"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`

	// Tokens
	InstagramToken      string `json:"-"`
	YoutubeToken        string `json:"-"`
	YoutubeRefreshToken string `json:"-"`

	BrandProfile   *BrandProfile   `json:"brand_profile,omitempty"`
	CreatorProfile *CreatorProfile `json:"creator_profile,omitempty"`
}

type BrandProfile struct {
	UserID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	CompanyName   string    `json:"company_name"`
	ContactName   string    `json:"contact_name"`
	Phone         string    `json:"phone"`
	RoleInCompany string    `json:"role_in_company"`
	GSTNumber     string    `json:"gst_number"`
	Website       string    `json:"website"`
	LogoURL       string    `json:"logo_url"`
	// Extended profile
	Industry          string `json:"industry"`
	Description       string `gorm:"type:text" json:"description"`
	FoundedYear       int    `json:"founded_year"`
	CompanySize       string `json:"company_size"`
	Headquarters      string `json:"headquarters"`
	InstagramURL      string `json:"instagram_url"`
	TwitterURL        string `json:"twitter_url"`
	LinkedinURL       string `json:"linkedin_url"`
	ProductCategories string `gorm:"type:text" json:"product_categories"`
	TargetAudience    string `gorm:"type:text" json:"target_audience"`
	CampaignTypes     string `gorm:"type:text" json:"campaign_types"`
	WalletBalance float64   `gorm:"default:0" json:"wallet_balance"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreatorProfile struct {
	UserID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	Niche             string    `gorm:"type:text" json:"niche"` // Comma separated tags
	MinBudget         float64   `json:"min_budget"`
	City              string    `json:"city"`
	Phone             string    `json:"phone"`
	Platform          string    `json:"platform"` // e.g. "instagram", "youtube"
	RazorpayAccountID string    `json:"razorpay_account_id"`

	// ── Professional identity ────────────────────────────────────────────────
	Headline        string     `gorm:"type:text" json:"headline"`                                    // "Tech & Lifestyle Creator | 500K+ reach"
	Gender          string     `gorm:"type:varchar(20)" json:"gender"`                               // male, female, non_binary, prefer_not_to_say
	DateOfBirth     *time.Time `json:"date_of_birth"`                                                // For age calculation
	ProfileComplete int        `gorm:"default:0" json:"profile_complete"`                            // 0-100 percentage

	// ── Extended profile fields ──────────────────────────────────────────────
	Bio               string                 `gorm:"type:text" json:"bio"`
	Languages         string                 `json:"languages"`                            // Comma separated e.g. "Hindi,English"
	YearsExperience   int                    `json:"years_experience"`
	ContentCategories string                 `gorm:"type:text" json:"content_categories"`  // Comma separated
	PastBrands        string                 `gorm:"type:text" json:"past_brands"`         // Comma separated brand names (legacy)
	RateCard          map[string]interface{} `gorm:"serializer:json" json:"rate_card"`     // per_post, per_reel, per_video, per_story, per_carousel, per_live, per_youtube_integration, per_youtube_short, per_barter, custom_packages
	SocialLinks       map[string]interface{} `gorm:"serializer:json" json:"social_links"`  // twitter, linkedin, website

	// ── Availability & logistics ─────────────────────────────────────────────
	AvailabilityStatus string `gorm:"type:varchar(20);default:'available'" json:"availability_status"` // available, busy, not_accepting
	TurnaroundDays     int    `json:"turnaround_days"`                                                 // avg content delivery days
	Location           string `json:"location"`                                                        // full address/region
	PinCode            string `gorm:"type:varchar(10)" json:"pin_code"`
	WillingToTravel    bool   `gorm:"default:false" json:"willing_to_travel"`

	// ── Audience demographics (JSONB) ────────────────────────────────────────
	// { age_split: {18-24: 35, ...}, gender_split: {male: 45, ...}, top_cities: ["Mumbai", ...], top_countries: ["India", ...] }
	AudienceDemographics map[string]interface{} `gorm:"serializer:json" json:"audience_demographics"`

	// ── Collaboration preferences (JSONB) ────────────────────────────────────
	// { preferred_categories: ["tech","lifestyle"], brand_size: ["startup","enterprise"], content_types: ["reel","long_form"], exclusivity_open: true, barter_open: true }
	CollaborationPrefs map[string]interface{} `gorm:"serializer:json" json:"collaboration_prefs"`

	// ── Structured past work (JSONB array) ───────────────────────────────────
	// [{ brand_name, deliverable_type, platform, date, url, description }]
	PastWork []map[string]interface{} `gorm:"serializer:json" json:"past_work"`

	// ── Performance metrics (computed, cached) ───────────────────────────────
	TotalCampaigns     int     `json:"total_campaigns"`
	CompletedCampaigns int     `json:"completed_campaigns"`
	AvgRating          float64 `json:"avg_rating"`
	ResponseTime       string  `gorm:"type:varchar(20)" json:"response_time"` // "< 1 hour", "< 6 hours", "< 24 hours"

	// ── JSONB for stats & portfolio ──────────────────────────────────────────
	CachedStats map[string]interface{}   `gorm:"serializer:json" json:"cached_stats"`
	Portfolio   []map[string]interface{} `gorm:"serializer:json" json:"portfolio"` // [{ title, url, platform, thumbnail_url, description, category }]
	UpdatedAt   time.Time                `json:"updated_at"`

	// Embedding fields for AI matching
	Embedding            pgvector.Vector `gorm:"type:vector(768)" json:"-"`
	EmbeddingLastUpdated *time.Time      `json:"embedding_last_updated,omitempty"`

	// Relationship
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// CalculateCompletion returns a 0-100 percentage based on how many profile sections are filled.
func (cp *CreatorProfile) CalculateCompletion() int {
	score := 0
	total := 10

	if cp.Headline != "" {
		score++
	}
	if cp.Bio != "" {
		score++
	}
	if cp.City != "" || cp.Location != "" {
		score++
	}
	if cp.Gender != "" || cp.DateOfBirth != nil {
		score++
	}
	if cp.Languages != "" {
		score++
	}
	if cp.ContentCategories != "" {
		score++
	}
	if len(cp.RateCard) >= 3 {
		score++
	}
	if len(cp.PastWork) > 0 || cp.PastBrands != "" {
		score++
	}
	if len(cp.SocialLinks) > 0 {
		score++
	}
	if len(cp.AudienceDemographics) > 0 {
		score++
	}

	return (score * 100) / total
}

type Campaign struct {
	ID           uuid.UUID              `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BrandID      uuid.UUID              `gorm:"type:uuid;not null" json:"brand_id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Budget       float64                `json:"budget"`
	Niche        string                 `json:"niche"`
	Platform     string                 `json:"platform"` // e.g. "instagram", "youtube"
	Requirements map[string]interface{} `gorm:"serializer:json" json:"requirements"`
	Status       CampaignStatus         `gorm:"default:'OPEN'" json:"status"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type Proposal struct {
	ID                uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CampaignID        uuid.UUID      `gorm:"type:uuid;not null" json:"campaign_id"`
	CreatorID         uuid.UUID      `gorm:"type:uuid;not null" json:"creator_id"`
	BidAmount         float64        `json:"bid_amount"`
	CoverNote         string         `json:"cover_note"`
	TrialVideoURL     string         `json:"trial_video_url"` // or just video_url
	Status            ProposalStatus `gorm:"default:'APPLIED'" json:"status"`
	RazorpayOrderID   string         `json:"razorpay_order_id"`
	RazorpayPaymentID string         `json:"razorpay_payment_id"`
	ProofURL          string         `json:"proof_url"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`

	Campaign Campaign `json:"campaign,omitempty" gorm:"foreignKey:CampaignID"`
}

// BankAccount stores creator bank account details for payouts
type BankAccount struct {
	ID                  uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID              uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	AccountHolderName   string    `json:"account_holder_name"`
	AccountNumber       string    `json:"account_number"`
	IFSC                string    `json:"ifsc"`
	BankName            string    `json:"bank_name"`
	RazorpayContactID   string    `json:"razorpay_contact_id"`
	RazorpayFundAccID   string    `json:"razorpay_fund_account_id"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Transaction struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	Amount    float64   `json:"amount"`
	Type      string    `json:"type"`      // "credit", "debit"
	Reference string    `json:"reference"` // Order ID or Payout ID
	CreatedAt time.Time `json:"created_at"`
}

type Message struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ProposalID uuid.UUID `gorm:"type:uuid;not null" json:"proposal_id"` // Room ID — proposal ID or direct conversation ID
	SenderID   uuid.UUID `gorm:"type:uuid;not null" json:"sender_id"`
	Content    string    `json:"content"`
	ImageURL   string    `json:"image_url"`
	CreatedAt  time.Time `json:"created_at"`
}

// DirectConversation stores a brand↔creator 1-on-1 chat outside of any proposal
type DirectConversation struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	User1ID   uuid.UUID `gorm:"type:uuid;not null;index" json:"user1_id"`
	User2ID   uuid.UUID `gorm:"type:uuid;not null;index" json:"user2_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SubscriptionPlan struct {
	ID             uuid.UUID              `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Amount         float64                `json:"amount"` // in smallest currency unit if needed, but float for now
	Currency       string                 `json:"currency" default:"INR"`
	Duration       int                    `json:"duration"`                            // in days
	TargetRole     Role                   `gorm:"type:varchar(20)" json:"target_role"` // CREATOR or BRAND
	RazorpayPlanID string                 `json:"razorpay_plan_id"`
	Features       map[string]interface{} `gorm:"serializer:json" json:"features"`
	IsActive       bool                   `json:"is_active" default:"true"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type Subscription struct {
	ID                     uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID                 uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	PlanID                 uuid.UUID `gorm:"type:uuid;not null" json:"plan_id"`
	RazorpaySubscriptionID string    `json:"razorpay_subscription_id"`
	RazorpayPaymentID      string    `json:"razorpay_payment_id"`
	Status                 string    `json:"status"` // created, authenticated, active, expired
	StartDate              time.Time `json:"start_date"`
	EndDate                time.Time `json:"end_date"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`

	// Relationships
	User User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Plan SubscriptionPlan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
}

type NotificationType string

const (
	NotifNewProposal      NotificationType = "NEW_PROPOSAL"
	NotifProposalAccepted NotificationType = "PROPOSAL_ACCEPTED"
	NotifProposalRejected NotificationType = "PROPOSAL_REJECTED"
	NotifNewMessage       NotificationType = "NEW_MESSAGE"
	NotifCampaignCreated  NotificationType = "CAMPAIGN_CREATED"
	NotifPaymentReceived  NotificationType = "PAYMENT_RECEIVED"
	NotifCampaignInvite   NotificationType = "CAMPAIGN_INVITE"
)

type Notification struct {
	ID         uuid.UUID        `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID     uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	Type       NotificationType `gorm:"type:varchar(50);not null" json:"type"`
	Title      string           `json:"title"`
	Body       string           `json:"body"`
	ResourceID string           `json:"resource_id"`
	IsRead     bool             `gorm:"default:false" json:"is_read"`
	CreatedAt  time.Time        `json:"created_at"`
}

type DeviceToken struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index" json:"user_id"`
	Token     string    `gorm:"not null;uniqueIndex" json:"token"`
	Platform  string    `gorm:"type:varchar(20)" json:"platform"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NotificationRepository interface {
	Create(n *Notification) error
	ListByUserID(userID uuid.UUID, limit int) ([]Notification, error)
	MarkRead(id uuid.UUID, userID uuid.UUID) error
	MarkAllRead(userID uuid.UUID) error
	UnreadCount(userID uuid.UUID) (int64, error)
	GetDeviceTokens(userID uuid.UUID) ([]string, error)
	SaveDeviceToken(token *DeviceToken) error
	DeleteDeviceToken(userID uuid.UUID, token string) error
	GetAllCreatorIDs() ([]uuid.UUID, error)
}

type NotificationService interface {
	Notify(userID uuid.UUID, nType NotificationType, title, body, resourceID string)
	NotifyAllCreators(nType NotificationType, title, body, resourceID string)
	List(userID uuid.UUID) ([]Notification, error)
	MarkRead(id string, userID uuid.UUID) error
	MarkAllRead(userID uuid.UUID) error
	UnreadCount(userID uuid.UUID) (int64, error)
	RegisterDevice(userID uuid.UUID, token, platform string) error
	UnregisterDevice(userID uuid.UUID, token string) error
}
