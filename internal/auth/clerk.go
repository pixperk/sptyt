package auth

import (
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwt"
	"github.com/labstack/echo/v4"
)

type ClerkMiddleware struct{}

func NewClerkMiddleware(apiKey string) *ClerkMiddleware {
	clerk.SetKey(apiKey)
	return &ClerkMiddleware{}
}

// RequireAuth is middleware that requires authentication
func (cm *ClerkMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionToken := extractToken(c)
			if sessionToken == "" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Missing authentication token",
				})
			}

			claims, err := jwt.Verify(c.Request().Context(), &jwt.VerifyParams{
				Token: sessionToken,
			})
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Invalid authentication token",
				})
			}

			c.Set("clerk_user_id", claims.Subject)
			c.Set("clerk_session_id", claims.SessionID)

			return next(c)
		}
	}
}

// OptionalAuth is middleware that optionally checks for authentication
func (cm *ClerkMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionToken := extractToken(c)
			if sessionToken != "" {
				claims, err := jwt.Verify(c.Request().Context(), &jwt.VerifyParams{
					Token: sessionToken,
				})
				if err == nil {
					c.Set("clerk_user_id", claims.Subject)
					c.Set("clerk_session_id", claims.SessionID)
				}
			}

			return next(c)
		}
	}
}

// extractToken extracts the session token from Authorization header or cookie
func extractToken(c echo.Context) string {
	// Try Authorization header first
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}

	// Try cookie as fallback
	cookie, err := c.Cookie("__session")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// GetClerkUserID retrieves the Clerk user ID from context
func GetClerkUserID(c echo.Context) (string, bool) {
	userID, ok := c.Get("clerk_user_id").(string)
	return userID, ok
}

// GetClerkSessionID retrieves the Clerk session ID from context
func GetClerkSessionID(c echo.Context) (string, bool) {
	sessionID, ok := c.Get("clerk_session_id").(string)
	return sessionID, ok
}

// VerifyToken verifies a Clerk JWT token and returns the user ID
func VerifyToken(c echo.Context, token string) (string, error) {
	claims, err := jwt.Verify(c.Request().Context(), &jwt.VerifyParams{
		Token: token,
	})
	if err != nil {
		return "", err
	}
	return claims.Subject, nil
}
