package btp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyma-project/auditlog-manager/internal/btp/auth"
)

func Test_Transport_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("client bearer authorization", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer token" {
				t.Errorf("expected Authorization header 'Bearer token', got '%s'", got)
			}
		}))
		defer svr.Close()

		req, err := http.NewRequest(http.MethodPost, svr.URL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		clientTransport := auth.Transport{
			Token: &auth.Token{
				AccessToken: "token",
			},
		}
		_, err = clientTransport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip failed: %v", err)
		}
	})

	t.Run("client with nil token", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("expected no Authorization header, got '%s'", got)
			}
		}))
		defer svr.Close()

		req, err := http.NewRequest(http.MethodPost, svr.URL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		clientTransport := auth.Transport{Token: nil}
		_, err = clientTransport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip failed: %v", err)
		}
	})
}

func Test_GenericRequest(t *testing.T) {
	t.Parallel()

	testEmptyServer := httptest.NewServer(http.HandlerFunc(fixGenericRequestHandler(t, RequestOptions{})))
	defer testEmptyServer.Close()

	testServer := httptest.NewServer(http.HandlerFunc(fixGenericRequestHandler(t, fixRequestOptions())))
	defer testServer.Close()

	testErrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(415)
	}))
	defer testErrorServer.Close()

	t.Run("simple GET request", func(t *testing.T) {
		c := &Client{httpClient: http.DefaultClient}

		response, err := c.genericRequest(context.Background(), http.MethodGet, testEmptyServer.URL, RequestOptions{})

		if err != nil {
			t.Fatalf("genericRequest failed: %v", err)
		}
		if response == nil {
			t.Fatal("expected response, got nil")
		}
		if response.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", response.StatusCode)
		}
	})

	t.Run("simple POST request with additional data", func(t *testing.T) {
		c := &Client{httpClient: http.DefaultClient}

		response, err := c.genericRequest(context.Background(), http.MethodPost, testServer.URL, fixRequestOptions())

		if err != nil {
			t.Fatalf("genericRequest failed: %v", err)
		}
		if response == nil {
			t.Fatal("expected response, got nil")
		}
		if response.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", response.StatusCode)
		}
	})

	t.Run("build request error because of wrong method name", func(t *testing.T) {
		c := &Client{httpClient: http.DefaultClient}

		response, err := c.genericRequest(context.Background(), "DoEsNoTeXiSt)", testServer.URL, RequestOptions{})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to build request") {
			t.Errorf("expected 'failed to build request' error, got: %v", err)
		}
		if response != nil {
			t.Errorf("expected nil response, got %v", response)
		}
	})

	t.Run("can't reach server by URL error", func(t *testing.T) {
		c := &Client{httpClient: http.DefaultClient}

		response, err := c.genericRequest(context.Background(), http.MethodGet, "does-not-exist", RequestOptions{})

		expectedErr := "failed to get data from server: Get \"does-not-exist\": unsupported protocol scheme \"\""
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got: %v", expectedErr, err)
		}
		if response != nil {
			t.Errorf("expected nil response, got %v", response)
		}
	})

	t.Run("handle 415 response status", func(t *testing.T) {
		c := &Client{httpClient: http.DefaultClient}

		response, err := c.genericRequest(context.Background(), http.MethodGet, testErrorServer.URL, RequestOptions{})

		expectedErr := "request failed with status 415 Unsupported Media Type"
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got: %v", expectedErr, err)
		}
		if response == nil {
			t.Fatal("expected response even with error, got nil")
		}
	})
}

func Test_Client_buildResponseError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    *http.Response
		body        []byte
		expectedErr error
	}{
		{
			name: "build error from status",
			response: &http.Response{
				Status:     "Unauthorized",
				StatusCode: 401,
				Header:     http.Header{},
			},
			body:        []byte{},
			expectedErr: errors.New("request failed with status Unauthorized"),
		},
		{
			name: "build error from body",
			response: &http.Response{
				Status:     "Unauthorized",
				StatusCode: 401,
				Header:     http.Header{},
			},
			body: []byte(`{
				"error": {
					"code": 123,
					"message": "message",
					"target": "target",
					"correlationID": "correlationID"
				}
			}`),
			expectedErr: errors.New("message"),
		},
		{
			name: "decode response error",
			response: &http.Response{
				Status:     "Unauthorized",
				StatusCode: 401,
				Header:     http.Header{},
			},
			body:        []byte("[test=value]"),
			expectedErr: errors.New("request failed with status Unauthorized"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{}

			err := c.buildResponseError(tt.response, tt.body)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.expectedErr.Error() {
				t.Errorf("expected error '%v', got '%v'", tt.expectedErr, err)
			}
		})
	}
}

