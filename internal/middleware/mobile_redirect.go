package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// MobileAppRedirect detects mobile devices and redirects to native apps
func MobileAppRedirect() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip if it's the home page
			if c.Request().URL.Path == "/" || strings.HasPrefix(c.Request().URL.Path, "/static") {
				return next(c)
			}

			userAgent := c.Request().UserAgent()
			isMobile := isMobileDevice(userAgent)

			if isMobile {
				// Store mobile flag in context for handlers to use
				c.Set("is_mobile", true)
			}

			return next(c)
		}
	}
}

func isMobileDevice(userAgent string) bool {
	userAgent = strings.ToLower(userAgent)

	mobileKeywords := []string{
		"android",
		"iphone",
		"ipad",
		"ipod",
		"mobile",
		"webos",
		"blackberry",
		"windows phone",
	}

	for _, keyword := range mobileKeywords {
		if strings.Contains(userAgent, keyword) {
			return true
		}
	}

	return false
}
