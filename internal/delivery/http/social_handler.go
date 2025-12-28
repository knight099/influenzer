package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vaibhaw/influenzer-backend/internal/service"
)

type SocialHandler struct {
	service service.SocialService
}

func NewSocialHandler(r *gin.Engine, s service.SocialService, authMiddleware gin.HandlerFunc) {
	handler := &SocialHandler{service: s}

	g := r.Group("/proposals")
	g.Use(authMiddleware)
	{
		g.POST("/:id/submit-proof", handler.SubmitProof)
	}
}

type submitProofRequest struct {
	InstagramPostURL string `json:"instagram_post_url" binding:"required"`
}

func (h *SocialHandler) SubmitProof(c *gin.Context) {
	proposalID := c.Param("id")
	var req submitProofRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.SubmitProof(c.Request.Context(), proposalID, req.InstagramPostURL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) // Bad Request usually for verification fail
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Proof submitted and verified"})
}
