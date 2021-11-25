package k8sovirtcredentialsmonitor

import (
	ovirtclient "github.com/ovirt/go-ovirt-client"
)

// ValidateCredentials provides default validation of an oVirt Connection.
func ValidateCredentials(connection ovirtclient.ClientWithLegacySupport) error {
	return connection.Test()
}
