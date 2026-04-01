package btp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kyma-project/auditlog-manager/internal/btp/auth"
)

// Client provides authenticated HTTP access to BTP CIS APIs.
// It automatically includes OAuth2 bearer tokens in all requests.
type Client struct {
	httpClient *http.Client
	token      *auth.Token
}

// NewClient creates an authenticated HTTP client for BTP API requests.
// The client automatically includes the OAuth2 bearer token in all requests.
func NewClient(timeout time.Duration, token *auth.Token) *Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &auth.Transport{
				Token: token,
			},
		},
		token: token,
	}
}

// NewClientWithHTTPClient creates a client from an existing HTTP client for testing.
//func NewClientWithHTTPClient(httpClient *http.Client) *Client {
//	if httpClient == nil {
//		httpClient = &http.Client{Timeout: 30 * time.Second}
//	}
//	return &Client{httpClient: httpClient}
//}

// CISError represents a structured error response from BTP CIS APIs.
type CISError struct {
	Code          int    `json:"code"`
	Message       string `json:"message"`
	Target        string `json:"target"`
	CorrelationID string `json:"correlationID"`
}

// CISErrorResponse wraps a CIS error for parsing from JSON responses.
type CISErrorResponse struct {
	Error CISError `json:"error"`
}

// APIResponse encapsulates HTTP response data from BTP APIs.
type APIResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// RequestOptions holds optional parameters for HTTP requests.
type RequestOptions struct {
	Body    io.Reader
	Headers map[string]string
	Query   map[string]string
}

func (c *Client) Get(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodGet, url, options)
}

func (c *Client) Post(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodPost, url, options)
}

func (c *Client) Delete(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodDelete, url, options)
}

func (c *Client) Put(ctx context.Context, url string, options RequestOptions) (*APIResponse, error) {
	return c.genericRequest(ctx, http.MethodPut, url, options)
}

func (c *Client) genericRequest(ctx context.Context, method, endpoint string, options RequestOptions) (*APIResponse, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, options.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	for k, v := range options.Headers {
		req.Header.Set(k, v)
	}

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

	if resp.StatusCode >= 400 {
		return apiResp, c.buildResponseError(resp, respBody)
	}

	return apiResp, nil
}

func (c *Client) buildResponseError(response *http.Response, body []byte) error {
	var cisErr CISErrorResponse
	if err := json.Unmarshal(body, &cisErr); err == nil && cisErr.Error.Message != "" {
		return errors.New(cisErr.Error.Message)
	}

	if len(body) == 0 {
		return fmt.Errorf("request failed with status %s", response.Status)
	}

	return fmt.Errorf("request failed with status %s", response.Status)
}
