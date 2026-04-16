// Package btp provides a client for managing SAP BTP audit logging services.
//
// The package handles the complete lifecycle of audit log stacks including:
//   - Service Manager binding creation and management
//   - Audit log service instance provisioning
//   - Status verification and monitoring
//
// Example usage:
//
//	client, err := btp.NewBtpClient("/path/to/credentials.json")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create logging stack
//	err = client.CreateLoggingStack(ctx, tenantID)
//
//	// Verify status
//	status, err := client.VerifyLoggingStack(ctx, tenantID)
//
//	// Delete logging stack
//	err = client.DeleteLoggingStack(ctx, tenantID)
package btp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kyma-project/auditlog-manager/internal/btp/auth"
)

// InstallStatus represents the current state of an audit logging stack installation.
type InstallStatus string

const (
	// NoInstallationStatus indicates no audit log installation exists for the tenant.
	NoInstallationStatus InstallStatus = "NoInstallation"
	// InstallStatusReady indicates the audit log stack is fully operational.
	InstallStatusReady InstallStatus = "Ready"
	// InstallStatusProcessing indicates the audit log stack is being created or updated.
	InstallStatusProcessing InstallStatus = "Processing"
	// InstallStatusError indicates the audit log stack encountered an error.
	InstallStatusError InstallStatus = "Error"
)

// BTPClient provides operations for managing BTP audit logging stacks.
//
// All operations are idempotent and safe to retry. The client automatically
// handles Service Manager authentication and resource provisioning.
type BTPClient interface {
	// CreateLoggingStack creates a complete audit logging stack for the given tenant.
	// This includes Service Manager binding and both auditlog-management and auditlog instances.
	// The operation is idempotent - existing resources are reused.
	CreateLoggingStack(ctx context.Context, tenantID string) error

	// DeleteLoggingStack removes all audit logging resources for the given tenant.
	// This includes service instances, bindings, and the Service Manager binding.
	// Missing resources are silently ignored.
	DeleteLoggingStack(ctx context.Context, tenantID string) error

	// VerifyLoggingStack checks the current installation status for the given tenant.
	// Returns the current state and any errors encountered during verification.
	VerifyLoggingStack(ctx context.Context, tenantID string) (InstallStatus, error)

	// CreateSubaccount creates a new BTP subaccount in the specified region.
	// Returns the subaccount GUID on success.
	CreateSubaccount(ctx context.Context, region, globalAccountID, displayName string, administrators []string) (string, error)

	// GetSubaccount retrieves subaccount details for drift detection.
	// Returns true if the subaccount exists, along with its current state.
	GetSubaccount(ctx context.Context, subaccountGUID string) (exists bool, state string, err error)

	// DeleteSubaccount deletes a BTP subaccount by its GUID.
	DeleteSubaccount(ctx context.Context, subaccountGUID string) error
}

// btpClient implements BTPClient with authenticated access to BTP services.
type btpClient struct {
	client      *Client
	credentials *Credentials
	token       *auth.Token
	timeout     time.Duration
}

// NewBtpClient creates a new authenticated BTP client.
//
// The credentials file must contain:
//   - UAA authentication details (URL, client ID, client secret)
//   - BTP service endpoints (accounts service URL, etc.)
//
// Returns an error if the credentials file cannot be loaded or authentication fails.
func NewBtpClient(credentialPath string, timeout time.Duration) (BTPClient, error) {
	creds, err := LoadCredentials(credentialPath)
	if err != nil {
		return nil, err
	}

	token, err := auth.GetToken(
		"client_credentials",
		creds.UAA.URL,
		creds.UAA.ClientID,
		creds.UAA.ClientSecret,
	)
	if err != nil {
		return nil, err
	}

	httpClient := NewClient(timeout, token)

	return &btpClient{
		client:      httpClient,
		credentials: creds,
		token:       token,
		timeout:     timeout,
	}, nil
}

