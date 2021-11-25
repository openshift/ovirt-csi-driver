package service

import (
	"fmt"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	ovirtclient "github.com/ovirt/go-ovirt-client"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	ParameterStorageDomainName = "storageDomainName"
	ParameterThinProvisioning  = "thinProvisioning"
	minimumDiskSize            = 1 * 1024 * 1024
)

func NewControllerServer(getClient func() (ovirtclient.Client, error)) csi.ControllerServer {
	return &controllerService{
		getClient: getClient,
	}
}

// controllerService implements the controller interface.
type controllerService struct {
	getClient func() (ovirtclient.Client, error)
}

var ControllerCaps = []csi.ControllerServiceCapability_RPC_Type{
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME, // attach/detach
	csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
}

//CreateVolume creates the disk for the request, unattached from any VM
func (c *controllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.Infof("Creating disk %s", req.Name)
	storageDomainName := req.Parameters[ParameterStorageDomainName]
	if len(storageDomainName) == 0 {
		return nil, fmt.Errorf("error required storageClass paramater %s wasn't set",
			ParameterStorageDomainName)
	}
	diskName := req.Name
	if len(diskName) == 0 {
		return nil, fmt.Errorf("error required request parameter Name was not provided")
	}
	thinProvisioning, err := strconv.ParseBool(req.Parameters[ParameterThinProvisioning])
	if req.Parameters[ParameterThinProvisioning] == "" {
		// In case thin provisioning is not set, we default to true
		thinProvisioning = true
	}
	if err != nil {
		return nil, fmt.Errorf(
			"failed to parse storage class field %s, expected 'true' or 'false' but got %s",
			ParameterThinProvisioning, req.Parameters[ParameterThinProvisioning])
	}
	requiredSize := req.CapacityRange.GetRequiredBytes()
	client, err := c.getClient()
	if err != nil {
		// Client is not ready yet.
		return nil, err
	}
	// Check if a disk with the same name already exist
	disks, err := client.ListDisksByAlias(diskName, ovirtclient.ContextStrategy(ctx))
	if err != nil {
		msg := fmt.Errorf("error while finding disk %s by name, error: %w", diskName, err)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	if len(disks) > 1 {
		msg := fmt.Errorf(
			"found more then one disk with the name %s,"+
				"please contanct the oVirt admin to check the name duplication", diskName)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	var disk ovirtclient.Disk
	// If disk doesn't already exist then create it
	if len(disks) == 0 {
		disk, err = c.createDisk(ctx, diskName, storageDomainName, requiredSize, thinProvisioning)
		if err != nil {
			klog.Errorf(err.Error())
			return nil, err
		}
	} else {
		disk = disks[0]
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: int64(disk.ProvisionedSize()),
			VolumeId:      disk.ID(),
		},
	}, nil
}

func (c *controllerService) createDisk(
	ctx context.Context, diskName string, storageDomainName string,
	size int64, thinProvisioning bool) (ovirtclient.Disk, error) {
	var err error
	params := ovirtclient.CreateDiskParams()
	params, err = params.WithSparse(thinProvisioning)
	if err != nil {
		return nil, err
	}
	params, err = params.WithAlias(diskName)
	if err != nil {
		return nil, err
	}

	provisionedSize := size
	if provisionedSize < minimumDiskSize {
		provisionedSize = minimumDiskSize
	}

	ovirtClient, err := c.getClient()
	if err != nil {
		return nil, err
	}

	sd, err := getStorageDomainByName(ctx, ovirtClient, storageDomainName)
	if err != nil {
		return nil, fmt.Errorf("failed searching for storage domain with name %s, error: %w", storageDomainName, err)
	}
	if sd == nil {
		return nil, fmt.Errorf("failed searching for storage domain with name %s, error: %w", storageDomainName, err)
	}
	imageFormat := handleCreateVolumeImageFormat(sd.StorageType(), thinProvisioning)

	disk, err := ovirtClient.CreateDisk(
		sd.ID(),
		imageFormat,
		uint64(provisionedSize),
		params,
		ovirtclient.ContextStrategy(ctx))
	if err != nil {
		return nil, fmt.Errorf("creating oVirt disk %s, error: %w", diskName, err)
	}
	klog.Infof("Finished creating disk %s", diskName)
	return disk, nil

}

func handleCreateVolumeImageFormat(
	storageType ovirtclient.StorageDomainType,
	thinProvisioning bool) ovirtclient.ImageFormat {
	// Use COW diskformat only when thin provisioning is requested and storage domain
	// is a non file storage type (for example ISCSI)
	if !isFileDomain(storageType) && thinProvisioning {
		return ovirtclient.ImageFormatCow
	} else {
		return ovirtclient.ImageFormatRaw
	}
}

//DeleteVolume removed the disk from oVirt
func (c *controllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	vId := req.VolumeId
	if len(vId) == 0 {
		return nil, fmt.Errorf("error required paramater VolumeId wasn't set")
	}
	klog.Infof("Removing disk %s", vId)

	ovirtClient, err := c.getClient()
	if err != nil {
		return nil, err
	}

	// idempotence first - see if disk already exists, ovirt creates disk by name(alias in ovirt as well)
	// Check if a disk with the same name already exist
	_, err = ovirtClient.GetDisk(vId, ovirtclient.ContextStrategy(ctx))
	if err != nil {
		if isNotFound(err) {
			// if disk doesn't exist we're done
			return &csi.DeleteVolumeResponse{}, nil
		}
		msg := fmt.Errorf("error while finding disk %s by id, error: %w", vId, err)
		klog.Errorf(msg.Error())
		return nil, msg
	}

	err = ovirtClient.RemoveDisk(vId, ovirtclient.ContextStrategy(ctx))
	if err != nil {
		msg := fmt.Errorf("failed removing disk %s by id, error: %w", vId, err)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	klog.Infof("Finished removing disk %s", vId)
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume takes a volume, which is an oVirt disk, and attaches it to a node, which is an oVirt VM.
func (c *controllerService) ControllerPublishVolume(
	ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	vId := req.VolumeId
	if len(vId) == 0 {
		return nil, fmt.Errorf("error required request paramater VolumeId wasn't set")
	}
	nId := req.NodeId
	if len(nId) == 0 {
		return nil, fmt.Errorf("error required request paramater NodeId wasn't set")
	}
	ovirtClient, err := c.getClient()
	if err != nil {
		return nil, err
	}
	klog.Infof("Attaching Disk %s to VM %s", vId, nId)
	da, err := diskAttachmentByVmAndDisk(ctx, ovirtClient, nId, vId)
	if err != nil {
		msg := fmt.Errorf("failed finding disk attachment, error: %w", err)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	if da != nil {
		klog.Infof("Disk %s is already attached to VM %s, returning OK", vId, nId)
		return &csi.ControllerPublishVolumeResponse{}, nil
	}
	params := ovirtclient.CreateDiskAttachmentParams()
	params, err = params.WithActive(true)
	if err != nil {
		return nil, err
	}
	params, err = params.WithBootable(false)
	if err != nil {
		return nil, err
	}
	_, err = ovirtClient.CreateDiskAttachment(
		nId,
		vId,
		ovirtclient.DiskInterfaceVirtIOSCSI,
		params,
		ovirtclient.ContextStrategy(ctx))
	if err != nil {
		msg := fmt.Errorf("failed creating disk attachment, error: %w", err)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	klog.Infof("Attached Disk %v to VM %s", vId, nId)
	return &csi.ControllerPublishVolumeResponse{}, nil
}

//ControllerUnpublishVolume detaches the disk from the VM.
func (c *controllerService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	vId := req.VolumeId
	if len(vId) == 0 {
		return nil, fmt.Errorf("error required request paramater VolumeId wasn't set")
	}
	nId := req.NodeId
	if len(nId) == 0 {
		return nil, fmt.Errorf("error required request paramater NodeId wasn't set")
	}
	klog.Infof("Detaching Disk %s from VM %s", vId, nId)
	ovirtClient, err := c.getClient()
	if err != nil {
		return nil, err
	}
	attachment, err := diskAttachmentByVmAndDisk(ctx, ovirtClient, nId, vId)
	if err != nil {
		msg := fmt.Errorf("failed finding disk attachment, error: %w", err)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	if attachment == nil {
		klog.Infof("Disk attachment %s for VM %s already detached, returning OK", vId, nId)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}
	err = ovirtClient.RemoveDiskAttachment(nId, attachment.ID(), ovirtclient.ContextStrategy(ctx))
	if err != nil {
		msg := fmt.Errorf("failed removing disk attachment %s, error: %w", attachment.ID(), err)
		klog.Errorf(msg.Error())
		return nil, msg
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

//ValidateVolumeCapabilities
func (c *controllerService) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ListVolumes
func (c *controllerService) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//GetCapacity
func (c *controllerService) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//CreateSnapshot
func (c *controllerService) CreateSnapshot(context.Context, *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//DeleteSnapshot
func (c *controllerService) DeleteSnapshot(context.Context, *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ListSnapshots
func (c *controllerService) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

//ControllerExpandVolume
func (c *controllerService) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}
	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range not provided")
	}
	newSize := capRange.GetRequiredBytes()

	ovirtClient, err := c.getClient()
	if err != nil {
		return nil, err
	}

	klog.Infof("Expanding volume %v to %v bytes.", volumeID, newSize)
	disk, err := ovirtClient.GetDisk(volumeID, ovirtclient.ContextStrategy(ctx))
	if err != nil {
		if isNotFound(err) {
			msg := fmt.Errorf("disk %s wasn't found", volumeID)
			klog.Error(msg)
			return nil, status.Error(codes.NotFound, msg.Error())
		}
		msg := fmt.Errorf("error while finding disk %s, error: %w", volumeID, err)
		klog.Error(msg)
		return nil, status.Error(codes.Internal, msg.Error())
	}
	// According to the CSI spec, if the volume is already larger than or equal to the target capacity of
	// the expansion request, the plugin SHOULD reply 0 OK.
	diskSize := int64(disk.TotalSize())
	if diskSize >= newSize {
		klog.Infof("Volume %s of size %d is larger than requested size %d, no need to extend",
			volumeID, diskSize, newSize)
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         diskSize,
			NodeExpansionRequired: false}, nil
	}

	params := ovirtclient.UpdateDiskParams()
	params, err = params.WithProvisionedSize(uint64(newSize))
	if err != nil {
		return nil, err
	}
	disk, err = ovirtClient.UpdateDisk(volumeID, params, ovirtclient.ContextStrategy(ctx))
	if err != nil {
		return nil, status.Errorf(codes.ResourceExhausted, "failed to expand volume %s: %v", volumeID, err)
	}
	klog.Infof("Expanded Disk %v to %v bytes", volumeID, newSize)
	nodeExpansionRequired, err := c.isNodeExpansionRequired(ctx, req.GetVolumeCapability(), volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed checking if node expansion is required for volume %s (%v)", volumeID, err)
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         newSize,
		NodeExpansionRequired: nodeExpansionRequired}, nil
}

func (c *controllerService) isNodeExpansionRequired(ctx context.Context, vc *csi.VolumeCapability, volumeID string) (bool, error) {
	// If this is a raw block device, no expansion should be necessary on the node
	if vc != nil && vc.GetBlock() != nil {
		return false, nil
	}
	// If disk is not attached to any VM then no need to expand
	ovirtClient, err := c.getClient()
	if err != nil {
		return false, err
	}
	diskAttachment, err := findDiskAttachmentByDiskInCluster(ctx, ovirtClient, volumeID)
	if err != nil {
		return false, fmt.Errorf("error while searching disk attachment for volume %s, error %w",
			volumeID, err)
	}
	if diskAttachment == nil {
		return false, nil
	}
	return true, nil
}

//ControllerGetCapabilities
func (c *controllerService) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	caps := make([]*csi.ControllerServiceCapability, 0, len(ControllerCaps))
	for _, capability := range ControllerCaps {
		caps = append(
			caps,
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: capability,
					},
				},
			},
		)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}
