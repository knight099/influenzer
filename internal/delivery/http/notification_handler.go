package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vaibhaw/influenzer-backend/internal/domain"
)

type NotificationHandler struct {
	svc domain.NotificationService
}

func NewNotificationHandler(r *gin.Engine, svc domain.NotificationService, authMiddleware gin.HandlerFunc) {
	h := &NotificationHandler{svc: svc}
	g := r.Group("/notifications")
	g.Use(authMiddleware)
	{
		g.GET("", h.List)
		g.GET("/unread-count", h.UnreadCount)
		g.PATCH("/:id/read", h.MarkRead)
		g.PATCH("/read-all", h.MarkAllRead)
		g.POST("/device-token", h.RegisterDevice)
		g.DELETE("/device-token", h.UnregisterDevice)
	}
}

func (h *NotificationHandler) List(c *gin.Context) {
	userID := mustUserID(c)
	notifications, err := h.svc.List(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notifications)
}

func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID := mustUserID(c)
	count, err := h.svc.UnreadCount(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID := mustUserID(c)
	id := c.Param("id")
	if err := h.svc.MarkRead(id, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID := mustUserID(c)
	if err := h.svc.MarkAllRead(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "all marked as read"})
}

type deviceTokenRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform" binding:"required"`
}

func (h *NotificationHandler) RegisterDevice(c *gin.Context) {
	userID := mustUserID(c)
	var req deviceTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.RegisterDevice(userID, req.Token, req.Platform); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "device registered"})
}

type deleteTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

func (h *NotificationHandler) UnregisterDevice(c *gin.Context) {
	userID := mustUserID(c)
	var req deleteTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.UnregisterDevice(userID, req.Token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "device unregistered"})
}

// mustUserID extracts the authenticated user's UUID from gin context.
func mustUserID(c *gin.Context) uuid.UUID {
	val, _ := c.Get("userID")
	id, _ := uuid.Parse(val.(string))
	return id
}
