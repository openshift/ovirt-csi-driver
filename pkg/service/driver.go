package service

import (
	"context"
	"fmt"

	log "github.com/ovirt/go-ovirt-client-log/v2"
	"github.com/ovirt/k8sovirtcredentialsmonitor"
)

var (
	// set by ldflags
	VendorVersion = "0.1.1"
	VendorName    = "csi.ovirt.org"
)

type CSIDriver interface {
	Run(ctx context.Context, running chan struct{}, stopping chan struct{}, stopped chan struct{})
	LastError() error
}

// NewOvirtCSIDriver creates a driver instance
func NewOvirtCSIDriver(
	logger log.Logger,
	kubeConnectionConfig k8sovirtcredentialsmonitor.ConnectionConfig,
	kubeSecretConfig k8sovirtcredentialsmonitor.OVirtSecretConfig,
	nodeId string,
	endpoint string,
) (CSIDriver, error) {
	if err := kubeSecretConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid secret configuration (%w)", err)
	}
	monitor := newCredentialsMonitor(logger, kubeConnectionConfig, kubeSecretConfig)
	ids := NewIdentityServer(monitor.getClient)
	cs := NewControllerServer(monitor.getClient)
	ns := NewNodeServer(nodeId, monitor.getClient)
	grpc, err := NewNonBlockingGRPCServer(logger, ids, cs, ns, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create GRPC server (%v)", err)
	}

	return &ovirtCSIDriver{
		monitor: monitor,
		grpc:    grpc,
	}, nil
}

type ovirtCSIDriver struct {
	logger    log.Logger
	monitor   *credentialsMonitor
	grpc      Service
	lastError error
}

func (driver *ovirtCSIDriver) LastError() error {
	return driver.lastError
}

func (driver *ovirtCSIDriver) Run(
	ctx context.Context,
	running chan struct{},
	stopping chan struct{},
	stopped chan struct{},
) {
	ctx, cancel := context.WithCancel(ctx)
	defer close(stopped)

	grpcRunning := make(chan struct{})
	grpcStopping := make(chan struct{})
	grpcStopped := make(chan struct{})
	monitorRunning := make(chan struct{})
	monitorStopping := make(chan struct{})
	monitorStopped := make(chan struct{})

	driver.grpc.run(ctx, grpcRunning, grpcStopping, grpcStopped)
	defer func() {
		// Make sure this function waits for GRPC to be stopped.
		<-grpcStopped
	}()
	select {
	case <-grpcRunning:
	case <-grpcStopping:
		driver.lastError = fmt.Errorf("GRPC server unexpectedly entered \"stopping\" state during startup")
		close(stopping)
		cancel()
		return
	case <-grpcStopped:
		driver.lastError = fmt.Errorf("GRPC server unexpectedly stopped during startup")
		close(stopping)
		cancel()
		return
	}

	driver.monitor.run(ctx, monitorRunning, monitorStopping, monitorStopped)
	defer func() {
		// Make sure this function waits for monitor to be stopped
		<-monitorStopped
	}()
	select {
	case <-monitorRunning:
	case <-monitorStopping:
		driver.lastError = fmt.Errorf("credentials monitor unexpectedly entered \"stopping\" state during startup")
		close(stopping)
		cancel()
		return
	case <-monitorStopped:
		driver.lastError = fmt.Errorf("credentials monitor unexpectedly stopped during startup")
		close(stopping)
		cancel()
		return
	case <-grpcStopped:
		driver.lastError = fmt.Errorf("GRPC server unexpectedly entered \"stopping\" state during startup")
		close(stopping)
		cancel()
		return
	case <-grpcStopping:
		driver.lastError = fmt.Errorf("GRPC server unexpectedly entered \"stopping\" state during startup")
		close(stopping)
		cancel()
		return
	}

	close(running)

	select {
	case <-ctx.Done():
	case <-monitorStopped:
		driver.lastError = fmt.Errorf("credentials monitor unexpectedly entered stopped during run")
	case <-grpcStopped:
		driver.lastError = fmt.Errorf("GRPC server unexpectedly stopped during run")
	case <-monitorStopping:
		driver.lastError = fmt.Errorf("credentials monitor unexpectedly entered \"stopping\" state during run")
	case <-grpcStopping:
		driver.lastError = fmt.Errorf("GRPC server unexpectedly entered \"stopping\" state during run")
	}
	close(stopping)
	cancel()
}
