package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/pkg/utils"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
)

type CustomLinkService struct {
	db *bun.DB
}

func NewCustomLinkService(db *bun.DB) *CustomLinkService {
	return &CustomLinkService{db: db}
}

// CreateCustomLink creates a new custom link for a user
func (s *CustomLinkService) CreateCustomLink(ctx context.Context, userID uuid.UUID, req CreateLinkRequest) (*models.CustomLink, error) {
	// Validate input lengths
	if len(req.Title) > 200 {
		return nil, fmt.Errorf("title too long (max 200 characters)")
	}
	if len(req.Description) > 1000 {
		return nil, fmt.Errorf("description too long (max 1000 characters)")
	}
	if len(req.CustomSlug) > 100 {
		return nil, fmt.Errorf("custom slug too long (max 100 characters)")
	}

	// Generate slug
	var slug string
	if req.CustomSlug != "" {
		// Premium feature: custom slug
		slug = utils.GenerateSlug(req.CustomSlug)

		// Check if slug already exists
		exists, err := s.slugExists(ctx, slug)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, fmt.Errorf("slug '%s' is already taken", slug)
		}
	} else {
		// Generate random slug for free users or if not specified
		slug = s.generateUniqueSlug(ctx, req.Title)
	}

	// Calculate expiration for free users
	var expiresAt *time.Time
	if !req.IsPremium {
		expiry := time.Now().Add(7 * 24 * time.Hour) // 7 days
		expiresAt = &expiry
	}

	// Hash password if provided
	var passwordHash string
	if req.Password != "" {
		if !req.IsPremium {
			return nil, fmt.Errorf("password protection is a premium feature")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		passwordHash = string(hash)
	}

	link := &models.CustomLink{
		UserID:              userID,
		Slug:                slug,
		Title:               req.Title,
		Description:         req.Description,
		ProfileImage:        req.ProfileImage,
		Theme:               req.Theme,
		IsPasswordProtected: req.Password != "",
		PasswordHash:        passwordHash,
		ConversionID:        req.ConversionID,
		ExpiresAt:           expiresAt,
		IsPublic:            req.IsPublic,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	_, err := s.db.NewInsert().Model(link).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom link: %w", err)
	}

	return link, nil
}

// GetUserLinks returns all custom links for a user
func (s *CustomLinkService) GetUserLinks(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.CustomLink, int, error) {
	// Enforce pagination limits
	if limit <= 0 || limit > 100 {
		limit = 20 // Default to 20
	}
	if offset < 0 {
		offset = 0
	}

	var links []models.CustomLink

	query := s.db.NewSelect().
		Model(&links).
		Where("user_id = ?", userID).
		Order("created_at DESC")

	// Get total count
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	err = query.Limit(limit).Offset(offset).Scan(ctx)
	if err != nil {
		return nil, 0, err
	}

	return links, total, nil
}

// GetLinkBySlug retrieves a custom link by its slug (for public access)
func (s *CustomLinkService) GetLinkBySlug(ctx context.Context, slug string) (*models.CustomLink, error) {
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("slug = ?", slug).
		Relation("Elements", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("is_visible = ?", true).Order("display_index ASC")
		}).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return &link, nil
}

// GetLinkByID retrieves a custom link by ID (for owner access)
func (s *CustomLinkService) GetLinkByID(ctx context.Context, linkID, userID uuid.UUID) (*models.CustomLink, error) {
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Relation("Elements", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Order("display_index ASC")
		}).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return &link, nil
}

// UpdateCustomLink updates an existing custom link
func (s *CustomLinkService) UpdateCustomLink(ctx context.Context, linkID, userID uuid.UUID, req UpdateLinkRequest) error {
	// Validate input lengths
	if req.Title != nil && len(*req.Title) > 200 {
		return fmt.Errorf("title too long (max 200 characters)")
	}
	if req.Description != nil && len(*req.Description) > 1000 {
		return fmt.Errorf("description too long (max 1000 characters)")
	}

	update := s.db.NewUpdate().
		Model((*models.CustomLink)(nil)).
		Where("id = ? AND user_id = ?", linkID, userID).
		Set("updated_at = ?", time.Now())

	if req.Title != nil {
		update = update.Set("title = ?", *req.Title)
	}
	if req.Description != nil {
		update = update.Set("description = ?", *req.Description)
	}
	if req.ProfileImage != nil {
		update = update.Set("profile_image = ?", *req.ProfileImage)
	}
	if req.Theme != nil {
		update = update.Set("theme = ?", *req.Theme)
	}
	if req.IsPublic != nil {
		update = update.Set("is_public = ?", *req.IsPublic)
	}

	result, err := update.Exec(ctx)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("custom link not found")
	}

	return nil
}

