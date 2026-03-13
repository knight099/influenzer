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

	g := r.Group("/conversations")
	g.Use(authMiddleware)
	{
		g.GET("", handler.ListConversations)
		g.POST("", handler.GetOrCreateConversation)
		g.GET("/:id/messages", handler.GetHistory)
		g.POST("/:id/messages", handler.SendMessage)
	}
}

func (h *ChatHTTPHandler) ListConversations(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	userIDStr, _ := userIDVal.(string)
	userID, _ := uuid.Parse(userIDStr)

	// Get user to determine role
	var user domain.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var proposals []domain.Proposal
	var conversations []gin.H

	if user.Role == domain.RoleCreator {
		// Creator: Get proposals where they are the creator
		if err := h.db.Preload("Campaign").Where("creator_id = ?", userID).Find(&proposals).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		for _, p := range proposals {
			// Get brand info
			var brand domain.User
			h.db.Where("id = ?", p.Campaign.BrandID).First(&brand)

			// Get last message
			var lastMsg domain.Message
			h.db.Where("proposal_id = ?", p.ID).Order("created_at DESC").First(&lastMsg)

			conversations = append(conversations, gin.H{
				"id":           p.ID,
				"user":         brand.Name,
				"user_id":      brand.ID,
				"avatar_url":   brand.AvatarURL,
				"last_message": lastMsg.Content,
				"updated_at":   lastMsg.CreatedAt,
			})
		}
	} else if user.Role == domain.RoleBrand {
		// Brand: Get proposals for campaigns they own
		var campaigns []domain.Campaign
		if err := h.db.Where("brand_id = ?", userID).Find(&campaigns).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var campaignIDs []uuid.UUID
		for _, c := range campaigns {
			campaignIDs = append(campaignIDs, c.ID)
		}

		if len(campaignIDs) > 0 {
			if err := h.db.Where("campaign_id IN ?", campaignIDs).Find(&proposals).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		for _, p := range proposals {
			// Get creator info
			var creator domain.User
			h.db.Where("id = ?", p.CreatorID).First(&creator)

			// Get last message
			var lastMsg domain.Message
			h.db.Where("proposal_id = ?", p.ID).Order("created_at DESC").First(&lastMsg)

			conversations = append(conversations, gin.H{
				"id":           p.ID,
				"user":         creator.Name,
				"user_id":      creator.ID,
				"avatar_url":   creator.AvatarURL,
				"last_message": lastMsg.Content,
				"updated_at":   lastMsg.CreatedAt,
			})
		}
	}

	// Append direct conversations for this user
	var directConvs []domain.DirectConversation
	h.db.Where("user1_id = ? OR user2_id = ?", userID, userID).Find(&directConvs)
	for _, dc := range directConvs {
		otherID := dc.User2ID
		if dc.User2ID == userID {
			otherID = dc.User1ID
		}
		var other domain.User
		h.db.Where("id = ?", otherID).First(&other)

		var lastMsg domain.Message
		h.db.Where("proposal_id = ?", dc.ID).Order("created_at DESC").First(&lastMsg)

		conversations = append(conversations, gin.H{
			"id":           dc.ID,
			"user":         other.Name,
			"user_id":      other.ID,
			"avatar_url":   other.AvatarURL,
			"last_message": lastMsg.Content,
			"updated_at":   lastMsg.CreatedAt,
		})
	}

	// Return empty array if no conversations
	if conversations == nil {
		conversations = []gin.H{}
	}

	c.JSON(http.StatusOK, conversations)
}

type getOrCreateConversationRequest struct {
	RecipientID string `json:"recipient_id" binding:"required"`
}

func (h *ChatHTTPHandler) GetOrCreateConversation(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, _ := uuid.Parse(userIDVal.(string))

	var req getOrCreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	recipientID, err := uuid.Parse(req.RecipientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid recipient_id"})
		return
	}

	// Find existing direct conversation (check both orderings)
	var existing domain.DirectConversation
	err = h.db.Where(
		"(user1_id = ? AND user2_id = ?) OR (user1_id = ? AND user2_id = ?)",
		userID, recipientID, recipientID, userID,
	).First(&existing).Error

	if err == nil {
		c.JSON(http.StatusOK, gin.H{"id": existing.ID})
		return
	}

	// Create new direct conversation
	conv := domain.DirectConversation{
		User1ID: userID,
		User2ID: recipientID,
	}
	if err := h.db.Create(&conv).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": conv.ID})
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
