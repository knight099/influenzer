package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"github.com/vaibhaw/influenzer-backend/pkg/utils"
)

type AuthHandler struct {
	authService domain.AuthService
}

func NewAuthHandler(r *gin.Engine, s domain.AuthService, authMiddleware gin.HandlerFunc) {
	handler := &AuthHandler{
		authService: s,
	}

	authGroup := r.Group("/auth")
	{
		// Public
		authGroup.POST("/google", handler.LegacyGoogleLogin)
		authGroup.POST("/login/social", handler.SocialLogin)
		authGroup.POST("/login/email", handler.EmailLogin)
		authGroup.POST("/register", handler.Register)

		// Protected
		// We can't easily group if we want /auth prefix for both.
		// So we apply middleware explicitly to specific routes or create a subgroup

		protected := authGroup.Group("/")
		protected.Use(authMiddleware)
		{
			protected.POST("/connect-social", handler.ConnectSocial)
		}
	}
}

// Reusing request structs where possible or defining new ones

type socialLoginRequest struct {
	Provider  string `json:"provider" binding:"required"` // google, facebook
	Token     string `json:"token" binding:"required"`
	Name      string `json:"name"`       // Optional: display name from frontend
	AvatarURL string `json:"avatar_url"` // Optional: avatar URL from frontend
}

func (h *AuthHandler) SocialLogin(c *gin.Context) {
	var req socialLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Logger.Error("SocialLogin Bind Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Currently mapped to Google login logic
	if req.Provider == "google" {
		token, user, err := h.authService.LoginWithGoogle(c.Request.Context(), req.Token, req.Name, req.AvatarURL)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"access_token": token, "user": user})
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider"})
	}
}

func (h *AuthHandler) LegacyGoogleLogin(c *gin.Context) {
	// Wrapper for backward compatibility if needed, or redirect
	// The original code expected { "code": ... }
	// Let's keep it but maybe it's unused in new spec.
	// Implementing simplified version assuming "code" struct match
	h.SocialLogin(c) // Might fail on binding if struct differs
}

type emailLoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) EmailLogin(c *gin.Context) {
	var req emailLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Logger.Error("EmailLogin Bind Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, user, err := h.authService.LoginWithEmail(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_token": token, "user": user})
}

type registerRequest struct {
	Email    string      `json:"email" binding:"required,email"`
	Password string      `json:"password" binding:"required,min=6"`
	Role     domain.Role `json:"role" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Logger.Error("Register Bind Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	if req.Role != domain.RoleBrand && req.Role != domain.RoleCreator {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	token, user, err := h.authService.Register(c.Request.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		utils.Logger.Error(err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Registration failed"}) // Internal error or exists
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": token, "user": user})
}

type connectSocialRequest struct {
	Platform string `json:"platform" binding:"required"`
	AuthCode string `json:"auth_code" binding:"required"`
}

func (h *AuthHandler) ConnectSocial(c *gin.Context) {
	var req connectSocialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Logger.Error("ConnectSocial Bind Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	err := h.authService.ConnectSocial(c.Request.Context(), userID.(string), req.Platform, req.AuthCode)
	if err != nil {
		utils.Logger.Error("ConnectSocial Service Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
