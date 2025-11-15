package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode represents a unique error code for programmatic handling
type ErrorCode string

const (
	// Authentication & Authorization errors
	ErrCodeUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden        ErrorCode = "FORBIDDEN"
	ErrCodeInvalidToken     ErrorCode = "INVALID_TOKEN"
	ErrCodeTokenExpired     ErrorCode = "TOKEN_EXPIRED"

	// Resource errors
	ErrCodeNotFound         ErrorCode = "NOT_FOUND"
	ErrCodeAlreadyExists    ErrorCode = "ALREADY_EXISTS"
	ErrCodeConflict         ErrorCode = "CONFLICT"

	// Validation errors
	ErrCodeValidation       ErrorCode = "VALIDATION_ERROR"
	ErrCodeInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrCodeMissingField     ErrorCode = "MISSING_FIELD"
	ErrCodeInvalidFormat    ErrorCode = "INVALID_FORMAT"
	ErrCodeExceedsLimit     ErrorCode = "EXCEEDS_LIMIT"

	// Quota & Limit errors
	ErrCodeQuotaExceeded    ErrorCode = "QUOTA_EXCEEDED"
	ErrCodeRateLimited      ErrorCode = "RATE_LIMITED"
	ErrCodePremiumRequired  ErrorCode = "PREMIUM_REQUIRED"

	// External API errors
	ErrCodeSpotifyAPI       ErrorCode = "SPOTIFY_API_ERROR"
	ErrCodeYouTubeAPI       ErrorCode = "YOUTUBE_API_ERROR"
	ErrCodeYouTubeQuota     ErrorCode = "YOUTUBE_QUOTA_EXCEEDED"
	ErrCodeGeniusAPI        ErrorCode = "GENIUS_API_ERROR"
	ErrCodeExternalAPI      ErrorCode = "EXTERNAL_API_ERROR"

	// Database errors
	ErrCodeDatabase         ErrorCode = "DATABASE_ERROR"
	ErrCodeDuplicateKey     ErrorCode = "DUPLICATE_KEY"

	// Internal errors
	ErrCodeInternal         ErrorCode = "INTERNAL_ERROR"
	ErrCodeTimeout          ErrorCode = "TIMEOUT"
	ErrCodeUnavailable      ErrorCode = "SERVICE_UNAVAILABLE"
)

// AppError represents an application error with context
type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Details    string                 `json:"details,omitempty"`
	StatusCode int                    `json:"-"`
	Err        error                  `json:"-"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap implements error unwrapping for errors.Is and errors.As
func (e *AppError) Unwrap() error {
	return e.Err
}

// WithDetails adds additional details to the error
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// WithMeta adds metadata to the error
func (e *AppError) WithMeta(key string, value interface{}) *AppError {
	if e.Meta == nil {
		e.Meta = make(map[string]interface{})
	}
	e.Meta[key] = value
	return e
}

// New creates a new AppError
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: getDefaultStatusCode(code),
	}
}

// Wrap wraps an existing error with context
func Wrap(err error, code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		Err:        err,
		StatusCode: getDefaultStatusCode(code),
	}
}

// Is checks if an error is a specific AppError code
func Is(err error, code ErrorCode) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}

// getDefaultStatusCode maps error codes to HTTP status codes
func getDefaultStatusCode(code ErrorCode) int {
	switch code {
	case ErrCodeUnauthorized, ErrCodeInvalidToken, ErrCodeTokenExpired:
		return http.StatusUnauthorized
	case ErrCodeForbidden, ErrCodePremiumRequired:
		return http.StatusForbidden
	case ErrCodeNotFound:
		return http.StatusNotFound
	case ErrCodeAlreadyExists, ErrCodeConflict, ErrCodeDuplicateKey:
		return http.StatusConflict
	case ErrCodeValidation, ErrCodeInvalidInput, ErrCodeMissingField, ErrCodeInvalidFormat:
		return http.StatusBadRequest
	case ErrCodeExceedsLimit, ErrCodeQuotaExceeded:
		return http.StatusUnprocessableEntity
	case ErrCodeRateLimited:
		return http.StatusTooManyRequests
	case ErrCodeYouTubeQuota:
		return http.StatusServiceUnavailable
	case ErrCodeTimeout, ErrCodeUnavailable:
		return http.StatusServiceUnavailable
	case ErrCodeSpotifyAPI, ErrCodeYouTubeAPI, ErrCodeGeniusAPI, ErrCodeExternalAPI:
		return http.StatusBadGateway
	case ErrCodeDatabase, ErrCodeInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Common error constructors for frequently used errors

func NotFound(resource string) *AppError {
	return New(ErrCodeNotFound, fmt.Sprintf("%s not found", resource))
}

func Unauthorized(message string) *AppError {
	return New(ErrCodeUnauthorized, message)
}

func Forbidden(message string) *AppError {
	return New(ErrCodeForbidden, message)
}

func Validation(message string) *AppError {
	return New(ErrCodeValidation, message)
}

func QuotaExceeded(message string) *AppError {
	return New(ErrCodeQuotaExceeded, message)
}

func PremiumRequired(feature string) *AppError {
	return New(ErrCodePremiumRequired, fmt.Sprintf("%s is a premium feature", feature))
}

func YouTubeQuotaExceeded() *AppError {
	return New(ErrCodeYouTubeQuota, "YouTube API quota exceeded. Please try again later.")
}

func Internal(message string) *AppError {
	return New(ErrCodeInternal, message)
}

func Database(err error) *AppError {
	return Wrap(err, ErrCodeDatabase, "Database operation failed")
}

// IsYouTubeAPIError checks if an error is a YouTube API error that shouldn't count against quota
func IsYouTubeAPIError(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == ErrCodeYouTubeAPI || appErr.Code == ErrCodeYouTubeQuota
	}
	return false
}