func Test_Get(t *testing.T) {
	t.Parallel()

	token := &auth.Token{AccessToken: "test-token"}

	t.Run("successful GET request", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("expected Authorization header 'Bearer test-token', got '%s'", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		defer svr.Close()

		client := NewClient(0, token)
		resp, err := client.Get(context.Background(), svr.URL, RequestOptions{})

		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if !strings.Contains(string(resp.Body), "ok") {
			t.Errorf("unexpected body: %s", string(resp.Body))
		}
	})
}

func Test_Post(t *testing.T) {
	t.Parallel()

	token := &auth.Token{AccessToken: "test-token"}

	t.Run("successful POST request", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("expected Authorization header 'Bearer test-token', got '%s'", got)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got '%s'", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "test") {
				t.Errorf("unexpected body: %s", string(body))
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created"}`))
		}))
		defer svr.Close()

		client := NewClient(0, token)
		resp, err := client.Post(context.Background(), svr.URL, RequestOptions{
			Body:    strings.NewReader(`{"test":"data"}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		})

		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", resp.StatusCode)
		}
	})
}

func Test_Delete(t *testing.T) {
	t.Parallel()

	token := &auth.Token{AccessToken: "test-token"}

	t.Run("successful DELETE request", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("expected Authorization header 'Bearer test-token', got '%s'", got)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer svr.Close()

		client := NewClient(0, token)
		resp, err := client.Delete(context.Background(), svr.URL, RequestOptions{})

		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected status 204, got %d", resp.StatusCode)
		}
	})
}

func Test_Put(t *testing.T) {
	t.Parallel()

	token := &auth.Token{AccessToken: "test-token"}

	t.Run("successful PUT request", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				t.Errorf("expected PUT, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("expected Authorization header 'Bearer test-token', got '%s'", got)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("expected Content-Type 'application/json', got '%s'", got)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "test") {
				t.Errorf("unexpected body: %s", string(body))
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"accepted"}`))
		}))
		defer svr.Close()

		client := NewClient(0, token)
		resp, err := client.Put(context.Background(), svr.URL, RequestOptions{
			Body:    strings.NewReader(`{"test":"data"}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		})

		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected status 202, got %d", resp.StatusCode)
		}
		if !strings.Contains(string(resp.Body), "accepted") {
			t.Errorf("unexpected body: %s", string(resp.Body))
		}
	})

	t.Run("PUT request with error response", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"insufficient permissions"}}`))
		}))
		defer svr.Close()

		client := NewClient(0, token)
		resp, err := client.Put(context.Background(), svr.URL, RequestOptions{
			Body:    strings.NewReader(`{"test":"data"}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if resp == nil {
			t.Fatal("expected response even with error, got nil")
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", resp.StatusCode)
		}
		if !strings.Contains(err.Error(), "insufficient permissions") {
			t.Errorf("expected error to contain 'insufficient permissions', got: %v", err)
		}
	})
}

func fixRequestOptions() RequestOptions {
	return RequestOptions{
		Body: strings.NewReader("test data"),
		Headers: map[string]string{
			"Test-Header": "test-header-value",
		},
		Query: map[string]string{
			"test-query": "test-query-value",
		},
	}
}

func fixGenericRequestHandler(t *testing.T, expectedOptions RequestOptions) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		for key, expectedValue := range expectedOptions.Query {
			values, ok := r.URL.Query()[key]
			if !ok {
				t.Errorf("expected query param '%s' not found", key)
			} else if len(values) == 0 || values[0] != expectedValue {
				t.Errorf("expected query param '%s'='%s', got '%s'", key, expectedValue, values[0])
			}
		}

		for key, expectedValue := range expectedOptions.Headers {
			values, ok := r.Header[key]
			if !ok {
				t.Errorf("expected header '%s' not found", key)
			} else if len(values) == 0 || values[0] != expectedValue {
				t.Errorf("expected header '%s'='%s', got '%s'", key, expectedValue, values[0])
			}
		}

		data := make([]byte, 0)
		expectedData := make([]byte, 0)
		var err error
		if r.Body != nil {
			data, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
		}
		if expectedOptions.Body != nil {
			expectedData, err = io.ReadAll(expectedOptions.Body)
			if err != nil {
				t.Fatalf("failed to read expected body: %v", err)
			}
		}
		if string(data) != string(expectedData) {
			t.Errorf("expected body '%s', got '%s'", string(expectedData), string(data))
		}

		w.WriteHeader(200)
	}
}
