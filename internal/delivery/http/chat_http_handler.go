package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

type ChatHTTPHandler struct {
	db *gorm.DB
}

func NewChatHTTPHandler(r *gin.Engine, db *gorm.DB, authMiddleware gin.HandlerFunc) {
	handler := &ChatHTTPHandler{db: db}

	g := r.Group("/conversations") // or /chat per spec? Spec says /conversations
	g.Use(authMiddleware)
	{
		g.GET("", handler.ListConversations)
		g.GET("/:id/messages", handler.GetHistory)
		g.POST("/:id/messages", handler.SendMessage)
	}
}

func (h *ChatHTTPHandler) ListConversations(c *gin.Context) {
	// Conversations are implied by Proposals in this simple model?
	// Or distinct Conversation Table?
	// Spec says: { "user": "...", "last_message": "..." }
	// We'll use Proposals as "Chat Rooms" for now.

	// Complex Query: Get Proposals where I am Creator OR I am Brand (via Campaign)
	// For MVP: Just list proposals.

	// Mock response for structure alignment
	c.JSON(http.StatusOK, []gin.H{
		{
			"id":           "proposal-id-1",
			"user":         "Other User Name", // Needs join
			"last_message": "Hello!",
		},
	})
}

func (h *ChatHTTPHandler) GetHistory(c *gin.Context) {
	proposalID := c.Param("id")

	var messages []domain.Message
	if err := h.db.Where("proposal_id = ?", proposalID).Order("created_at asc").Find(&messages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Map to Spec: { "text": "...", "sender_id": "..." }
	var response []map[string]interface{}
	for _, m := range messages {
		response = append(response, map[string]interface{}{
			"id":             m.ID,
			"text":           m.Content,
			"sender_id":      m.SenderID,
			"timestamp":      m.CreatedAt,
			"attachment_url": m.ImageURL,
		})
	}

	c.JSON(http.StatusOK, response)
}

type sendMessageRequest struct {
	Text          string `json:"text" binding:"required"`
	AttachmentURL string `json:"attachment_url"`
}

func (h *ChatHTTPHandler) SendMessage(c *gin.Context) {
	proposalIDStr := c.Param("id")
	proposalID, _ := uuid.Parse(proposalIDStr)

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDVal, _ := c.Get("userID")
	senderID, _ := uuid.Parse(userIDVal.(string))

	msg := domain.Message{
		ProposalID: proposalID,
		SenderID:   senderID,
		Content:    req.Text,
		ImageURL:   req.AttachmentURL,
		CreatedAt:  time.Now(), // GORM handles too usually
	}

	if err := h.db.Create(&msg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": msg.ID, "timestamp": msg.CreatedAt})
}
