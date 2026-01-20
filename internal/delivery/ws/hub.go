package ws

import (
	"log"
	"sync"

	"github.com/google/uuid"
)

// Client represents a connected WebSocket client
type Client struct {
	ID     uuid.UUID
	UserID uuid.UUID
	RoomID uuid.UUID
	Hub    *Hub
	Conn   *Connection
	Send   chan []byte
}

// Connection wraps the websocket connection
type Connection struct {
	WS interface {
		WriteMessage(messageType int, data []byte) error
		ReadMessage() (messageType int, p []byte, err error)
		Close() error
	}
}

// Hub maintains the set of active clients and broadcasts messages to clients in rooms
type Hub struct {
	// Registered clients by room
	rooms map[uuid.UUID]map[*Client]bool

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Broadcast messages to room
	broadcast chan *BroadcastMessage

	// Mutex for thread-safe operations
	mu sync.RWMutex
}

// BroadcastMessage represents a message to be broadcast to a room
type BroadcastMessage struct {
	RoomID  uuid.UUID
	Message []byte
	Sender  *Client // Optional: exclude sender from broadcast
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[uuid.UUID]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *BroadcastMessage),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.RoomID] == nil {
				h.rooms[client.RoomID] = make(map[*Client]bool)
			}
			h.rooms[client.RoomID][client] = true
			h.mu.Unlock()
			log.Printf("Client %s joined room %s (total clients in room: %d)",
				client.UserID, client.RoomID, len(h.rooms[client.RoomID]))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.rooms[client.RoomID]; ok {
				if _, ok := h.rooms[client.RoomID][client]; ok {
					delete(h.rooms[client.RoomID], client)
					close(client.Send)
					log.Printf("Client %s left room %s", client.UserID, client.RoomID)

					// Clean up empty rooms
					if len(h.rooms[client.RoomID]) == 0 {
						delete(h.rooms, client.RoomID)
						log.Printf("Room %s is now empty and removed", client.RoomID)
					}
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			if clients, ok := h.rooms[message.RoomID]; ok {
				for client := range clients {
					// Optionally skip sender
					if message.Sender != nil && client == message.Sender {
						continue
					}
					select {
					case client.Send <- message.Message:
					default:
						// Client buffer full, close connection
						close(client.Send)
						delete(h.rooms[message.RoomID], client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastToRoom sends a message to all clients in a room
func (h *Hub) BroadcastToRoom(roomID uuid.UUID, message []byte, excludeSender *Client) {
	h.broadcast <- &BroadcastMessage{
		RoomID:  roomID,
		Message: message,
		Sender:  excludeSender,
	}
}

// GetRoomClientCount returns the number of clients in a room
func (h *Hub) GetRoomClientCount(roomID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if clients, ok := h.rooms[roomID]; ok {
		return len(clients)
	}
	return 0
}
