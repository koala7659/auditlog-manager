package btp

import (
	"context"
	"encoding/json"
	"fmt"
)

type InstallStatus string

const (
	NoInstallationStatus    InstallStatus = "NoInstallation"
	InstallStatusReady      InstallStatus = "Ready"
	InstallStatusProcessing InstallStatus = "Processing"
	InstallStatusError      InstallStatus = "Error"
)

type BTPClient interface {
	CreateLoggingStack(ctx context.Context, tenantID string) error
	DeleteLoggingStack(ctx context.Context, tenantID string) error
	VerifyLoggingStack(ctx context.Context, tenantID string) (InstallStatus, error)
}

type btpClient struct {
	client      *Client
	credentials *Credentials
	token       *XSUAAToken
}

// NewBtpClient creates a new BTP client with authentication
func NewBtpClient(credentialPath string) (BTPClient, error) {
	// Load credentials from file
	creds, err := LoadCredentials(credentialPath)
	if err != nil {
		return nil, err
	}

	// Authenticate with XSUAA to get OAuth token
	token, err := GetOAuthToken(
		"client_credentials",
		creds.UAA.URL,
		creds.UAA.ClientID,
		creds.UAA.ClientSecret,
	)
	if err != nil {
		return nil, err
	}

	// Create HTTP client with OAuth transport
	httpClient := NewClient(DefaultHTTPTimeout, token)

	return &btpClient{
		client:      httpClient,
		credentials: creds,
		token:       token,
	}, nil
}

// CreateLoggingStack creates the audit logging stack for a given tenant
func (c *btpClient) CreateLoggingStack(ctx context.Context, tenantID string) error {
	// Step 1: Check if Service Manager binding already exists
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, tenantID)
	if err != nil {
		return err
	}

	if !exists || smCreds == nil {
		// Create Service Manager binding
		smCreds, err = c.addServiceManagerBinding(ctx, tenantID)
		if err != nil {
			return err
		}
	}

	// Step 2: Authenticate with Service Manager
	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return err
	}

	// Step 3: Create auditlog-management instance with binding
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

	// Step 4: Create auditlog instance with binding
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

// DeleteLoggingStack deletes the audit logging stack for a given tenant
func (c *btpClient) DeleteLoggingStack(ctx context.Context, tenantID string) error {
	// Step 1: Check if Service Manager binding exists
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, tenantID)
	if err != nil {
		return err
	}

	if !exists || smCreds == nil {
		// Nothing to delete
		return nil
	}

	// Step 2: Authenticate with Service Manager
	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return err
	}

	// Step 3: Delete auditlog instance (and its bindings)
	if err := c.deleteServiceInstance(ctx, smClient, smURL, InstanceNameAuditlog); err != nil {
		// Log error but continue with deletion
		return err
	}

	// Step 4: Delete auditlog-management instance (and its bindings)
	if err := c.deleteServiceInstance(ctx, smClient, smURL, InstanceNameAuditlogManagement); err != nil {
		// Log error but continue with deletion
		return err
	}

	// Step 5: Delete Service Manager binding
	if err := c.deleteServiceManagerBinding(ctx, tenantID); err != nil {
		return err
	}

	return nil
}

// VerifyLoggingStack verifies the status of the logging stack for a given tenant
func (c *btpClient) VerifyLoggingStack(ctx context.Context, tenantID string) (InstallStatus, error) {
	// Step 1: Check if Service Manager binding exists
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, tenantID)
	if err != nil {
		return InstallStatusError, err
	}

	if !exists || smCreds == nil {
		// No installation exists
		return NoInstallationStatus, nil
	}

	// Step 2: Authenticate with Service Manager
	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return InstallStatusError, err
	}

	// Step 3: Check auditlog-management instance
	managementExists, err := c.serviceInstanceExists(ctx, smClient, smURL, InstanceNameAuditlogManagement)
	if err != nil {
		return InstallStatusError, err
	}

	// Step 4: Check auditlog instance
	auditlogExists, err := c.serviceInstanceExists(ctx, smClient, smURL, InstanceNameAuditlog)
	if err != nil {
		return InstallStatusError, err
	}

	// If neither instance exists, no installation
	if !managementExists && !auditlogExists {
		return NoInstallationStatus, nil
	}

	// If both instances exist, need to check if they are ready
	if managementExists && auditlogExists {
		// Get instance details to check ready status
		managementReady, err := c.isServiceInstanceReady(ctx, smClient, smURL, InstanceNameAuditlogManagement)
		if err != nil {
			return InstallStatusError, err
		}

		auditlogReady, err := c.isServiceInstanceReady(ctx, smClient, smURL, InstanceNameAuditlog)
		if err != nil {
			return InstallStatusError, err
		}

		// Both instances ready
		if managementReady && auditlogReady {
			return InstallStatusReady, nil
		}

		// At least one instance not ready
		return InstallStatusProcessing, nil
	}

	// Partial installation - still processing
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
		// If ready field is missing, assume not ready
		return false, nil
	}

	return false, nil
}
