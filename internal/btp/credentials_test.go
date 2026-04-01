package btp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCredentials(t *testing.T) {
	t.Parallel()

	t.Run("valid credentials file", func(t *testing.T) {
		tmpDir := t.TempDir()
		credFile := filepath.Join(tmpDir, "credentials.json")

		content := `{
			"endpoints": {
				"accounts_service_url": "https://accounts.example.com"
			},
			"uaa": {
				"url": "https://uaa.example.com",
				"clientid": "test-client",
				"clientsecret": "test-secret",
				"subaccountid": "test-subaccount",
				"tenantid": "test-tenant"
			}
		}`

		if err := os.WriteFile(credFile, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		creds, err := LoadCredentials(credFile)

		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if creds == nil {
			t.Fatal("expected credentials, got nil")
		}
		if creds.UAA.ClientID != "test-client" {
			t.Errorf("expected ClientID 'test-client', got '%s'", creds.UAA.ClientID)
		}
		if creds.UAA.ClientSecret != "test-secret" {
			t.Errorf("expected ClientSecret 'test-secret', got '%s'", creds.UAA.ClientSecret)
		}
		if creds.Endpoints["accounts_service_url"] != "https://accounts.example.com" {
			t.Errorf("expected accounts_service_url, got '%s'", creds.Endpoints["accounts_service_url"])
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		_, err := LoadCredentials("/nonexistent/credentials.json")

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to read credentials file") {
			t.Errorf("expected 'failed to read credentials file' error, got: %v", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		credFile := filepath.Join(tmpDir, "invalid.json")

		if err := os.WriteFile(credFile, []byte("not valid json"), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := LoadCredentials(credFile)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse credentials") {
			t.Errorf("expected 'failed to parse credentials' error, got: %v", err)
		}
	})

	t.Run("missing UAA URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		credFile := filepath.Join(tmpDir, "missing-url.json")

		content := `{
			"endpoints": {
				"accounts_service_url": "https://accounts.example.com"
			},
			"uaa": {
				"clientid": "test-client",
				"clientsecret": "test-secret"
			}
		}`

		if err := os.WriteFile(credFile, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := LoadCredentials(credFile)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "uaa.url is required") {
			t.Errorf("expected 'uaa.url is required' error, got: %v", err)
		}
	})

	t.Run("missing client credentials", func(t *testing.T) {
		tmpDir := t.TempDir()
		credFile := filepath.Join(tmpDir, "missing-creds.json")

		content := `{
			"endpoints": {
				"accounts_service_url": "https://accounts.example.com"
			},
			"uaa": {
				"url": "https://uaa.example.com"
			}
		}`

		if err := os.WriteFile(credFile, []byte(content), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := LoadCredentials(credFile)

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "uaa.clientid is required") {
			t.Errorf("expected 'uaa.clientid is required' error, got: %v", err)
		}
	})
}
