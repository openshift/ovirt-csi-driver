package service

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
	ovirtclient "github.com/ovirt/go-ovirt-client"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func NewIdentityServer(getClient func() (ovirtclient.Client, error)) csi.IdentityServer {
	return &identityService{
		getClient: getClient,
	}
}

//identityService of ovirt-csi-driver
type identityService struct {
	getClient func() (ovirtclient.Client, error)
}

//GetPluginInfo returns the vendor name and version - set in build time
func (i *identityService) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          VendorName,
		VendorVersion: VendorVersion,
	}, nil
}

//GetPluginCapabilities declares the plugins capabilities
func (i *identityService) GetPluginCapabilities(context.Context, *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_VolumeExpansion_{
					VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
						Type: csi.PluginCapability_VolumeExpansion_ONLINE,
					},
				},
			},
		},
	}, nil
}

// Probe checks the state of the connection to ovirt-engine
func (i *identityService) Probe(_ context.Context, _ *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	client, err := i.getClient()
	if err != nil {
		return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: false}}, nil
	}
	if err := client.Test(); err != nil {
		klog.Errorf("Could not get connection %v", err)
		return nil, status.Error(codes.FailedPrecondition, "Could not get connection to ovirt-engine")
	}
	return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: true}}, nil
}