// CreateLoggingStack creates the audit logging stack for a given tenant.
// For ADR architecture: first creates subaccount, then provisions services.
func (c *btpClient) CreateLoggingStack(ctx context.Context, tenantID string) error {
	// Step 1: Create BTP subaccount if it doesn't exist
	// Note: In ADR architecture, subaccountID should be passed explicitly
	// For now, using tenantID as subaccountID (to be refactored)
	subaccountID := tenantID

	// Step 2: Create Service Manager binding
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, subaccountID)
	if err != nil {
		return err
	}

	if !exists || smCreds == nil {
		smCreds, err = c.addServiceManagerBinding(ctx, subaccountID)
		if err != nil {
			return err
		}
	}

	// Step 3: Authenticate with Service Manager
	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return err
	}

	// Step 4: Create auditlog-management service instance + binding
	if err := c.addServiceInstanceWithBinding(
		ctx,
		smClient,
		smURL,
		InstanceNameAuditlogManagement,
		ServiceOfferingAuditlogManagement,
		ServicePlanAuditlogManagement,
	); err != nil {
		return err
	}

	// Step 5: Create auditlog service instance + binding
	if err := c.addServiceInstanceWithBinding(
		ctx,
		smClient,
		smURL,
		InstanceNameAuditlog,
		ServiceOfferingAuditlog,
		ServicePlanAuditlog,
	); err != nil {
		return err
	}

	return nil
}

// DeleteLoggingStack deletes the audit logging stack for a given tenant.
// For ADR architecture: deletes services first, then optionally deletes subaccount.
func (c *btpClient) DeleteLoggingStack(ctx context.Context, tenantID string) error {
	// Note: In ADR architecture, subaccountID should be passed explicitly
	// For now, using tenantID as subaccountID (to be refactored)
	subaccountID := tenantID

	// Step 1: Check if Service Manager binding exists
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, subaccountID)
	if err != nil {
		return err
	}

	if !exists || smCreds == nil {
		return nil
	}

	// Step 2: Authenticate with Service Manager
	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return err
	}

	// Step 3: Delete auditlog service instance (includes bindings)
	if err := c.deleteServiceInstance(ctx, smClient, smURL, InstanceNameAuditlog); err != nil {
		return err
	}

	// Step 4: Delete auditlog-management service instance (includes bindings)
	if err := c.deleteServiceInstance(ctx, smClient, smURL, InstanceNameAuditlogManagement); err != nil {
		return err
	}

	// Step 5: Delete Service Manager binding
	if err := c.deleteServiceManagerBinding(ctx, subaccountID); err != nil {
		return err
	}

	// Note: Subaccount deletion will be added when implementing full ADR architecture
	// For now, we only delete the service instances

	return nil
}

// VerifyLoggingStack verifies the installation status of the logging stack for a given tenant.
func (c *btpClient) VerifyLoggingStack(ctx context.Context, tenantID string) (InstallStatus, error) {
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, tenantID)
	if err != nil {
		return InstallStatusError, err
	}

	if !exists || smCreds == nil {
		return NoInstallationStatus, nil
	}

	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return InstallStatusError, err
	}

	managementExists, err := c.serviceInstanceExists(ctx, smClient, smURL, InstanceNameAuditlogManagement)
	if err != nil {
		return InstallStatusError, err
	}

	auditlogExists, err := c.serviceInstanceExists(ctx, smClient, smURL, InstanceNameAuditlog)
	if err != nil {
		return InstallStatusError, err
	}

	if !managementExists && !auditlogExists {
		return NoInstallationStatus, nil
	}

	if managementExists && auditlogExists {
		managementReady, err := c.isServiceInstanceReady(ctx, smClient, smURL, InstanceNameAuditlogManagement)
		if err != nil {
			return InstallStatusError, err
		}

		auditlogReady, err := c.isServiceInstanceReady(ctx, smClient, smURL, InstanceNameAuditlog)
		if err != nil {
			return InstallStatusError, err
		}

		if managementReady && auditlogReady {
			return InstallStatusReady, nil
		}

		return InstallStatusProcessing, nil
	}

	return InstallStatusProcessing, nil
}

// isServiceInstanceReady checks if a service instance is ready
func (c *btpClient) isServiceInstanceReady(ctx context.Context, smClient *Client, smURL, instanceName string) (bool, error) {
	listEndpoint := fmt.Sprintf("%s/v1/service_instances", smURL)
	listResp, err := smClient.Get(ctx, listEndpoint, RequestOptions{
		Query: map[string]string{
			"fieldQuery": fmt.Sprintf("name eq '%s'", instanceName),
		},
	})
	if err != nil {
		return false, err
	}

	var listResult struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		return false, err
	}

	if len(listResult.Items) > 0 {
		instance := listResult.Items[0]
		if ready, ok := instance["ready"].(bool); ok {
			return ready, nil
		}
		return false, nil
	}

	return false, nil
}

