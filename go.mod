module github.com/ovirt/csi-driver

go 1.16

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/golang/protobuf v1.5.2
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	github.com/onsi/ginkgo v1.16.4 // indirect
	github.com/onsi/gomega v1.13.0 // indirect
	github.com/ovirt/go-ovirt v0.0.0-20210809163552-d4276e35d3db // indirect
	github.com/ovirt/go-ovirt-client v0.7.2-0.20211125105627-fccb94e139ab
	github.com/ovirt/go-ovirt-client-log-klog v1.0.0
	github.com/ovirt/go-ovirt-client-log/v2 v2.2.0
	github.com/ovirt/k8sovirtcredentialsmonitor v0.3.1-0.20211125152404-ad789a253681
	github.com/pkg/errors v0.9.1
	golang.org/x/net v0.0.0-20210428140749-89ef3d95e781
	golang.org/x/sys v0.0.0-20210603081109-ebe580a85c40
	google.golang.org/grpc v1.29.1
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v0.21.2
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.2
)
