package service

import (
	"context"
	"fmt"
	"sync"

	ovirtclient "github.com/ovirt/go-ovirt-client"
	log "github.com/ovirt/go-ovirt-client-log/v2"
	"github.com/ovirt/k8sovirtcredentialsmonitor"
)

func newCredentialsMonitor(
	logger log.Logger,
	kubeConnectionConfig k8sovirtcredentialsmonitor.ConnectionConfig,
	kubeSecretConfig k8sovirtcredentialsmonitor.OVirtSecretConfig,
) *credentialsMonitor {
	return &credentialsMonitor{
		lock:                 &sync.Mutex{},
		logger:               logger,
		client:               nil,
		kubeConnectionConfig: kubeConnectionConfig,
		kubeSecretConfig:     kubeSecretConfig,
	}
}

type credentialsMonitor struct {
	lock                 *sync.Mutex
	logger               log.Logger
	client               ovirtclient.Client
	kubeConnectionConfig k8sovirtcredentialsmonitor.ConnectionConfig
	kubeSecretConfig     k8sovirtcredentialsmonitor.OVirtSecretConfig
}

func (c *credentialsMonitor) getClient() (ovirtclient.Client, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.client == nil {
		return nil, fmt.Errorf("the oVirt client is currently not available")
	}
	return c.client, nil
}

func (c *credentialsMonitor) run(
	ctx context.Context,
	ready chan struct{},
	stopping chan struct{},
	stopped chan struct{},
) {
	defer close(stopped)
	monitor, err := k8sovirtcredentialsmonitor.New(
		c.kubeConnectionConfig,
		c.kubeSecretConfig,
		k8sovirtcredentialsmonitor.Callbacks{
			OnMonitorRunning: func() {
				close(ready)
			},
			OnMonitorShuttingDown: func() {
				close(stopping)
			},
			OnCredentialsChange: func(client ovirtclient.ClientWithLegacySupport) {
				c.lock.Lock()
				defer c.lock.Unlock()
				c.client = client
			},
		},
		c.logger,
	)
	if err != nil {
		c.logger.Errorf("failed to initialize k8s credentials monitor (%w)", err)
		close(stopping)
		return
	}
	monitor.Run(ctx)
}
