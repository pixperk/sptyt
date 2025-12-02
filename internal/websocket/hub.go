package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// ProgressEvent represents a progress update message
type ProgressEvent struct {
	Type         string      `json:"type"` // "started", "progress", "completed", "failed"
	ConversionID string      `json:"conversion_id"`
	Message      string      `json:"message,omitempty"`
	Data         interface{} `json:"data,omitempty"`
}

// ProgressData contains detailed progress information
type ProgressData struct {
	TotalTracks      int    `json:"total_tracks"`
	ProcessedTracks  int    `json:"processed_tracks"`
	SuccessCount     int    `json:"success_count"`
	FailureCount     int    `json:"failure_count"`
	CurrentTrack     string `json:"current_track,omitempty"`
	YouTubePlaylistID string `json:"youtube_playlist_id,omitempty"`
	YouTubePlaylistURL string `json:"youtube_playlist_url,omitempty"`
}

// Client represents a WebSocket client connection
type Client struct {
	Hub    *Hub
	Conn   *websocket.Conn
	UserID string
	Send   chan []byte
}

// Hub maintains active client connections and broadcasts messages
type Hub struct {
	// Registered clients mapped by user ID
	clients map[string]map[*Client]bool

	// Inbound messages from clients
	broadcast chan *BroadcastMessage

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	mu sync.RWMutex
}

// BroadcastMessage represents a message to broadcast to specific user
type BroadcastMessage struct {
	UserID  string
	Message []byte
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		broadcast:  make(chan *BroadcastMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// NewClient creates a new WebSocket client and registers it with the hub
func NewClient(hub *Hub, conn *websocket.Conn, userID string) *Client {
	client := &Client{
		Hub:    hub,
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}
	hub.register <- client
	return client
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if _, ok := h.clients[client.UserID]; !ok {
				h.clients[client.UserID] = make(map[*Client]bool)
			}
			h.clients[client.UserID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.UserID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.Send)
					if len(clients) == 0 {
						delete(h.clients, client.UserID)
					}
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := h.clients[message.UserID]
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.Send <- message.Message:
				default:
					// Client's send buffer is full, close the connection
					close(client.Send)
					h.mu.Lock()
					delete(h.clients[message.UserID], client)
					h.mu.Unlock()
				}
			}
		}
	}
}

// BroadcastToUser sends a message to all connections for a specific user
func (h *Hub) BroadcastToUser(userID string, event ProgressEvent) {
	messageBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("WebSocket: Failed to marshal event: %v", err)
		return
	}

	h.broadcast <- &BroadcastMessage{
		UserID:  userID,
		Message: messageBytes,
	}
}

// ReadPump handles reading messages from the WebSocket connection
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		// We don't expect clients to send messages, but we need to read to detect disconnects
	}
}

// WritePump handles writing messages to the WebSocket connection
func (c *Client) WritePump() {
	defer func() {
		c.Conn.Close()
	}()

	for {
		message, ok := <-c.Send
		if !ok {
			// Hub closed the channel
			c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}

		if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}
