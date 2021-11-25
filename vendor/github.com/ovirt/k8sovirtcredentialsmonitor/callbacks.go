package k8sovirtcredentialsmonitor

import (
	ovirtclient "github.com/ovirt/go-ovirt-client"
)

// Callbacks is a configuration structure that holds the possible callbacks for the monitor.
type Callbacks struct {
	// OnMonitorRunning is a callback that is called after the watch is set up for the secret.
	OnMonitorRunning func()

	// OnMonitorShuttingDown is a callback that is called before the watch is shut down.
	OnMonitorShuttingDown func()

	// OnCredentialsChange is called when the oVirt credentials change.
	OnCredentialsChange func(ovirtclient.ClientWithLegacySupport)

	// OnCredentialsValidate is called before the credentials are passed/returned. This
	// can be used to validate the credentials, e.g. by calling connection.Test()
	// If not configured, k8sovirtcredentialsmonitor.ValidateCredentials will be used here.
	OnCredentialsValidate func(ovirtclient.ClientWithLegacySupport) error
}
