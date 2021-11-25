package k8sovirtcredentialsmonitor

import (
	"k8s.io/client-go/rest"
)

// ConnectionConfig contains the configuration for the Kubernetes API.
type ConnectionConfig struct {
	*rest.Config
}
