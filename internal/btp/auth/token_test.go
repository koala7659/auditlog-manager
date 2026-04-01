package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	correctClientID     = "user"
	correctClientSecret = "zaq12wsx"
	correctToken        = "kusedug"
	supportedGrantType  = "client_credentials"
)

func TestGetToken(t *testing.T) {
	t.Parallel()

	svrGood := httptest.NewServer(http.HandlerFunc(fixAuthenticationHandler(t)))
	defer svrGood.Close()
	svrBad := httptest.NewServer(http.HandlerFunc(fixAuthenticationErrorHandler(t)))
	defer svrBad.Close()

	tests := []struct {
		name           string
		grantType      string
		serverURL      string
		clientID       string
		clientSecret   string
		want           *Token
		expectedErrMsg string
	}{
		{
			name:           "Correct credentials",
			grantType:      supportedGrantType,
			serverURL:      svrGood.URL,
			clientID:       correctClientID,
			clientSecret:   correctClientSecret,
			want:           &Token{AccessToken: correctToken},
			expectedErrMsg: "",
		},
		{
			name:           "Incorrect URL",
			grantType:      supportedGrantType,
			serverURL:      "?\n?",
			clientID:       correctClientID,
			clientSecret:   correctClientSecret,
			want:           nil,
			expectedErrMsg: "failed to build authentication request",
		},
		{
			name:           "Wrong URL",
			grantType:      supportedGrantType,
			serverURL:      "doesnotexist",
			clientID:       correctClientID,
			clientSecret:   correctClientSecret,
			want:           nil,
			expectedErrMsg: "authentication request failed",
		},
		{
			name:           "Error response",
			grantType:      supportedGrantType,
			serverURL:      svrBad.URL,
			clientID:       correctClientID,
			clientSecret:   correctClientSecret,
			want:           nil,
			expectedErrMsg: "authentication failed (error): description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetToken(
				tt.grantType,
				tt.serverURL,
				tt.clientID,
				tt.clientSecret,
			)

			if tt.expectedErrMsg == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if got == nil {
					t.Fatal("expected token, got nil")
				}
				if got.AccessToken != tt.want.AccessToken {
					t.Errorf("expected access token '%s', got '%s'", tt.want.AccessToken, got.AccessToken)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error containing '%s', got nil", tt.expectedErrMsg)
				}
				if !strings.Contains(err.Error(), tt.expectedErrMsg) {
					t.Errorf("expected error containing '%s', got: %v", tt.expectedErrMsg, err)
				}
				if got != nil {
					t.Errorf("expected nil token, got %v", got)
				}
			}
		})
	}
}

func TestGetToken_ResponseDecoding(t *testing.T) {
	t.Parallel()

	t.Run("Invalid JSON response", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("not json"))
		}))
		defer svr.Close()

		got, err := GetToken(
			supportedGrantType,
			svr.URL,
			correctClientID,
			correctClientSecret,
		)

		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse token response") {
			t.Errorf("expected 'failed to parse token response' error, got: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil token, got %v", got)
		}
	})

	t.Run("Token type defaults to Bearer", func(t *testing.T) {
		svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"access_token":"token123"}`))
		}))
		defer svr.Close()

		got, err := GetToken(
			supportedGrantType,
			svr.URL,
			correctClientID,
			correctClientSecret,
		)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got == nil {
			t.Fatal("expected token, got nil")
		}
		if got.AccessToken != "token123" {
			t.Errorf("expected access token 'token123', got '%s'", got.AccessToken)
		}
	})
}

func fixAuthenticationHandler(t *testing.T) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/%s", authorizationEndpoint) {
			w.WriteHeader(404)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth")
			w.WriteHeader(401)
			return
		}
		if username != correctClientID {
			t.Errorf("expected username '%s', got '%s'", correctClientID, username)
		}
		if password != correctClientSecret {
			t.Errorf("expected password '%s', got '%s'", correctClientSecret, password)
		}

		data := Token{
			AccessToken: correctToken,
		}
		response, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}

		w.WriteHeader(200)
		_, err = w.Write(response)
		if err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}
}

func fixAuthenticationErrorHandler(t *testing.T) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		data := errorResponse{
			Error:            "error",
			ErrorDescription: "description",
		}
		response, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("failed to marshal error response: %v", err)
		}

		w.WriteHeader(401)
		_, err = w.Write(response)
		if err != nil {
			t.Fatalf("failed to write error response: %v", err)
		}
	}
}