// DeleteCustomLink deletes a custom link and its elements
func (s *CustomLinkService) DeleteCustomLink(ctx context.Context, linkID, userID uuid.UUID) error {
	// Delete elements first
	_, err := s.db.NewDelete().
		Model((*models.LinkElement)(nil)).
		Where("custom_link_id = ?", linkID).
		Exec(ctx)
	if err != nil {
		return err
	}

	// Delete link
	result, err := s.db.NewDelete().
		Model((*models.CustomLink)(nil)).
		Where("id = ? AND user_id = ?", linkID, userID).
		Exec(ctx)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("custom link not found")
	}

	return nil
}

// AddElement adds an element to a custom link
func (s *CustomLinkService) AddElement(ctx context.Context, linkID, userID uuid.UUID, req AddElementRequest) (*models.LinkElement, error) {
	// Validate element data
	if len(req.ElementData.CustomText) > 5000 {
		return nil, fmt.Errorf("custom text too long (max 5000 characters)")
	}
	if len(req.ElementData.Title) > 500 {
		return nil, fmt.Errorf("title too long (max 500 characters)")
	}

	// Verify ownership
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("custom link not found")
	}

	// Get max display index
	maxIndex := 0
	s.db.NewSelect().
		Model((*models.LinkElement)(nil)).
		Where("custom_link_id = ?", linkID).
		ColumnExpr("COALESCE(MAX(display_index), -1) + 1").
		Scan(ctx, &maxIndex)

	element := &models.LinkElement{
		CustomLinkID: linkID,
		ElementType:  req.ElementType,
		ElementData:  req.ElementData,
		DisplayIndex: maxIndex,
		IsVisible:    true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err = s.db.NewInsert().Model(element).Exec(ctx)
	if err != nil {
		return nil, err
	}

	return element, nil
}

// UpdateElement updates an existing element in a custom link
func (s *CustomLinkService) UpdateElement(ctx context.Context, linkID, userID, elementID uuid.UUID, req UpdateElementRequest) error {
	// Verify ownership through link
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("custom link not found")
	}

	update := s.db.NewUpdate().
		Model((*models.LinkElement)(nil)).
		Where("id = ? AND custom_link_id = ?", elementID, linkID).
		Set("updated_at = ?", time.Now())

	if req.ElementData != nil {
		update = update.Set("element_data = ?", req.ElementData)
	}
	if req.DisplayIndex != nil {
		update = update.Set("display_index = ?", *req.DisplayIndex)
	}
	if req.IsVisible != nil {
		update = update.Set("is_visible = ?", *req.IsVisible)
	}

	result, err := update.Exec(ctx)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("element not found")
	}

	return nil
}

// BatchUpdateElements updates multiple elements at once
func (s *CustomLinkService) BatchUpdateElements(ctx context.Context, linkID, userID uuid.UUID, updates []ElementUpdate) error {
	// Enforce batch update limits
	if len(updates) > 50 {
		return fmt.Errorf("too many updates in batch (max 50, got %d)", len(updates))
	}

	// Verify ownership
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("custom link not found")
	}

	// Update each element in a transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, update := range updates {
		_, err := tx.NewUpdate().
			Model((*models.LinkElement)(nil)).
			Set("element_data = ?", update.ElementData).
			Set("updated_at = ?", time.Now()).
			Where("id = ? AND custom_link_id = ?", update.ElementID, linkID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to update element %s: %w", update.ElementID, err)
		}
	}

	return tx.Commit()
}

