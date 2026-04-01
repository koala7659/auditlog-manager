package btp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// serviceManagerBindingExists checks if a Service Manager binding exists for the given subaccount
func (c *btpClient) serviceManagerBindingExists(ctx context.Context, subaccountGUID string) (bool, *ServiceManagerCredentials, error) {
	accountsServiceURL := c.credentials.Endpoints["accounts_service_url"]
	if accountsServiceURL == "" {
		return false, nil, fmt.Errorf("accounts_service_url not found in credentials")
	}

	// Construct endpoint for V1 API
	endpoint := fmt.Sprintf("%s/accounts/v1/subaccounts/%s/serviceManagementBinding", accountsServiceURL, subaccountGUID)

	// Send GET request
	resp, err := c.client.Get(ctx, endpoint, RequestOptions{})
	if err != nil {
		// If error contains "not found" or similar, binding doesn't exist
		if resp != nil && resp.StatusCode == 404 {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to check service manager binding: %w", err)
	}

	// Status 200 means binding exists
	if resp.StatusCode == 200 {
		// Parse credentials from response
		var smCreds ServiceManagerCredentials
		if err := json.Unmarshal(resp.Body, &smCreds); err != nil {
			return true, nil, fmt.Errorf("binding exists but failed to parse credentials: %w", err)
		}
		return true, &smCreds, nil
	}

	// Status 404 means binding doesn't exist
	if resp.StatusCode == 404 {
		return false, nil, nil
	}

	// Other status codes are unexpected
	return false, nil, fmt.Errorf("unexpected status code %d when checking binding", resp.StatusCode)
}

// addServiceManagerBinding creates a Service Manager binding for the given subaccount
func (c *btpClient) addServiceManagerBinding(ctx context.Context, subaccountGUID string) (*ServiceManagerCredentials, error) {
	accountsServiceURL := c.credentials.Endpoints["accounts_service_url"]
	if accountsServiceURL == "" {
		return nil, fmt.Errorf("accounts_service_url not found in credentials")
	}

	// Construct endpoint for V1 API
	endpoint := fmt.Sprintf("%s/accounts/v1/subaccounts/%s/serviceManagementBinding", accountsServiceURL, subaccountGUID)

	// Send POST request with empty body for basic credentials
	resp, err := c.client.Post(ctx, endpoint, RequestOptions{
		Body:    bytes.NewReader([]byte("{}")),
		Headers: map[string]string{"Content-Type": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("create service manager binding request failed: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Parse credentials from response
		var smCreds ServiceManagerCredentials
		if err := json.Unmarshal(resp.Body, &smCreds); err != nil {
			return nil, fmt.Errorf("failed to parse service manager credentials: %w", err)
		}

		return &smCreds, nil
	}

	return nil, fmt.Errorf("create service manager binding failed with status code: %d", resp.StatusCode)
}

// deleteServiceManagerBinding deletes the Service Manager binding for the given subaccount
func (c *btpClient) deleteServiceManagerBinding(ctx context.Context, subaccountGUID string) error {
	accountsServiceURL := c.credentials.Endpoints["accounts_service_url"]
	if accountsServiceURL == "" {
		return fmt.Errorf("accounts_service_url not found in credentials")
	}

	// Construct endpoint for V1 API
	endpoint := fmt.Sprintf("%s/accounts/v1/subaccounts/%s/serviceManagementBinding", accountsServiceURL, subaccountGUID)

	// Send DELETE request
	resp, err := c.client.Delete(ctx, endpoint, RequestOptions{})
	if err != nil {
		return fmt.Errorf("delete service manager binding request failed: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("delete service manager binding failed with status code: %d", resp.StatusCode)
}

// authenticateServiceManager authenticates with Service Manager using binding credentials
func (c *btpClient) authenticateServiceManager(smCreds *ServiceManagerCredentials) (*Client, string, error) {
	token, err := GetOAuthToken(
		"client_credentials",
		smCreds.TokenURL,
		smCreds.ClientID,
		smCreds.ClientSecret,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get Service Manager token: %w", err)
	}

	return NewClient(DefaultHTTPTimeout, token), smCreds.SMURL, nil
}

// serviceInstanceExists checks if a service instance with the given name exists
func (c *btpClient) serviceInstanceExists(ctx context.Context, smClient *Client, smURL, instanceName string) (bool, error) {
	// Use query parameters with proper URL encoding
	listEndpoint := fmt.Sprintf("%s/v1/service_instances", smURL)
	listResp, err := smClient.Get(ctx, listEndpoint, RequestOptions{
		Query: map[string]string{
			"fieldQuery": fmt.Sprintf("name eq '%s'", instanceName),
		},
	})
	if err != nil {
		return false, fmt.Errorf("failed to list service instances: %w", err)
	}

	// Parse response to check if instance exists
	var listResult struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		return false, fmt.Errorf("failed to parse service instances list: %w", err)
	}

	if len(listResult.Items) > 0 {
		instance := listResult.Items[0]

		// Additional verification - check service_plan_id exists
		// This ensures it's the right type of service
		if _, ok := instance["service_plan_id"].(string); !ok {
			return false, fmt.Errorf("instance exists but missing service_plan_id - may be invalid")
		}

		return true, nil
	}

	return false, nil
}

// getServiceInstanceID retrieves the ID of a service instance by name
func (c *btpClient) getServiceInstanceID(ctx context.Context, smClient *Client, smURL, instanceName string) (string, error) {
	// Use query parameters with proper URL encoding
	listEndpoint := fmt.Sprintf("%s/v1/service_instances", smURL)
	listResp, err := smClient.Get(ctx, listEndpoint, RequestOptions{
		Query: map[string]string{
			"fieldQuery": fmt.Sprintf("name eq '%s'", instanceName),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list service instances: %w", err)
	}

	// Parse response to get instance ID
	var listResult struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		return "", fmt.Errorf("failed to parse service instances list: %w", err)
	}

	if len(listResult.Items) > 0 {
		instance := listResult.Items[0]
		if id, ok := instance["id"].(string); ok {
			return id, nil
		}
		return "", fmt.Errorf("instance found but ID is missing")
	}

	return "", fmt.Errorf("instance '%s' not found", instanceName)
}

// createServiceInstance creates a service instance with the given parameters
func (c *btpClient) createServiceInstance(ctx context.Context, smClient *Client, smURL, instanceName, serviceOfferingName, servicePlanName string) (string, error) {
	// Use format=cpcli as per BTP CLI
	createEndpoint := fmt.Sprintf("%s/v1/service_instances?format=cpcli", smURL)

	payload := map[string]interface{}{
		"name":                  instanceName,
		"service_offering_name": serviceOfferingName,
		"service_plan_name":     servicePlanName,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Send POST request
	createResp, err := smClient.Post(ctx, createEndpoint, RequestOptions{
		Body:    bytes.NewReader(body),
		Headers: map[string]string{"Content-Type": "application/json"},
	})

	if err != nil {
		return "", fmt.Errorf("create service instance request failed: %w", err)
	}

	if createResp.StatusCode >= 200 && createResp.StatusCode < 300 {
		// Extract instance ID from response
		var instanceResponse map[string]interface{}
		var instanceID string
		if err := json.Unmarshal(createResp.Body, &instanceResponse); err == nil {
			if id, ok := instanceResponse["id"].(string); ok {
				instanceID = id
				return instanceID, nil
			}
		}

		return "", fmt.Errorf("instance created but ID not found in response")
	}

	return "", fmt.Errorf("create service instance failed with status code: %d", createResp.StatusCode)
}

// listServiceBindingsForInstance retrieves all bindings for a service instance
func (c *btpClient) listServiceBindingsForInstance(ctx context.Context, smClient *Client, smURL, serviceInstanceID string) ([]map[string]interface{}, error) {
	listEndpoint := fmt.Sprintf("%s/v1/service_bindings", smURL)
	listResp, err := smClient.Get(ctx, listEndpoint, RequestOptions{
		Query: map[string]string{
			"fieldQuery": fmt.Sprintf("service_instance_id eq '%s'", serviceInstanceID),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list service bindings: %w", err)
	}

	var listResult struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		return nil, fmt.Errorf("failed to parse service bindings list: %w", err)
	}

	return listResult.Items, nil
}

// createServiceBinding creates a binding for a service instance
func (c *btpClient) createServiceBinding(ctx context.Context, smClient *Client, smURL, bindingName, serviceInstanceID string) (string, error) {
	// Check if a binding already exists for this instance
	existingBindings, err := c.listServiceBindingsForInstance(ctx, smClient, smURL, serviceInstanceID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing bindings: %w", err)
	}

	// If bindings exist, check if one matches our desired name or just return the first one
	if len(existingBindings) > 0 {
		// First try to find binding with matching name
		for _, binding := range existingBindings {
			if name, ok := binding["name"].(string); ok && name == bindingName {
				if id, ok := binding["id"].(string); ok {
					return id, nil
				}
			}
		}

		// If no exact name match but bindings exist, return the first one
		if id, ok := existingBindings[0]["id"].(string); ok {
			return id, nil
		}
	}

	createEndpoint := fmt.Sprintf("%s/v1/service_bindings", smURL)

	payload := map[string]interface{}{
		"name":                bindingName,
		"service_instance_id": serviceInstanceID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Send POST request
	createResp, err := smClient.Post(ctx, createEndpoint, RequestOptions{
		Body:    bytes.NewReader(body),
		Headers: map[string]string{"Content-Type": "application/json"},
	})

	if err != nil {
		return "", fmt.Errorf("create service binding request failed: %w", err)
	}

	if createResp.StatusCode >= 200 && createResp.StatusCode < 300 {
		// Extract binding ID from response
		var bindingResponse map[string]interface{}
		var bindingID string
		if err := json.Unmarshal(createResp.Body, &bindingResponse); err == nil {
			if id, ok := bindingResponse["id"].(string); ok {
				bindingID = id
				return bindingID, nil
			}
		}
		return "", fmt.Errorf("binding created but ID not found in response")
	}

	return "", fmt.Errorf("create service binding failed with status code: %d", createResp.StatusCode)
}

// deleteServiceBindingsForInstance deletes all bindings for a service instance
func (c *btpClient) deleteServiceBindingsForInstance(ctx context.Context, smClient *Client, smURL, serviceInstanceID string) error {
	// Use the common helper function to list bindings
	bindings, err := c.listServiceBindingsForInstance(ctx, smClient, smURL, serviceInstanceID)
	if err != nil {
		return err
	}

	if len(bindings) == 0 {
		return nil
	}

	// Delete each binding
	for _, item := range bindings {
		bindingID, ok := item["id"].(string)
		if !ok {
			continue
		}

		deleteEndpoint := fmt.Sprintf("%s/v1/service_bindings/%s", smURL, bindingID)
		deleteResp, err := smClient.Delete(ctx, deleteEndpoint, RequestOptions{})

		if err != nil {
			return fmt.Errorf("failed to delete binding %s: %w", bindingID, err)
		}

		if deleteResp.StatusCode < 200 || deleteResp.StatusCode >= 300 {
			return fmt.Errorf("delete binding failed with status code: %d", deleteResp.StatusCode)
		}
	}

	return nil
}

// deleteServiceInstance deletes a service instance and all its bindings
func (c *btpClient) deleteServiceInstance(ctx context.Context, smClient *Client, smURL, instanceName string) error {
	// Step 1: Check if instance exists and get its ID
	exists, err := c.serviceInstanceExists(ctx, smClient, smURL, instanceName)
	if err != nil {
		return fmt.Errorf("failed to check if instance exists: %w", err)
	}

	if !exists {
		// Instance doesn't exist, nothing to delete
		return nil
	}

	// Step 2: Get the instance ID by listing instances with the name
	listEndpoint := fmt.Sprintf("%s/v1/service_instances", smURL)
	listResp, err := smClient.Get(ctx, listEndpoint, RequestOptions{
		Query: map[string]string{
			"fieldQuery": fmt.Sprintf("name eq '%s'", instanceName),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list service instances: %w", err)
	}

	var listResult struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(listResp.Body, &listResult); err != nil {
		return fmt.Errorf("failed to parse service instances list: %w", err)
	}

	if len(listResult.Items) == 0 {
		return fmt.Errorf("instance '%s' not found", instanceName)
	}

	instanceID, ok := listResult.Items[0]["id"].(string)
	if !ok {
		return fmt.Errorf("failed to get instance ID from response")
	}

	// Step 3: Delete all service bindings for this instance first
	if err := c.deleteServiceBindingsForInstance(ctx, smClient, smURL, instanceID); err != nil {
		return fmt.Errorf("failed to delete service bindings: %w", err)
	}

	// Step 4: Delete the instance
	deleteEndpoint := fmt.Sprintf("%s/v1/service_instances/%s", smURL, instanceID)

	deleteResp, err := smClient.Delete(ctx, deleteEndpoint, RequestOptions{})
	if err != nil {
		return fmt.Errorf("delete service instance request failed: %w", err)
	}

	if deleteResp.StatusCode >= 200 && deleteResp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("delete service instance failed with status code: %d", deleteResp.StatusCode)
}

// addServiceInstanceWithBinding creates a service instance and binding (idempotent)
func (c *btpClient) addServiceInstanceWithBinding(ctx context.Context, smClient *Client, smURL, instanceName, serviceOfferingName, servicePlanName string) error {
	exists, err := c.serviceInstanceExists(ctx, smClient, smURL, instanceName)
	if err != nil {
		return err
	}

	var instanceID string
	if exists {
		// Get the instance ID for binding creation
		instanceID, err = c.getServiceInstanceID(ctx, smClient, smURL, instanceName)
		if err != nil {
			return fmt.Errorf("failed to get existing instance ID: %w", err)
		}
	} else {
		// Create service instance
		instanceID, err = c.createServiceInstance(ctx, smClient, smURL, instanceName, serviceOfferingName, servicePlanName)
		if err != nil {
			return err
		}
	}

	bindingName := instanceName + "-binding"

	// Create service binding for the instance (idempotent)
	if instanceID != "" {
		_, err = c.createServiceBinding(ctx, smClient, smURL, bindingName, instanceID)
		if err != nil {
			return fmt.Errorf("failed to create binding: %w", err)
		}
	}

	return nil
}
