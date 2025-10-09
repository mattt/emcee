package internal

import (
	"crypto/tls"
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

// RetryableClientOptions configures the retryable HTTP client.
type RetryableClientOptions struct {
	Retries  int
	Timeout  time.Duration
	RPS      int
	Logger   interface{}
	Insecure bool
}

// RetryableClient returns a new http.Client with a retryablehttp.Client configured per opts.
func RetryableClient(opts RetryableClientOptions) (*http.Client, error) {
	if opts.Retries < 0 {
		return nil, fmt.Errorf("retries must be greater than 0")
	}
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("timeout must be greater than 0")
	}
	if opts.RPS < 0 {
		return nil, fmt.Errorf("rps must be greater than 0")
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = opts.Retries
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 30 * time.Second
	retryClient.HTTPClient.Timeout = opts.Timeout
	retryClient.Logger = opts.Logger
	if opts.Insecure {
		// Clone the default transport to preserve defaults (pooling, timeouts, proxies), then override TLS.
		if base, ok := http.DefaultTransport.(*http.Transport); ok && base != nil {
			transport := base.Clone()
			if transport.TLSClientConfig == nil {
				transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			} else {
				transport.TLSClientConfig = transport.TLSClientConfig.Clone()
				transport.TLSClientConfig.InsecureSkipVerify = true
			}
			retryClient.HTTPClient.Transport = transport
		} else {
			// Fallback: construct a new transport if default transport type is unexpected.
			retryClient.HTTPClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		}
	}
	if opts.RPS > 0 {
		retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
			// Ensure we wait at least 1/rps between requests
			minWait := time.Second / time.Duration(opts.RPS)
			if min < minWait {
				min = minWait
			}
			return retryablehttp.DefaultBackoff(min, max, attemptNum, resp)
		}
	}

	return retryClient.StandardClient(), nil
}
