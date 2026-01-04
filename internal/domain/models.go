package domain

import (
	"time"

	"github.com/google/uuid"
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

	// Encrypted Tokens
	InstagramToken string `json:"-"`
	YoutubeToken   string `json:"-"`

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
	WalletBalance float64   `gorm:"default:0" json:"wallet_balance"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreatorProfile struct {
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	Niche     string    `gorm:"type:text" json:"niche"` // Comma separated tags
	MinBudget float64   `json:"min_budget"`
	City      string    `json:"city"`
	Phone     string    `json:"phone"`
	Platform  string    `json:"platform"` // e.g. "instagram", "youtube"
	// JSONB for stats
	CachedStats map[string]interface{} `gorm:"serializer:json" json:"cached_stats"`
	Portfolio   map[string]interface{} `gorm:"serializer:json" json:"portfolio"` // Store video links
	UpdatedAt   time.Time              `json:"updated_at"`

	// Relationship
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

type Campaign struct {
	ID           uuid.UUID              `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	BrandID      uuid.UUID              `gorm:"type:uuid;not null" json:"brand_id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Budget       float64                `json:"budget"`
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
	ProposalID uuid.UUID `gorm:"type:uuid;not null" json:"proposal_id"` // Room ID is usually Proposal ID
	SenderID   uuid.UUID `gorm:"type:uuid;not null" json:"sender_id"`
	Content    string    `json:"content"`
	ImageURL   string    `json:"image_url"`
	CreatedAt  time.Time `json:"created_at"`
}

type SubscriptionPlan struct {
	ID             uuid.UUID              `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Amount         float64                `json:"amount"` // in smallest currency unit if needed, but float for now
	Currency       string                 `json:"currency" default:"INR"`
	Duration       int                    `json:"duration"` // in days
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
