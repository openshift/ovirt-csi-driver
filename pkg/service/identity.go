package service

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
	ovirtclient "github.com/ovirt/go-ovirt-client"
	log "github.com/ovirt/go-ovirt-client-log/v2"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewIdentityServer(logger log.Logger, getClient func() (ovirtclient.Client, error)) csi.IdentityServer {
	return &identityService{
		logger:    logger,
		getClient: getClient,
	}
}

//identityService of ovirt-csi-driver
type identityService struct {
	logger    log.Logger
	getClient func() (ovirtclient.Client, error)
}

//GetPluginInfo returns the vendor name and version - set in build time
func (i *identityService) GetPluginInfo(context.Context, *csi.GetPluginInfoRequest) (
	*csi.GetPluginInfoResponse,
	error,
) {
	return &csi.GetPluginInfoResponse{
		Name:          VendorName,
		VendorVersion: VendorVersion,
	}, nil
}

//GetPluginCapabilities declares the plugins capabilities
func (i *identityService) GetPluginCapabilities(
	context.Context,
	*csi.GetPluginCapabilitiesRequest,
) (*csi.GetPluginCapabilitiesResponse, error) {
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
		i.logger.Infof("CSI driver is not ready, client not available yet.")
		return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: false}}, nil
	}
	if err := client.Test(); err != nil {
		i.logger.Warningf("oVirt connection failed (%v).", err)
		return nil, status.Error(codes.FailedPrecondition, "Could not get connection to ovirt-engine")
	}
	return &csi.ProbeResponse{Ready: &wrappers.BoolValue{Value: true}}, nil
}
