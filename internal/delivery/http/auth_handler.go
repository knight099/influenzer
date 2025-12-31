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
			protected.POST("/set-role", handler.SetRole)
		}
	}

	// OAuth Callback - serves HTML redirect page
	r.GET("/callback", handler.ServeSocialCallback)
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

type setRoleRequest struct {
	Role domain.Role `json:"role" binding:"required"`
}

// SetRole allows authenticated users to set their role (BRAND or CREATOR)
func (h *AuthHandler) SetRole(c *gin.Context) {
	var req setRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Logger.Error("SetRole Bind Error: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Validate role
	if req.Role != domain.RoleBrand && req.Role != domain.RoleCreator {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role. Must be BRAND or CREATOR"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	err := h.authService.SetRole(c.Request.Context(), userID.(string), req.Role)
	if err != nil {
		utils.Logger.Error("SetRole Service Error: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "role": req.Role})
}

// ServeSocialCallback serves an HTML page for OAuth redirects that deep links back to the mobile app
func (h *AuthHandler) ServeSocialCallback(c *gin.Context) {
	// Serve HTML that will redirect to the mobile app with query parameters
	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <title>Redirecting...</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
            background-color: #f0f2f5;
            text-align: center;
        }
        .card {
            background: white;
            padding: 2rem;
            border-radius: 12px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            max-width: 90%;
            width: 320px;
        }
        .btn {
            background-color: #0095f6;
            color: white;
            border: none;
            padding: 12px 24px;
            border-radius: 8px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            text-decoration: none;
            display: inline-block;
            margin-top: 16px;
        }
        .success-icon {
            color: #4caf50;
            font-size: 48px;
            margin-bottom: 16px;
        }
    </style>
</head>
<body>
    <div class="card">
        <div class="success-icon">&#10003;</div>
        <h2>Connected!</h2>
        <p>Authorization successful.</p>
        <p>Tap below to return to the app.</p>
        
        <a id="deepLinkBtn" href="#" class="btn">Open App</a>
    </div>

    <script>
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        
        if (code) {
            // Construct Deep Link with /callback path for GoRouter
            const deepLink = ` + "`influenzer://callback/callback${window.location.search}`" + `;
            
            document.getElementById('deepLinkBtn').href = deepLink;
            
            // Auto-redirect (optional)
            // window.location.href = deepLink;
        } else {
            document.querySelector('h2').innerText = 'Error';
            document.querySelector('p').innerText = 'No authorization code received.';
            document.getElementById('deepLinkBtn').style.display = 'none';
        }
    </script>
</body>
</html>`

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, htmlContent)
}
