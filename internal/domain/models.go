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
	Platform  string    `json:"platform"` // e.g. "instagram", "youtube"
	// JSONB for stats
	CachedStats map[string]interface{} `gorm:"serializer:json" json:"cached_stats"`
	Portfolio   map[string]interface{} `gorm:"serializer:json" json:"portfolio"` // Store video links
	UpdatedAt   time.Time              `json:"updated_at"`
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
