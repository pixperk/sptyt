package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/services"
	"github.com/uptrace/bun"
)

type CustomLinkHandler struct {
	service     *services.CustomLinkService
	db          *bun.DB
	frontendURL string
}

func NewCustomLinkHandler(service *services.CustomLinkService, db *bun.DB) *CustomLinkHandler {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	return &CustomLinkHandler{
		service:     service,
		db:          db,
		frontendURL: frontendURL,
	}
}

// CreateCustomLink creates a new custom link
func (h *CustomLinkHandler) CreateCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx := context.Background()

	// Get user
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.CreateLinkRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Set premium status
	req.IsPremium = user.IsPremium()

	// Validate custom slug for free users
	if req.CustomSlug != "" && !req.IsPremium {
		return echo.NewHTTPError(http.StatusForbidden, "Custom slugs are a premium feature")
	}

	// Check link limits for free users
	if !req.IsPremium {
		count, err := h.db.NewSelect().
			Model((*models.CustomLink)(nil)).
			Where("user_id = ?", user.ID).
			Count(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check limits")
		}
		if count >= 3 {
			return echo.NewHTTPError(http.StatusForbidden, map[string]interface{}{
				"error":           "Free tier limit reached",
				"current_links":   count,
				"max_links":       3,
				"upgrade_required": true,
			})
		}
	}

	link, err := h.service.CreateCustomLink(ctx, user.ID, req)
	if err != nil {
		log.Printf("CreateCustomLink: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"link":       link,
		"public_url": fmt.Sprintf("%s/l/%s", h.frontendURL, link.Slug),
	})
}

// GetUserLinks returns all custom links for the authenticated user
func (h *CustomLinkHandler) GetUserLinks(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx := context.Background()

	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Parse pagination
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	links, total, err := h.service.GetUserLinks(ctx, user.ID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get links")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"links":  links,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetCustomLink returns a specific custom link by ID (for owner)
func (h *CustomLinkHandler) GetCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx := context.Background()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	link, err := h.service.GetLinkByID(ctx, linkID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Link not found")
	}

	return c.JSON(http.StatusOK, link)
}

// UpdateCustomLink updates a custom link
func (h *CustomLinkHandler) UpdateCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx := context.Background()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.UpdateLinkRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	err = h.service.UpdateCustomLink(ctx, linkID, user.ID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Link updated successfully"})
}

// DeleteCustomLink deletes a custom link
func (h *CustomLinkHandler) DeleteCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx := context.Background()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	err = h.service.DeleteCustomLink(ctx, linkID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Link deleted successfully"})
}

// AddElement adds an element to a custom link
func (h *CustomLinkHandler) AddElement(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx := context.Background()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.AddElementRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Check element limits for free users
	if !user.IsPremium() {
		count, err := h.db.NewSelect().
			Model((*models.LinkElement)(nil)).
			Where("custom_link_id = ?", linkID).
			Count(ctx)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check limits")
		}
		if count >= 10 {
			return echo.NewHTTPError(http.StatusForbidden, "Free tier allows max 10 elements per link")
		}
	}

	element, err := h.service.AddElement(ctx, linkID, user.ID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, element)
}

// ReorderElements updates element display order
func (h *CustomLinkHandler) ReorderElements(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx := context.Background()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var order []services.ElementOrder
	if err := c.Bind(&order); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	err = h.service.ReorderElements(ctx, linkID, user.ID, order)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Elements reordered successfully"})
}

// DeleteElement removes an element from a custom link
func (h *CustomLinkHandler) DeleteElement(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	elementID, err := uuid.Parse(c.Param("element_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid element ID")
	}

	ctx := context.Background()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	err = h.service.DeleteElement(ctx, linkID, user.ID, elementID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Element deleted successfully"})
}

// ProxyToFrontend proxies the request to the frontend for rendering the custom link page
func (h *CustomLinkHandler) ProxyToFrontend(c echo.Context) error {
	slug := c.Param("slug")

	// Parse frontend URL
	frontendURL, err := url.Parse(h.frontendURL)
	if err != nil {
		log.Printf("Invalid frontend URL: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Configuration error")
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(frontendURL)

	// Modify the request
	req := c.Request()
	req.URL.Host = frontendURL.Host
	req.URL.Scheme = frontendURL.Scheme
	req.URL.Path = fmt.Sprintf("/l/%s", slug)
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = frontendURL.Host

	// Serve the proxy
	proxy.ServeHTTP(c.Response(), req)
	return nil
}
