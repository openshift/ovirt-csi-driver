package k8sovirtcredentialsmonitor

import (
	"context"
	"fmt"
	"sync"
	"time"

	ovirtclient "github.com/ovirt/go-ovirt-client"
	log "github.com/ovirt/go-ovirt-client-log/v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// OVirtCredentialMonitor is a utility to monitor a Kubernetes secret and call a callback function with a new
// connection whenever the credentials change.
type OVirtCredentialMonitor interface {
	// GetConnection returns the client from the latest credential update. It is recommended that this function should
	// only be called under a lock shared with the callback to avoid race conditions.
	// This function may return an error if the stored credentials are not valid.
	GetConnection() (ovirtclient.ClientWithLegacySupport, error)
	// Run runs in foreground until the context expires.
	Run(ctx context.Context)
}

type oVirtCredentialMonitor struct {
	cli          *kubernetes.Clientset
	secretConfig OVirtSecretConfig
	callbacks    Callbacks
	secret       *corev1.Secret
	lock         *sync.Mutex
	connection   ovirtclient.ClientWithLegacySupport
	logger       log.Logger
}

func (o *oVirtCredentialMonitor) GetConnection() (ovirtclient.ClientWithLegacySupport, error) {
	o.lock.Lock()
	defer o.lock.Unlock()

	if o.callbacks.OnCredentialsValidate != nil {
		if err := o.callbacks.OnCredentialsValidate(o.connection); err != nil {
			o.logger.Errorf("stored oVirt credentials are invalid (%v)", err)
			return nil, fmt.Errorf("stored oVirt credentials are invalid (%w)", err)
		}
	}

	return o.connection, nil
}

func (o *oVirtCredentialMonitor) createWatch(ctx context.Context) (watch.Interface, error) {
	o.logger.Debugf(
		"Watching secret %s for changes...",
		o.secretConfig.String(),
	)
	w, err := o.cli.CoreV1().Secrets(o.secretConfig.Namespace).Watch(
		ctx, v1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", o.secretConfig.Name),
			Watch:         true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create watch (%w)", err)
	}

	secret, err := o.cli.CoreV1().Secrets(o.secretConfig.Namespace).Get(ctx, o.secretConfig.Name, v1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf(
			"failed to fetch secret with the name %s in namespaces %s (%w)",
			o.secretConfig.Name,
			o.secretConfig.Namespace,
			err,
		)
	}

	o.sendCallback(secret)

	return w, nil
}

func (o *oVirtCredentialMonitor) Run(ctx context.Context) {
	var w watch.Interface
	var err error
	defer func() {
		if o.callbacks.OnMonitorShuttingDown != nil {
			o.callbacks.OnMonitorShuttingDown()
		}
		if w != nil {
			w.Stop()
		}
	}()
	if o.callbacks.OnMonitorRunning != nil {
		o.callbacks.OnMonitorRunning()
	}
	for {
		w, err = o.createWatch(ctx)
		if err == nil {
		loop:
			for {
				select {
				case result, ok := <-w.ResultChan():
					if !ok {
						o.logger.Warningf(
							"Watching the %s secret failed (%w).",
							o.secretConfig.String(),
							err,
						)
						break loop
					}
					o.logger.Debugf(
						"Change type %s received.",
						result.Type,
					)
					if result.Type == watch.Modified {
						if secret, ok := result.Object.(*corev1.Secret); ok {
							o.sendCallback(secret)
						}
					}
				case <-ctx.Done():
					o.logger.Infof(
						"Shutting down oVirt secret monitoring...",
					)
					return
				}
			}
		} else {
			o.logger.Warningf(
				"Creating a watch for the %s secret failed (%w).",
				o.secretConfig.String(),
				err,
			)
		}
		select {
		case <-time.After(time.Minute):
			continue
		case <-ctx.Done():
			return
		}
	}
}

func (o *oVirtCredentialMonitor) sendCallback(secret *corev1.Secret) {
	o.lock.Lock()
	defer o.lock.Unlock()

	if secret.ResourceVersion == o.secret.ResourceVersion {
		return
	}

	conn, err := buildConnection(secret, o.logger)
	if err != nil {
		o.logger.Errorf(
			"The %s secret contains an invalid oVirt configuration (%w).",
			o.secretConfig.String(),
			err,
		)
		return
	}

	o.logger.Infof("oVirt credentials in secret %s have changed.", o.secretConfig.String())

	if err := o.callbacks.OnCredentialsValidate(conn); err != nil {
		o.logger.Errorf("oVirt credentials are invalid, retaining old credentials (%v).", err)
		return
	}

	o.secret = secret
	o.connection = conn

	if o.callbacks.OnCredentialsChange != nil {
		o.callbacks.OnCredentialsChange(
			conn,
		)
	}
}

func buildConnection(secret *corev1.Secret, logger log.Logger) (ovirtclient.ClientWithLegacySupport, error) {
	data := secret.Data
	url, ok := data["ovirt_url"]
	if !ok {
		return nil, fmt.Errorf("missing ovirt_url in secret")
	}
	username, ok := data["ovirt_username"]
	if !ok {
		return nil, fmt.Errorf("missing ovirt_username in secret")
	}
	password, ok := data["ovirt_password"]
	if !ok {
		return nil, fmt.Errorf("missing ovirt_password in secret")
	}
	tls := ovirtclient.TLS()
	if insecure, ok := data["ovirt_insecure"]; ok {
		if string(insecure) == "true" {
			tls.Insecure()
		}
	} else if bundle, ok := data["ovirt_ca_bundle"]; ok {
		tls.CACertsFromMemory(bundle)
	} else {
		return nil, fmt.Errorf("neither ovirt_insecure nor ovirt_ca_bundle is specified, please specify a certificate verification method")
	}

	connection, err := ovirtclient.NewWithVerify(
		string(url),
		string(username),
		string(password),
		tls,
		logger,
		nil,
		// Disable verification as we'll do that.
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create oVirt connection (%w)", err)
	}

	return connection, nil
}