// CreateSubaccount creates a new BTP subaccount in the specified region.
// Returns the subaccount GUID on success.
//
// This is a synchronous operation that creates the subaccount with the provided parameters.
// The displayName should be unique to avoid conflicts.
func (c *btpClient) CreateSubaccount(ctx context.Context, region, globalAccountID, displayName string, administrators []string) (string, error) {
	accountsServiceURL := c.credentials.Endpoints["accounts_service_url"]
	if accountsServiceURL == "" {
		return "", fmt.Errorf("accounts_service_url not found in credentials")
	}

	endpoint := accountsServiceURL + "/accounts/v1/subaccounts"

	// Generate subdomain from display name (simplified - add random suffix in production)
	subdomain := displayName

	// Prepare request payload matching POC implementation
	payload := map[string]interface{}{
		"displayName":      displayName,
		"subdomain":        subdomain,
		"origin":           "API",
		"region":           region,
		"parentGUID":       globalAccountID,
		"subaccountAdmins": administrators,
	}

	// Marshal payload to JSON
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Send POST request
	resp, err := c.client.Post(ctx, endpoint, RequestOptions{
		Body:    strings.NewReader(string(bodyBytes)),
		Headers: map[string]string{"Content-Type": "application/json"},
	})
	if err != nil {
		return "", fmt.Errorf("create subaccount request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("subaccount creation failed with status code: %d", resp.StatusCode)
	}

	// Extract subaccount GUID from response
	var responseData map[string]interface{}
	if err := json.Unmarshal(resp.Body, &responseData); err != nil {
		return "", fmt.Errorf("failed to parse subaccount response: %w", err)
	}

	subaccountGUID, ok := responseData["guid"].(string)
	if !ok {
		return "", fmt.Errorf("subaccount GUID not found in response")
	}

	return subaccountGUID, nil
}

// DeleteSubaccount deletes a BTP subaccount by its GUID.
//
// This is a synchronous operation. The subaccount must be empty (no service instances)
// before it can be deleted, otherwise the operation will fail.
func (c *btpClient) DeleteSubaccount(ctx context.Context, subaccountGUID string) error {
	accountsServiceURL := c.credentials.Endpoints["accounts_service_url"]
	if accountsServiceURL == "" {
		return fmt.Errorf("accounts_service_url not found in credentials")
	}

	endpoint := fmt.Sprintf("%s/accounts/v1/subaccounts/%s", accountsServiceURL, subaccountGUID)

	// Send DELETE request
	resp, err := c.client.Delete(ctx, endpoint, RequestOptions{})
	if err != nil {
		return fmt.Errorf("delete subaccount request failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("delete subaccount failed with status code: %d", resp.StatusCode)
	}

	return nil
}

// GetSubaccount retrieves subaccount details for drift detection.
//
// Returns true if the subaccount exists, along with its current state.
// This method is used to verify that the subaccount is in the expected state.
func (c *btpClient) GetSubaccount(ctx context.Context, subaccountGUID string) (exists bool, state string, err error) {
	accountsServiceURL := c.credentials.Endpoints["accounts_service_url"]
	if accountsServiceURL == "" {
		return false, "", fmt.Errorf("accounts_service_url not found in credentials")
	}

	endpoint := fmt.Sprintf("%s/accounts/v1/subaccounts/%s", accountsServiceURL, subaccountGUID)

	// Send GET request
	resp, err := c.client.Get(ctx, endpoint, RequestOptions{})
	if err != nil {
		return false, "", fmt.Errorf("get subaccount request failed: %w", err)
	}

	// 404 means subaccount doesn't exist
	if resp.StatusCode == 404 {
		return false, "", nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, "", fmt.Errorf("get subaccount failed with status code: %d", resp.StatusCode)
	}

	// Parse response
	var subaccountData map[string]interface{}
	if err := json.Unmarshal(resp.Body, &subaccountData); err != nil {
		return false, "", fmt.Errorf("failed to parse subaccount response: %w", err)
	}

	// Extract state
	stateValue := "UNKNOWN"
	if s, ok := subaccountData["state"].(string); ok {
		stateValue = s
	}

	return true, stateValue, nil
}
