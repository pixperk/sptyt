package handlers

import (
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	ws "github.com/pixperk/sptyt/internal/websocket"
)

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	hub      *ws.Hub
	upgrader websocket.Upgrader
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(hub *ws.Hub, allowedOrigins []string) *WebSocketHandler {
	originSet := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		parsed, err := url.Parse(o)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" {
			originSet[parsed.Scheme+"://"+parsed.Host] = true
		}
	}

	return &WebSocketHandler{
		hub: hub,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return false
				}
				return originSet[origin]
			},
			Subprotocols: []string{"authorization"},
		},
	}
}

// HandleConnection handles WebSocket connection requests
func (h *WebSocketHandler) HandleConnection(c echo.Context) error {
	// Get token from Sec-WebSocket-Protocol header (subprotocol)
	// Format: "authorization, <token>"
	var clerkUserID string
	subprotocols := c.Request().Header.Get("Sec-WebSocket-Protocol")

	if subprotocols != "" && len(subprotocols) > len("authorization, ") {
		// Extract token from "authorization, token" format
		token := subprotocols[len("authorization, "):]

		// Verify the token
		var err error
		clerkUserID, err = auth.VerifyToken(c, token)
		if err != nil {
			log.Printf("WebSocket: Token verification failed: %v", err)
			return echo.NewHTTPError(http.StatusUnauthorized, "Invalid authentication token")
		}
	} else {
		return echo.NewHTTPError(http.StatusUnauthorized, "Missing authentication token in subprotocol")
	}

	// Upgrade HTTP connection to WebSocket with authorization subprotocol
	responseHeader := http.Header{}
	responseHeader.Set("Sec-WebSocket-Protocol", "authorization")

	conn, err := h.upgrader.Upgrade(c.Response(), c.Request(), responseHeader)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return err
	}

	// Create and register client
	client := ws.NewClient(h.hub, conn, clerkUserID)

	// Start goroutines for reading and writing
	go client.WritePump()
	go client.ReadPump()

	return nil
}
