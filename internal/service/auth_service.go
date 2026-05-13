package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vaibhaw/influenzer-backend/config"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/api/idtoken"
)

type authService struct {
	authRepo domain.AuthRepository
	cfg      *config.Config // Pass config directly or through an interface
}

func NewAuthService(authRepo domain.AuthRepository, cfg *config.Config) domain.AuthService {
	return &authService{
		authRepo: authRepo,
		cfg:      cfg,
	}
}

func (s *authService) LoginWithGoogle(ctx context.Context, tokenString, providedName, providedAvatarURL string) (string, *domain.User, error) {
	var email, googleID, name, picture string

	// 1. Try to validate as ID Token
	payload, err := idtoken.Validate(ctx, tokenString, s.cfg.GoogleClientID)
	if err == nil {
		// Valid ID Token
		if e, ok := payload.Claims["email"].(string); ok {
			email = e
		}
		googleID = payload.Subject
		if n, ok := payload.Claims["name"].(string); ok {
			name = n
		}
		if p, ok := payload.Claims["picture"].(string); ok {
			picture = p
		}
	} else {
		// Log the ID token validation error for debugging
		fmt.Printf("ID Token validation failed: %v. Falling back to UserInfo endpoint.\n", err)

		// 2. Fallback: Try to use as Access Token (Legacy method)
		userInfo, err := s.fetchGoogleUserInfo(tokenString)
		if err != nil {
			return "", nil, errors.New("invalid google token: " + err.Error())
		}
		email = userInfo.Email
		googleID = userInfo.ID
		name = userInfo.Name
		picture = userInfo.Picture
	}

	// Prefer frontend-provided values over API-fetched values
	if providedName != "" {
		name = providedName
	}
	if providedAvatarURL != "" {
		picture = providedAvatarURL
	}

	// 3. User Lookup or Creation
	user, err := s.authRepo.GetUserByEmail(ctx, email)
	if err != nil {
		// Check by GoogleID if email lookup failed (though usually email is consistent)
		user, err = s.authRepo.GetUserByGoogleID(ctx, googleID)
	}

	if user == nil {
		// Create new user without a default role
		newUser := &domain.User{
			Email:     email,
			Name:      name,
			GoogleID:  googleID,
			AvatarURL: picture,
			// Role is empty - user must select via /auth/set-role
		}
		if err := s.authRepo.CreateUser(ctx, newUser); err != nil {
			return "", nil, err
		}
		user = newUser
	} else {
		// Update name/avatar if provided by frontend and different
		needsUpdate := false
		if user.GoogleID == "" {
			user.GoogleID = googleID
			needsUpdate = true
		}
		if providedName != "" && user.Name != providedName {
			user.Name = providedName
			needsUpdate = true
		}
		if providedAvatarURL != "" && user.AvatarURL != providedAvatarURL {
			user.AvatarURL = providedAvatarURL
			needsUpdate = true
		}
		if needsUpdate {
			_ = s.authRepo.UpdateUser(ctx, user)
		}
	}

	// 4. Generate JWT
	token, err := s.generateJWT(user)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (s *authService) fetchGoogleUserInfo(accessToken string) (*domain.GoogleUserInfo, error) {
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		fmt.Printf("Google UserInfo API Error: Status=%d Body=%s\n", resp.StatusCode, string(bodyBytes))
		return nil, fmt.Errorf("failed to validate google token: %s", string(bodyBytes))
	}

	var userInfo domain.GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}
	return &userInfo, nil
}

func (s *authService) generateJWT(user *domain.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"role":  user.Role,
		"exp":   time.Now().Add(time.Hour * 72).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Fallback secret if not configured
	secret := "secret-fallback"
	if s.cfg.JWTSecret != "" {
		secret = s.cfg.JWTSecret
	}
	return token.SignedString([]byte(secret))
}


