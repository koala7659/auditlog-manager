package btp

import (
	"encoding/json"
	"fmt"
	"os"
)

// Credentials represents the structure of BTP credentials file
type Credentials struct {
	Endpoints map[string]string `json:"endpoints"`
	UAA       UAA               `json:"uaa"`
}

// UAA contains XSUAA authentication credentials
type UAA struct {
	APIurl            string `json:"apiurl"`
	ClientID          string `json:"clientid"`
	ClientSecret      string `json:"clientsecret"`
	CredentialType    string `json:"credential-type"`
	IdentityZone      string `json:"identityzone"`
	IdentityZoneID    string `json:"identityzoneid"`
	SBurl             string `json:"sburl"`
	ServiceInstanceID string `json:"serviceInstanceId"`
	SubAccountID      string `json:"subaccountid"`
	TenantID          string `json:"tenantid"`
	TenantMode        string `json:"tenantmode"`
	UAADomain         string `json:"uaadomain"`
	URL               string `json:"url"`
	VerificationKey   string `json:"verificationkey"`
	XSAppname         string `json:"xsappname"`
	XSMasterAppName   string `json:"xsmasterappname"`
	ZoneID            string `json:"zoneid"`
}

// ServiceManagerCredentials represents Service Manager binding credentials
type ServiceManagerCredentials struct {
	ClientID     string `json:"clientid"`
	ClientSecret string `json:"clientsecret"`
	SMURL        string `json:"sm_url"`
	TokenURL     string `json:"url"`
	UAAdomain    string `json:"uaadomain"`
	XSAppname    string `json:"xsappname"`
}

// LoadCredentials loads and parses BTP credentials from a file
func LoadCredentials(path string) (*Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	// Validate required fields
	if err := validateCredentials(&creds); err != nil {
		return nil, fmt.Errorf("invalid credentials: %w", err)
	}

	return &creds, nil
}

// validateCredentials checks that required credential fields are present
func validateCredentials(creds *Credentials) error {
	if creds.UAA.URL == "" {
		return fmt.Errorf("uaa.url is required")
	}
	if creds.UAA.ClientID == "" {
		return fmt.Errorf("uaa.clientid is required")
	}
	if creds.UAA.ClientSecret == "" {
		return fmt.Errorf("uaa.clientsecret is required")
	}
	if creds.Endpoints == nil || len(creds.Endpoints) == 0 {
		return fmt.Errorf("endpoints are required")
	}
	if creds.Endpoints["accounts_service_url"] == "" {
		return fmt.Errorf("endpoints.accounts_service_url is required")
	}

	return nil
}
