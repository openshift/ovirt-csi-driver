package service_test

import (
	"testing"

	ovirtclient "github.com/ovirt/go-ovirt-client"
	ovirtclientlog "github.com/ovirt/go-ovirt-client-log/v2"
)

func getMockHelper(t *testing.T) ovirtclient.TestHelper {
	helper, err := ovirtclient.NewTestHelper(
		"https://localhost/ovirt-engine/api",
		"admin@internal",
		"",
		nil,
		ovirtclient.TLS().Insecure(),
		true,
		ovirtclientlog.NewTestLogger(t),
	)
	if err != nil {
		t.Fatalf("Failed to create test helper (%v).", err)
	}
	return helper
}
