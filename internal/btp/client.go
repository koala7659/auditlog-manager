package btp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP calls to BTP CIS APIs and token endpoints
type Client struct {
	httpClient *http.Client
	token      *XSUAAToken
}

// NewClient creates a client with the provided timeout and token
func NewClient(timeout time.Duration, token *XSUAAToken) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &oauthTransport{
				token: token,
			},
		},
		token: token,
	}
}

// NewClientWithHTTPClient creates a client from an existing HTTP client (useful for tests)
func NewClientWithHTTPClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{httpClient: httpClient}
}

// oauthTransport adds OAuth bearer token to requests
type oauthTransport struct {
	token *XSUAAToken
}

func (t *oauthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.token != nil && t.token.AccessToken != "" {
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.token.AccessToken))
	}
	return http.DefaultTransport.RoundTrip(r)
}

// CISError represents a BTP CIS error response
type CISError struct {
	Code          int    `json:"code"`
	Message       string `json:"message"`
	Target        string `json:"target"`
	CorrelationID string `json:"correlationID"`
}

// CISErrorResponse wraps the error structure from BTP
type CISErrorResponse struct {
	Error CISError `json:"error"`
}

// APIResponse captures response data for inspection
type APIResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// RequestOptions holds optional parameters for API requests
type RequestOptions struct {
	Body    io.Reader
	Headers map[string]string
	Query   map[string]string
}

// Get executes a GET request
func (c *Client) Get(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodGet, url, options)
}

// Post executes a POST request
func (c *Client) Post(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodPost, url, options)
}

// Delete executes a DELETE request
func (c *Client) Delete(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodDelete, url, options)
}

// Put executes a PUT request
func (c *Client) Put(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodPut, url, options)
}

// genericRequest executes an HTTP request with full options support
func (c *Client) genericRequest(ctx context.Context, method, endpoint string, options RequestOptions) (*APIResponse, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, options.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	// Set headers
	for k, v := range options.Headers {
		req.Header.Set(k, v)
	}

	// Set query parameters
	if len(options.Query) > 0 {
		q := req.URL.Query()
		for key, value := range options.Query {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get data from server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	apiResp := &APIResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       respBody,
	}

	// Handle error responses (status >= 400)
	if resp.StatusCode >= 400 {
		return apiResp, c.buildResponseError(resp, respBody)
	}

	return apiResp, nil
}

// DoRequest executes an API request with optional bearer token and headers (legacy method for backwards compatibility)
func (c *Client) DoRequest(ctx context.Context, method, endpoint string, body []byte, headers map[string]string, bearerToken string) (*APIResponse, error) {
	opts := RequestOptions{
		Headers: headers,
	}
	if len(body) > 0 {
		opts.Body = bytes.NewReader(body)
		if headers == nil || headers["Content-Type"] == "" {
			if opts.Headers == nil {
				opts.Headers = make(map[string]string)
			}
			opts.Headers["Content-Type"] = "application/json"
		}
	}

	// Create a temporary client with the bearer token if provided
	if bearerToken != "" && bearerToken != c.token.AccessToken {
		tempClient := &Client{
			httpClient: &http.Client{
				Timeout: c.httpClient.Timeout,
				Transport: &oauthTransport{
					token: &XSUAAToken{AccessToken: bearerToken},
				},
			},
			token: &XSUAAToken{AccessToken: bearerToken},
		}
		return tempClient.genericRequest(ctx, method, endpoint, opts)
	}

	return c.genericRequest(ctx, method, endpoint, opts)
}

// buildResponseError constructs an error from HTTP response details
func (c *Client) buildResponseError(response *http.Response, body []byte) error {
	// Try to parse as CIS error response
	var cisErr CISErrorResponse
	if err := json.Unmarshal(body, &cisErr); err == nil && cisErr.Error.Message != "" {
		return errors.New(cisErr.Error.Message)
	}

	// Check for EOF (error in headers)
	if len(body) == 0 {
		return c.buildErrorFromHeaders(response)
	}

	// Fallback to generic error
	return fmt.Errorf("failed to decode error response with status '%s'", response.Status)
}

// buildErrorFromHeaders extracts error from WWW-Authenticate header
func (c *Client) buildErrorFromHeaders(response *http.Response) error {
	wwwAuthHeaderString := response.Header.Get("Www-Authenticate")
	if wwwAuthHeaderString == "" {
		return fmt.Errorf("failed to parse http error for status: %s", response.Status)
	}

	wwwAuthHeader := ParseAuthSettings(wwwAuthHeaderString)
	errType := wwwAuthHeader.Params["error"]
	errDesc := wwwAuthHeader.Params["error_description"]

	if errDesc != "" {
		return fmt.Errorf("%s: %s", errType, errDesc)
	}
	return fmt.Errorf("%s", errType)
}

// PrettyJSON renders JSON as indented text when possible
func PrettyJSON(raw []byte) (string, bool) {
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", false
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", false
	}
	return string(out), true
}

