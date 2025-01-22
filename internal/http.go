package internal

import "net/http"

// HeaderTransport is a custom RoundTripper that adds default headers to requests
type HeaderTransport struct {
	Base    http.RoundTripper
	Headers http.Header
}

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
