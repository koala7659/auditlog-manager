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
func (c *btpClient) CreateLoggingStack(ctx context.Context, tenantID string) error {
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, tenantID)
	if err != nil {
		return err
	}

	if !exists || smCreds == nil {
		smCreds, err = c.addServiceManagerBinding(ctx, tenantID)
		if err != nil {
			return err
		}
	}

	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return err
	}

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
func (c *btpClient) DeleteLoggingStack(ctx context.Context, tenantID string) error {
	exists, smCreds, err := c.serviceManagerBindingExists(ctx, tenantID)
	if err != nil {
		return err
	}

	if !exists || smCreds == nil {
		return nil
	}

	smClient, smURL, err := c.authenticateServiceManager(smCreds)
	if err != nil {
		return err
	}

	if err := c.deleteServiceInstance(ctx, smClient, smURL, InstanceNameAuditlog); err != nil {
		return err
	}

	if err := c.deleteServiceInstance(ctx, smClient, smURL, InstanceNameAuditlogManagement); err != nil {
		return err
	}

	if err := c.deleteServiceManagerBinding(ctx, tenantID); err != nil {
		return err
	}

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
