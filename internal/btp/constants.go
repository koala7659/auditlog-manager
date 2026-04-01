package btp

import "time"

const (
	// Service offering and plan names
	ServiceOfferingAuditlogManagement = "auditlog-management"
	ServicePlanAuditlogManagement     = "default"
	ServiceOfferingAuditlog           = "auditlog"
	ServicePlanAuditlog               = "standard"

	// Instance naming
	InstanceNameAuditlogManagement = "auditlog-management-instance"
	InstanceNameAuditlog           = "auditlog-instance"

	// Timeouts
	DefaultHTTPTimeout = 30 * time.Second
)