func (s *authService) Register(ctx context.Context, email, password string, role domain.Role) (string, *domain.User, error) {
	existingUser, _ := s.authRepo.GetUserByEmail(ctx, email)
	if existingUser != nil {
		return "", nil, errors.New("email already exists")
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, err
	}

	newUser := &domain.User{
		Email:    email,
		Password: string(hashedBytes),
		Role:     role,
	}

	// Create associated profile based on role (GORM will handle the association on Create)
	if role == domain.RoleCreator {
		newUser.CreatorProfile = &domain.CreatorProfile{UserID: newUser.ID}
	} else if role == domain.RoleBrand {
		newUser.BrandProfile = &domain.BrandProfile{UserID: newUser.ID}
	}

	if err := s.authRepo.CreateUser(ctx, newUser); err != nil {
		return "", nil, err
	}

	token, err := s.generateJWT(newUser)
	return token, newUser, nil
}

func (s *authService) LoginWithEmail(ctx context.Context, email, password string) (string, *domain.User, error) {
	user, err := s.authRepo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	token, err := s.generateJWT(user)
	return token, user, nil
}

func (s *authService) ConnectSocial(ctx context.Context, userID, platform, authCode, redirectURI string) error {
	user, err := s.authRepo.GetBaseUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if platform == "instagram" {
		// Use client-provided redirect URI, fall back to configured one
		if redirectURI == "" {
			redirectURI = s.cfg.InstagramRedirectURI
		}

		token, err := s.exchangeInstagramToken(authCode, redirectURI)
		if err != nil {
			return err
		}
		user.InstagramToken = token
		if user.CreatorProfile != nil {
			user.CreatorProfile.Platform = platform
			// In real flow, we might fetch follower count here too using the token
		}
	} else if platform == "youtube" {
		// For native Google Sign-In, the correct redirect_uri is "postmessage"
		// (server auth code flow with no browser redirect).
		// For web OAuth code flow, the client sends the actual redirect_uri.
		if redirectURI == "" {
			redirectURI = "postmessage"
		}
		accessToken, refreshToken, err := s.exchangeGoogleToken(authCode, redirectURI)
		if err != nil {
			return err
		}
		user.YoutubeToken = accessToken
		if refreshToken != "" {
			user.YoutubeRefreshToken = refreshToken
		}
	} else {
		return errors.New("unsupported platform")
	}

	return s.authRepo.UpdateUser(ctx, user)
}

func (s *authService) SetRole(ctx context.Context, userID string, role domain.Role) error {
	user, err := s.authRepo.GetBaseUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// Update the user's role
	user.Role = role

	// Create appropriate profile based on role if it doesn't exist
	if role == domain.RoleCreator && user.CreatorProfile == nil {
		user.CreatorProfile = &domain.CreatorProfile{UserID: user.ID}
	} else if role == domain.RoleBrand && user.BrandProfile == nil {
		user.BrandProfile = &domain.BrandProfile{UserID: user.ID}
	}

	return s.authRepo.UpdateUser(ctx, user)
}

func (s *authService) exchangeInstagramToken(code, redirectURI string) (string, error) {
	// Instagram Basic Display API
	values := url.Values{}
	values.Set("client_id", s.cfg.InstagramClientID)
	values.Set("client_secret", s.cfg.InstagramClientSecret)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectURI)
	values.Set("code", code)

	resp, err := http.PostForm("https://api.instagram.com/oauth/access_token", values)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("instagram auth failed: %s", string(bodyBytes))
	}

	var res struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}

	return res.AccessToken, nil
}

func (s *authService) exchangeGoogleToken(code, redirectURI string) (accessToken, refreshToken string, err error) {
	// For YouTube server auth code exchange, we must use the Web client credentials
	// (the same Web Client ID used as serverClientId on the mobile side).
	// The Android client ID has no secret and cannot be used for server-side exchange.
	clientID := s.cfg.GoogleWebClientID
	if clientID == "" {
		clientID = s.cfg.GoogleClientID // fallback
	}
	clientSecret := s.cfg.GoogleWebClientSecret
	if clientSecret == "" {
		clientSecret = s.cfg.GoogleClientSecret // fallback
	}

	values := url.Values{}
	values.Set("client_id", clientID)
	values.Set("client_secret", clientSecret)
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectURI)
	values.Set("code", code)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", values)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Google Token Exchange failed: %s\n", string(bodyBytes))
		return "", "", fmt.Errorf("google auth failed: %s", string(bodyBytes))
	}

	var res struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", "", err
	}

	return res.AccessToken, res.RefreshToken, nil
}
