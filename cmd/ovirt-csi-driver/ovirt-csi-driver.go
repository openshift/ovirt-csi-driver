package main

import (
	"context"
	"flag"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"github.com/ovirt/csi-driver/pkg/service"
	klog "github.com/ovirt/go-ovirt-client-log-klog"
	"github.com/ovirt/k8sovirtcredentialsmonitor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	endpoint        = flag.String("endpoint", "unix:/csi/csi.sock", "CSI endpoint")
	namespace       = flag.String("namespace", "", "Namespace to run the controllers on")
	secretNamespace = flag.String(
		"secret-namespace",
		"",
		"Which namespace to look for the oVirt secret on. If empty, the namespace will be loaded from /var/run/secrets/kubernetes.io if possible.",
	)
	secretName = flag.String("secret-name", "ovirt-credentials", "Secret to use for oVirt credentials")
	nodeName   = flag.String("node-name", "", "The node name - the node this pods runs on")
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	os.Exit(handle())
}

func handle() int {
	logger := klog.New()

	if service.VendorVersion == "" {
		logger.Errorf("VendorVersion must be set at compile time")
		return 1
	}
	logger.Infof("Driver vendor %v %v", service.VendorName, service.VendorVersion)

	restConfig, err := config.GetConfig()
	if err != nil {
		logger.Errorf("Failed to obtain Kubernetes connection configuration (%v)", err)
		return 1
	}

	opts := manager.Options{
		Namespace:          *namespace,
		MetricsBindAddress: "0",
	}

	ctx := signals.SetupSignalHandler()

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(restConfig, opts)
	if err != nil {
		logger.Errorf("Failed to create manager (%v)", err)
		return 1
	}
	go func() {
		if err := mgr.Start(ctx); err != nil {
			logger.Errorf("Failed to start manager (%v)", err)
		} else {
			logger.Infof("Manager stopped.")
		}
	}()

	// get the node object by name and pass the VM ID because it is the node
	// id from the storage perspective. It will be used for attaching disks
	var nodeId string
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		logger.Errorf("Failed to create Kubernetes client (%v)", err)
		return 1
	}

	if *nodeName != "" {
		get, err := clientSet.CoreV1().Nodes().Get(context.Background(), *nodeName, metav1.GetOptions{})
		if err != nil {
			logger.Errorf("Failed to fetch current node ID (%v)", err)
			return 1
		}
		nodeId = get.Status.NodeInfo.SystemUUID
	}

	if *secretNamespace == "" {
		fh, err := os.Open("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
		if err != nil {
			logger.Errorf("--secret-namespace is not set and /var/run/secrets/kubernetes.io/serviceaccount/namespace could not be opened (%v)", err)
			return 1
		}
		namespaceData, err := ioutil.ReadAll(fh)
		if err != nil {
			_ = fh.Close()
			logger.Errorf("--secret-namespace is not set and failed to read /var/run/secrets/kubernetes.io/serviceaccount/namespace (%v)", err)
			return 1
		}
		_ = fh.Close()
		nsData := string(namespaceData)
		secretNamespace = &nsData
		if *secretNamespace == "" {
			logger.Errorf("--secret-namespace is not set and /var/run/secrets/kubernetes.io/serviceaccount/namespace is empty")
			return 1
		}
	}

	driver, err := service.NewOvirtCSIDriver(
		logger,
		k8sovirtcredentialsmonitor.ConnectionConfig{
			Config: restConfig,
		},
		k8sovirtcredentialsmonitor.OVirtSecretConfig{
			Namespace: *secretNamespace,
			Name:      *secretName,
		},
		nodeId,
		*endpoint,
	)

	running := make(chan struct{})
	stopping := make(chan struct{})
	stopped := make(chan struct{})

	driver.Run(ctx, running, stopping, stopped)

	if err := driver.LastError(); err != nil {
		logger.Errorf(err.Error())
		return 1
	}
	return 0
}
