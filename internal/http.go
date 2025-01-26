package internal

import (
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// HeaderTransport is a custom RoundTripper that adds default headers to requests
type HeaderTransport struct {
	Base    http.RoundTripper
	Headers http.Header
}

// RoundTrip adds the default headers to the request
func (t *HeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, values := range t.Headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// RetryableClient returns a new http.Client with a retryablehttp.Client
// configured with the provided parameters.
func RetryableClient(retries int, timeout time.Duration, rps int, logger interface{}) (*http.Client, error) {
	if retries < 0 {
		return nil, fmt.Errorf("retries must be greater than 0")
	}
	if timeout < 0 {
		return nil, fmt.Errorf("timeout must be greater than 0")
	}
	if rps < 0 {
		return nil, fmt.Errorf("rps must be greater than 0")
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = retries
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.HTTPClient.Timeout = timeout
	retryClient.Logger = logger
	if rps > 0 {
		retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
			// Ensure we wait at least 1/rps between requests
			minWait := time.Second / time.Duration(rps)
			if min < minWait {
				min = minWait
			}
			return retryablehttp.DefaultBackoff(min, max, attemptNum, resp)
		}
	}

	return retryClient.StandardClient(), nil
}
