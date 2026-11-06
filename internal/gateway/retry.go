package gateway

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxAttempts          = 3
	baseRetryDelay       = time.Second
	maxRetryAfterSeconds = 60
)

// withRetry runs a GET attempt up to maxAttempts times while it fails with
// 502, 503, 504 or a network error.
func (c *Client) withRetry(ctx context.Context, path string, attempt func() (*Response, error)) (*Response, error) {
	for n := 1; ; n++ {
		resp, err := attempt()
		if n == maxAttempts || !retryable(resp, err) {
			return resp, err
		}
		var retryAfter string
		if resp != nil {
			retryAfter = resp.header.Get("Retry-After")
		}
		delay := c.retryDelay(n, retryAfter)
		c.log.Debug("GET %s failed (%v), retrying in %v", path, describeFailure(resp, err), delay)
		if err := c.sleep(ctx, delay); err != nil {
			return nil, newTransportError(http.MethodGet, path, c.host, err, false, true)
		}
	}
}

func retryable(resp *Response, err error) bool {
	if err != nil {
		var gerr *Error
		return errors.As(err, &gerr) && gerr.Kind == Network
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay honors Retry-After in seconds, capped at one minute; otherwise
// it doubles the base delay per attempt and adds up to 50 percent jitter.
func (c *Client) retryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds >= 0 {
		return time.Duration(min(seconds, maxRetryAfterSeconds)) * time.Second
	}
	delay := baseRetryDelay << (attempt - 1)
	return delay + time.Duration(c.random()*0.5*float64(delay))
}

func describeFailure(resp *Response, err error) any {
	if err != nil {
		return err
	}
	return "status " + strconv.Itoa(resp.StatusCode)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