// ReorderElements updates the display order of elements
func (s *CustomLinkService) ReorderElements(ctx context.Context, linkID, userID uuid.UUID, order []ElementOrder) error {
	// Verify ownership
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("custom link not found")
	}

	// Update each element's display_index
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range order {
		_, err := tx.NewUpdate().
			Model((*models.LinkElement)(nil)).
			Set("display_index = ?", item.Index).
			Set("updated_at = ?", time.Now()).
			Where("id = ? AND custom_link_id = ?", item.ElementID, linkID).
			Exec(ctx)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteElement removes an element from a custom link
func (s *CustomLinkService) DeleteElement(ctx context.Context, linkID, userID, elementID uuid.UUID) error {
	// Verify ownership through link
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Scan(ctx)
	if err != nil {
		return fmt.Errorf("custom link not found")
	}

	result, err := s.db.NewDelete().
		Model((*models.LinkElement)(nil)).
		Where("id = ? AND custom_link_id = ?", elementID, linkID).
		Exec(ctx)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("element not found")
	}

	return nil
}

// VerifyPassword checks if the provided password matches the link's password
func (s *CustomLinkService) VerifyPassword(ctx context.Context, slug, password string) (bool, error) {
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Column("password_hash").
		Where("slug = ?", slug).
		Scan(ctx)
	if err != nil {
		return false, err
	}

	if !link.IsPasswordProtected {
		return true, nil
	}

	err = bcrypt.CompareHashAndPassword([]byte(link.PasswordHash), []byte(password))
	return err == nil, nil
}

// IncrementViewCount increments the view count for a custom link
func (s *CustomLinkService) IncrementViewCount(ctx context.Context, linkID uuid.UUID) error {
	_, err := s.db.NewUpdate().
		Model((*models.CustomLink)(nil)).
		Set("view_count = view_count + 1").
		Where("id = ?", linkID).
		Exec(ctx)
	return err
}

// TrackPageView records a page view event in analytics and increments unique view count
func (s *CustomLinkService) TrackPageView(ctx context.Context, linkID uuid.UUID, ipAddress, userAgent, referrer string) error {
	now := time.Now()

	// Check if this IP has visited today
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.Add(24 * time.Hour)

	existingView, err := s.db.NewSelect().
		Model((*models.LinkAnalytics)(nil)).
		Where("custom_link_id = ?", linkID).
		Where("event_type = ?", "page_view").
		Where("ip_address = ?", ipAddress).
		Where("created_at >= ?", todayStart).
		Where("created_at < ?", todayEnd).
		Exists(ctx)
	if err != nil {
		return err
	}

	// Record the page view in analytics
	analytics := &models.LinkAnalytics{
		CustomLinkID:  linkID,
		LinkElementID: nil,
		EventType:     "page_view",
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		Referrer:      referrer,
		CreatedAt:     now,
	}

	_, err = s.db.NewInsert().Model(analytics).Exec(ctx)
	if err != nil {
		return err
	}

	// Only increment view_count if this is a unique visitor today
	if !existingView {
		_, err = s.db.NewUpdate().
			Model((*models.CustomLink)(nil)).
			Set("view_count = view_count + 1").
			Where("id = ?", linkID).
			Exec(ctx)
	}

	return err
}

// TrackElementClick records an element click and increments click count
func (s *CustomLinkService) TrackElementClick(ctx context.Context, linkID, elementID uuid.UUID, ipAddress, userAgent, referrer string) error {
	// Increment element click count
	_, err := s.db.NewUpdate().
		Model((*models.LinkElement)(nil)).
		Set("click_count = click_count + 1").
		Where("id = ? AND custom_link_id = ?", elementID, linkID).
		Exec(ctx)
	if err != nil {
		return err
	}

	// Record analytics event
	analytics := &models.LinkAnalytics{
		CustomLinkID:  linkID,
		LinkElementID: &elementID,
		EventType:     "element_click",
		IPAddress:     ipAddress,
		UserAgent:     userAgent,
		Referrer:      referrer,
		CreatedAt:     time.Now(),
	}

	_, err = s.db.NewInsert().Model(analytics).Exec(ctx)
	return err
}

