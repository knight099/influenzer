package domain

import (
	"context"
)

// AuthRepository defines the methods for user persistence
type AuthRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByGoogleID(ctx context.Context, googleID string) (*User, error)
	GetBaseUserByID(ctx context.Context, id string) (*User, error) // Renamed to avoid confusion
	UpdateUser(ctx context.Context, user *User) error
	GetCreatorProfileByUserID(ctx context.Context, userID string) (*CreatorProfile, error)
}

// AuthService defines the business logic for authentication
type AuthService interface {
	LoginWithGoogle(ctx context.Context, code, name, avatarURL string) (string, *User, error)
	LoginWithEmail(ctx context.Context, email, password string) (string, *User, error)
	Register(ctx context.Context, email, password string, role Role) (string, *User, error)
	ConnectSocial(ctx context.Context, userID, platform, authCode, redirectURI string) error
	SetRole(ctx context.Context, userID string, role Role) error
}

// GoogleUserInfo represents the data fetched from Google UserInfo API
type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}
