package errors

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ErrorResponse represents the JSON error response sent to clients
type ErrorResponse struct {
	Code    ErrorCode              `json:"code"`
	Message string                 `json:"message"`
	Details string                 `json:"details,omitempty"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

// ToHTTPError converts an AppError to an Echo HTTPError
func ToHTTPError(err error) error {
	var appErr *AppError
	if errors.As(err, &appErr) {
		// Log internal errors for debugging
		if appErr.StatusCode >= 500 {
			log.Printf("Internal error: %v", appErr)
		}

		response := ErrorResponse{
			Code:    appErr.Code,
			Message: appErr.Message,
			Details: appErr.Details,
			Meta:    appErr.Meta,
		}

		return echo.NewHTTPError(appErr.StatusCode, response)
	}

	// Unknown error - log and return generic internal error
	log.Printf("Unknown error: %v", err)
	return echo.NewHTTPError(http.StatusInternalServerError, ErrorResponse{
		Code:    ErrCodeInternal,
		Message: "An internal error occurred",
	})
}

// HandleError is a middleware-style handler that converts AppErrors to HTTP responses
func HandleError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}
	return ToHTTPError(err)
}