// GetLinkAnalytics returns analytics summary for a custom link
func (s *CustomLinkService) GetLinkAnalytics(ctx context.Context, linkID, userID uuid.UUID) (map[string]interface{}, error) {
	// Verify ownership
	var link models.CustomLink
	err := s.db.NewSelect().
		Model(&link).
		Where("id = ? AND user_id = ?", linkID, userID).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("custom link not found")
	}

	// Get total page views (all visits)
	pageViews, err := s.db.NewSelect().
		Model((*models.LinkAnalytics)(nil)).
		Where("custom_link_id = ? AND event_type = ?", linkID, "page_view").
		Count(ctx)
	if err != nil {
		return nil, err
	}

	// Get total element clicks
	elementClicks, err := s.db.NewSelect().
		Model((*models.LinkAnalytics)(nil)).
		Where("custom_link_id = ? AND event_type = ?", linkID, "element_click").
		Count(ctx)
	if err != nil {
		return nil, err
	}

	// Get element-wise click counts with element details
	type ElementStats struct {
		ElementID   uuid.UUID `bun:"element_id"`
		ElementType string    `bun:"element_type"`
		Title       string    `bun:"title"`
		ClickCount  int       `bun:"click_count"`
	}

	var elementStats []ElementStats
	err = s.db.NewSelect().
		TableExpr("link_elements AS le").
		ColumnExpr("le.id AS element_id").
		ColumnExpr("le.element_type AS element_type").
		ColumnExpr("COALESCE(le.element_data->>'title', le.element_data->>'custom_text', 'Untitled') AS title").
		ColumnExpr("COUNT(la.id) AS click_count").
		Join("LEFT JOIN link_analytics AS la ON la.link_element_id = le.id AND la.event_type = 'element_click'").
		Where("le.custom_link_id = ?", linkID).
		Group("le.id").
		Group("le.element_type").
		Order("click_count DESC").
		Scan(ctx, &elementStats)
	if err != nil {
		return nil, err
	}

	// Get recent views (last 30 days)
	thirtyDaysAgo := time.Now().Add(-30 * 24 * time.Hour)
	recentViews, err := s.db.NewSelect().
		Model((*models.LinkAnalytics)(nil)).
		Where("custom_link_id = ? AND event_type = ? AND created_at > ?", linkID, "page_view", thirtyDaysAgo).
		Count(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"link_id":         linkID,
		"total_views":     pageViews,
		"unique_views":    link.ViewCount, // view_count stores unique daily visitors
		"total_clicks":    elementClicks,
		"recent_views":    recentViews,
		"element_stats":   elementStats,
	}, nil
}

// Helper methods

func (s *CustomLinkService) slugExists(ctx context.Context, slug string) (bool, error) {
	count, err := s.db.NewSelect().
		Model((*models.CustomLink)(nil)).
		Where("slug = ?", slug).
		Count(ctx)
	return count > 0, err
}

func (s *CustomLinkService) generateUniqueSlug(ctx context.Context, title string) string {
	baseSlug := utils.GenerateSlug(title)
	if baseSlug == "" {
		baseSlug = utils.GenerateRandomSlug(8)
	}

	slug := baseSlug
	for i := 0; i < 5; i++ {
		exists, _ := s.slugExists(ctx, slug)
		if !exists {
			return slug
		}
		slug = utils.AppendRandomSuffix(baseSlug)
	}

	// Fallback to fully random slug
	return utils.GenerateRandomSlug(12)
}

// Request/Response types

type CreateLinkRequest struct {
	Title        string     `json:"title" validate:"required"`
	Description  string     `json:"description"`
	ProfileImage string     `json:"profile_image"` // Cloudinary URL
	CustomSlug   string     `json:"custom_slug"`   // Premium only
	Theme        string     `json:"theme"`
	Password     string     `json:"password"`      // Premium only
	ConversionID *uuid.UUID `json:"conversion_id"`
	IsPublic     bool       `json:"is_public"`
	IsPremium    bool       `json:"-"` // Set by handler based on user
}

type UpdateLinkRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	ProfileImage *string `json:"profile_image"` // Cloudinary URL
	Theme        *string `json:"theme"`
	IsPublic     *bool   `json:"is_public"`
}

type AddElementRequest struct {
	ElementType  string             `json:"element_type" validate:"required"`
	ElementData  models.ElementData `json:"element_data" validate:"required"`
	DisplayIndex int                `json:"display_index"`
}

type UpdateElementRequest struct {
	ElementData  *models.ElementData `json:"element_data"`
	DisplayIndex *int                `json:"display_index"`
	IsVisible    *bool               `json:"is_visible"`
}

type ElementOrder struct {
	ElementID uuid.UUID `json:"element_id"`
	Index     int       `json:"index"`
}

type BatchUpdateElementRequest struct {
	Updates []ElementUpdate `json:"updates" validate:"required"`
}

type ElementUpdate struct {
	ElementID   uuid.UUID           `json:"element_id" validate:"required"`
	ElementData models.ElementData  `json:"element_data"`
}
