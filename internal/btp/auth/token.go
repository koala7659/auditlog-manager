// Package auth provides OAuth2 authentication for SAP BTP XSUAA.
package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const authorizationEndpoint = "oauth/token"

// Token represents an OAuth2 access token from XSUAA.
type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
	JTI         string `json:"jti"`
}

type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// GetToken obtains an OAuth2 token using client credentials flow.
//
// Parameters:
//   - grantType: OAuth2 grant type (typically "client_credentials")
//   - serverURL: XSUAA server base URL
//   - clientID: OAuth2 client ID
//   - clientSecret: OAuth2 client secret
//
// Returns an access token or an error if authentication fails.
func GetToken(grantType, serverURL, clientID, clientSecret string) (*Token, error) {
	urlBody := url.Values{}
	urlBody.Set("grant_type", grantType)

	endpoint := fmt.Sprintf("%s/%s", serverURL, authorizationEndpoint)
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(urlBody.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to build authentication request: %w", err)
	}
	defer request.Body.Close()

	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(clientID, clientSecret)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("authentication request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, parseErrorResponse(response)
	}

	return parseSuccessResponse(response)
}

func parseSuccessResponse(response *http.Response) (*Token, error) {
	var token Token
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	return &token, nil
}

func parseErrorResponse(response *http.Response) error {
	var errResp errorResponse
	if err := json.NewDecoder(response.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("authentication failed with status %s", response.Status)
	}

	if errResp.ErrorDescription != "" {
		return fmt.Errorf("authentication failed (%s): %s", errResp.Error, errResp.ErrorDescription)
	}
	return fmt.Errorf("authentication failed: %s", errResp.Error)
}