// ParseAuthSettings parses WWW-Authenticate header
// Adapted from: https://github.com/gboddin/go-www-authenticate-parser
func ParseAuthSettings(digestBuffer string) *WWWAuthenticateSettings {
	digest := &WWWAuthenticateSettings{
		digestBuffer: bytes.NewBufferString(digestBuffer),
		buffer:       "",
		AuthType:     "",
		Params:       make(map[string]string),
	}
	digest.state = digest.ParseType
	for {
		if err := digest.state(); err != nil {
			break
		}
	}
	return digest
}

// WWWAuthenticateSettings represents parsed WWW-Authenticate header
type WWWAuthenticateSettings struct {
	state        func() error
	digestBuffer *bytes.Buffer
	buffer       string
	currentParam string
	quoteOpened  bool
	AuthType     string
	Params       map[string]string
}

// ParseType parses the authentication type
func (d *WWWAuthenticateSettings) ParseType() error {
	currentByte, err := d.digestBuffer.ReadByte()
	if err != nil {
		return err
	}
	if currentByte != ' ' && currentByte != '\n' {
		d.buffer += string(currentByte)
		return nil
	}
	d.AuthType = d.buffer
	d.buffer = ""
	d.state = d.ParseParamKey
	return nil
}

// ParseParamKey parses parameter keys
func (d *WWWAuthenticateSettings) ParseParamKey() error {
	currentByte, err := d.digestBuffer.ReadByte()
	if err != nil {
		return err
	}
	switch currentByte {
	case '=':
		d.currentParam = d.buffer
		d.buffer = ""
		d.state = d.ParseParamValue
		return nil
	case ' ':
		if len(d.buffer) > 0 {
			d.Params[d.buffer] = "true"
			d.currentParam = ""
			d.buffer = ""
			d.state = d.ParseParamKey
		}
		return nil
	case ',':
		if len(d.buffer) > 0 {
			d.Params[d.buffer] = "true"
		}
		d.currentParam = ""
		d.buffer = ""
		d.state = d.ParseParamKey
		return nil
	}
	d.buffer += string(currentByte)
	return nil
}

// ParseParamValue parses parameter values
func (d *WWWAuthenticateSettings) ParseParamValue() error {
	currentByte, err := d.digestBuffer.ReadByte()
	if err != nil {
		return err
	}
	switch currentByte {
	case '\\':
		nextByte, err := d.digestBuffer.ReadByte()
		if err != nil {
			return err
		}
		var unquoted string
		err = json.Unmarshal([]byte("\""+string(currentByte)+string(nextByte)+"\""), &unquoted)
		if err != nil {
			return err
		}
		d.buffer += unquoted
		return nil
	case '"':
		if d.quoteOpened {
			d.quoteOpened = false
			d.Params[d.currentParam] = d.buffer
			d.currentParam = ""
			d.buffer = ""
			_, err = d.digestBuffer.ReadString(',')
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			d.state = d.ParseParamKey
			return nil
		}
		if !d.quoteOpened {
			d.quoteOpened = true
			return nil
		}
	case ',':
		if !d.quoteOpened {
			d.quoteOpened = false
			d.Params[d.currentParam] = d.buffer
			d.currentParam = ""
			d.buffer = ""
			d.state = d.ParseParamKey
			return nil
		}
	}

	d.buffer += string(currentByte)
	return nil
}
