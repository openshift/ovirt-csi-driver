package k8sovirtcredentialsmonitor

import (
	"fmt"
)

// OVirtSecretConfig holds the configuration for which secret to monitor.
type OVirtSecretConfig struct {
	// Name is the name of the secret to be read.
	Name string
	// Namespace is the optional namespace specification.
	Namespace string
}

// Validate checks if the secret configuration contains valid values.
func (o OVirtSecretConfig) Validate() error {
	if o.Name == "" {
		return fmt.Errorf("the Name field for the oVirt secret config is required")
	}
	if o.Namespace == "" {
		return fmt.Errorf("the Namespace field for the oVirt secret config is required")
	}
	return nil
}

// String converts the secret configuration to a human-readable string.
func (o OVirtSecretConfig) String() string {
	return fmt.Sprintf("%s/%s", o.Namespace, o.Name)
}
