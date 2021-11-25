# K8S oVirt Credentials Monitor

[![GitHub Workflow Status](https://img.shields.io/github/workflow/status/ovirt/k8sOVirtCredentialsMonitor/Build?style=for-the-badge)](https://github.com/oVirt/k8sOVirtCredentialsMonitor/actions)

This library provides the ability to monitor a secret in Kubernetes and provide an updated oVirt SDK client to a hook function.

## Basic usage

In order to use this library you will need to add it as a Go dependency:

```
go get github.com/ovirt/k8sovirtcredentialsmonitor
```

Then you can set up the monitor:

```go
package main

import (
	"context"
	goLog "log"
	"os"
	"sync"

	ovirtclient "github.com/ovirt/go-ovirt-client"
	log "github.com/ovirt/go-ovirt-client-log/v2"
	"github.com/ovirt/k8sovirtcredentialsmonitor"
)

func main() {
	// Set the namespace to monitor.
	var secretNamespace = "default"
	// Set the secret to monitor.
	var secretName = "ovirt-credentials"
	// This variable will hold connection.
	var connection ovirtclient.ClientWithLegacySupport
	// Set up logging. See go-ovirt-client-log for details.
	var logger = log.NewGoLogger(goLog.New(os.Stdout, "", 0))

	// We use the connLock to avoid race conditions.
	connLock := &sync.Mutex{}

	// This callback will be called when the credentials change.
	callbacks := k8sovirtcredentialsmonitor.Callbacks{
		OnCredentialsChange: func (
			conn ovirtclient.ClientWithLegacySupport,
		) {
			connLock.Lock()
			defer connLock.Unlock()

			// Save the connection.
			connection = conn
		},
		// Optional callbacks without extra parameters
		// - OnMonitorRunning - when the monitor has started
		// - OnMonitorShuttingDown - before the monitor is shutting down
		// - OnCredentialsValidate - allows you to validate credentials before use.
		//                           You can pass the
		//                           k8sOVirtCredentialsMonitor.ValidateCredentials
		//                           function here.
	}

	// Set up the credential monitor.
	monitor, err := k8sovirtcredentialsmonitor.New(
		k8sovirtcredentialsmonitor.ConnectionConfig {
			// add Kubernetes connection parameters here
		},
		k8sovirtcredentialsmonitor.OVirtSecretConfig{
			Name:      secretName,
			Namespace: secretNamespace,
		},
		callbacks,
		logger,
	)
	if err != nil {
		// Handle error
		panic(err)
	}

	// Set up the context for stopping the monitor.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run the monitor in the background. This goroutine runs until the context
	// is canceled.
	go monitor.Run(ctx)

	// Set up initial connection. We do this in a lock to avoid race conditions.
	connLock.Lock()
	connection, err = monitor.GetConnection()
	connLock.Unlock()
	if err != nil {
		// The connection has failed. Handle error here.
		goLog.Fatalf("oVirt connection failed (%v)", err)
	}

	// Use sdkConnection here. The connection variable will be updated in the hooks above when the credentials change.
}
```

## Configuration format

The oVirt configuration format in secrets is as follows:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ovirt-credentials
type: generic
data:
  ovirt_url: <oVirt engine URL here, base64 encoded>
  ovirt_username: <oVirt username here, base64 encoded>
  ovirt_password: <oVirt password here, base64 encoded>
  ovirt_insecure: <true or false, base64 encoded; optional; not recommended>
  ovirt_ca_bundle: <CA certificate in PEM format, base64-encoded; optional if ovirt_insecure is true>
```

## Logging

This library uses [go-ovirt-client-log](https://github.com/ovirt/go-ovirt-client-log) as a logging backend. If no logger is provided (`nil`) logging is disabled. You can also create a logger that logs via the Go testing facility:

