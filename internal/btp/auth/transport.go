package auth

import (
	"fmt"
	"net/http"
)

// Transport is an HTTP RoundTripper that adds OAuth2 bearer token to requests.
type Transport struct {
	Token     *Token
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper by adding the OAuth2 bearer token.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Token != nil && t.Token.AccessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.Token.AccessToken))
	}

	transport := t.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return transport.RoundTrip(req)
}
