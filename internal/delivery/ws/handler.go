package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
	"gorm.io/gorm"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev
	},
}

type ChatHandler struct {
	hub       *Hub
	db        *gorm.DB
	jwtSecret string
}

// ChatMessage represents a message sent/received via WebSocket
type ChatMessage struct {
	Type          string    `json:"type"` // "message", "typing", "read"
	Text          string    `json:"text,omitempty"`
	SenderID      string    `json:"sender_id,omitempty"`
	SenderName    string    `json:"sender_name,omitempty"`
	AttachmentURL string    `json:"attachment_url,omitempty"`
	Timestamp     time.Time `json:"timestamp,omitempty"`
	MessageID     string    `json:"message_id,omitempty"`
}

func NewChatHandler(r *gin.Engine, db *gorm.DB, jwtSecret string) *Hub {
	hub := NewHub()
	handler := &ChatHandler{
		hub:       hub,
		db:        db,
		jwtSecret: jwtSecret,
	}

	// Start the hub in a goroutine
	go hub.Run()

	r.GET("/ws/chat", handler.HandleWebSocket)

	return hub
}

func (h *ChatHandler) HandleWebSocket(c *gin.Context) {
	// Get room_id (proposal_id)
	roomIDStr := c.Query("room_id")
	if roomIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room_id is required"})
		return
	}

	roomID, err := uuid.Parse(roomIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid room_id format"})
		return
	}

	// Authenticate via token query param or Authorization header
	token := c.Query("token")
	if token == "" {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	// Parse and validate JWT
	userID, userName, err := h.validateToken(token)
	if err != nil {
		log.Printf("JWT validation failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}

	// Verify user has access to this room (proposal)
	if !h.hasRoomAccess(userID, roomID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied to this conversation"})
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Failed to upgrade ws:", err)
		return
	}

	// Create client
	client := &Client{
		ID:     uuid.New(),
		UserID: userID,
		RoomID: roomID,
		Hub:    h.hub,
		Conn:   &Connection{WS: conn},
		Send:   make(chan []byte, 256),
	}

	// Register client with hub
	h.hub.register <- client

	// Send join notification to room
	joinMsg := ChatMessage{
		Type:       "user_joined",
		SenderID:   userID.String(),
		SenderName: userName,
		Timestamp:  time.Now(),
	}
	if msgBytes, err := json.Marshal(joinMsg); err == nil {
		h.hub.BroadcastToRoom(roomID, msgBytes, client)
	}

	// Start goroutines for reading and writing
	go h.writePump(client, conn)
	go h.readPump(client, conn, userName)
}

func (h *ChatHandler) validateToken(tokenString string) (uuid.UUID, string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return uuid.Nil, "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, "", jwt.ErrTokenInvalidClaims
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok {
		return uuid.Nil, "", jwt.ErrTokenInvalidClaims
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, "", err
	}

	// Get user name from database
	var user domain.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return uuid.Nil, "", err
	}

	return userID, user.Name, nil
}

func (h *ChatHandler) hasRoomAccess(userID, roomID uuid.UUID) bool {
	// Check if user is part of this proposal (either creator or brand)
	var proposal domain.Proposal
	if err := h.db.Preload("Campaign").Where("id = ?", roomID).First(&proposal).Error; err != nil {
		return false
	}

	// User is the creator
	if proposal.CreatorID == userID {
		return true
	}

	// User is the brand (campaign owner)
	if proposal.Campaign.BrandID == userID {
		return true
	}

	return false
}

func (h *ChatHandler) readPump(client *Client, conn *websocket.Conn, senderName string) {
	defer func() {
		h.hub.unregister <- client
		conn.Close()
	}()

	conn.SetReadLimit(512 * 1024) // 512KB max message size
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Parse incoming message
		var incomingMsg ChatMessage
		if err := json.Unmarshal(message, &incomingMsg); err != nil {
			log.Printf("Failed to parse message: %v", err)
			continue
		}

		// Handle different message types
		switch incomingMsg.Type {
		case "message", "":
			// Save message to database
			msg := domain.Message{
				ProposalID: client.RoomID,
				SenderID:   client.UserID,
				Content:    incomingMsg.Text,
				ImageURL:   incomingMsg.AttachmentURL,
				CreatedAt:  time.Now(),
			}

			if err := h.db.Create(&msg).Error; err != nil {
				log.Printf("Failed to save message: %v", err)
				continue
			}

			// Prepare broadcast message
			broadcastMsg := ChatMessage{
				Type:          "message",
				MessageID:     msg.ID.String(),
				Text:          msg.Content,
				SenderID:      client.UserID.String(),
				SenderName:    senderName,
				AttachmentURL: msg.ImageURL,
				Timestamp:     msg.CreatedAt,
			}

			if msgBytes, err := json.Marshal(broadcastMsg); err == nil {
				// Broadcast to all in room (including sender for confirmation)
				h.hub.BroadcastToRoom(client.RoomID, msgBytes, nil)
			}

		case "typing":
			// Broadcast typing indicator (don't persist)
			typingMsg := ChatMessage{
				Type:       "typing",
				SenderID:   client.UserID.String(),
				SenderName: senderName,
			}
			if msgBytes, err := json.Marshal(typingMsg); err == nil {
				h.hub.BroadcastToRoom(client.RoomID, msgBytes, client)
			}

		case "read":
			// Could update read receipts here
			log.Printf("User %s read messages in room %s", client.UserID, client.RoomID)
		}
	}
}

func (h *ChatHandler) writePump(client *Client, conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second) // Ping interval
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.Send:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub closed the channel
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
