package k8sovirtcredentialsmonitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/ovirt/go-ovirt-client-log/v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func New(
	connectionConfig ConnectionConfig,
	secretConfig OVirtSecretConfig,
	callbacks Callbacks,
	logger log.Logger,
) (OVirtCredentialMonitor, error) {
	if err := secretConfig.Validate(); err != nil {
		return nil, fmt.Errorf("secret configuration validation failed (%w)", err)
	}

	if callbacks.OnCredentialsChange == nil {
		return nil, fmt.Errorf("the OnCredentialsChange option is required for the callbacks")
	}
	if callbacks.OnCredentialsValidate == nil {
		callbacks.OnCredentialsValidate = ValidateCredentials
	}

	cli, err := kubernetes.NewForConfig(connectionConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client (%w)", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	secret, err := cli.CoreV1().Secrets(secretConfig.Namespace).Get(ctx, secretConfig.Name, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to fetch secret with the name %s in namespaces %s (%w)",
			secretConfig.Name,
			secretConfig.Namespace,
			err,
		)
	}

	if logger == nil {
		logger = log.NewNOOPLogger()
	}

	conn, err := buildConnection(secret, logger)
	if err != nil {
		return nil, err
	}

	return &oVirtCredentialMonitor{
		cli:          cli,
		secretConfig: secretConfig,
		callbacks:    callbacks,
		secret:       secret,
		lock:         &sync.Mutex{},
		connection:   conn,
		logger:       logger,
	}, nil
}
