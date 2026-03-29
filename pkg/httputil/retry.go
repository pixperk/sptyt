package httputil

import (
	"context"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"time"
)

// DoWithRetry executes an HTTP request with exponential backoff.
// Retries on 429 and 5xx status codes, up to maxRetries times.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			// Exponential backoff with jitter: 500ms * 2^attempt + random jitter
			backoff := time.Duration(float64(500*time.Millisecond) * math.Pow(2, float64(attempt-1)))
			jitter := time.Duration(rand.Int64N(int64(backoff / 2)))
			backoff += jitter

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}

			// Reset body for retry if possible
			if req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return nil, bodyErr
				}
				req.Body = body
			}
		}

		resp, err = client.Do(req)
		if err != nil {
			continue
		}

		// Don't retry on success or client errors (except 429)
		if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		// Drain and close body before retry
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	return resp, err
}
