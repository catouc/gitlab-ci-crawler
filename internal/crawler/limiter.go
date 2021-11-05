package crawler

import (
	"net/http"

	"golang.org/x/time/rate"
)

type rateLimitedHTTPClient struct {
	Client      *http.Client
	RateLimiter *rate.Limiter
}

func (c *rateLimitedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	err := c.RateLimiter.Wait(req.Context())
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
