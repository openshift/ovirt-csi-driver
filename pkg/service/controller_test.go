package service_test

import (
	"context"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/ovirt/csi-driver/pkg/service"
	ovirtclient "github.com/ovirt/go-ovirt-client"
)

func getController(helper ovirtclient.TestHelper) csi.ControllerServer {
	return service.NewControllerServer(func() (ovirtclient.Client, error) {
		return helper.GetClient(), nil
	})
}

func TestVolumeCreation(t *testing.T) {
	helper := getMockHelper(t)
	controller := getController(helper)

	createVolumeResponse, err := createTestVolume(helper, controller)
	if err != nil {
		t.Fatalf("failed to create test volume (%v)", err)
	}
	if createVolumeResponse.Volume.CapacityBytes < 4096 {
		t.Fatalf("created volume is too small (%v)", err)
	}
	diskList, err := helper.GetClient().ListDisks()
	if err != nil {
		t.Fatalf("failed to list disks (%v)", err)
	}
	if len(diskList) != 1 {
		t.Fatalf("incorrect number of disks created (%d)", len(diskList))
	}
	disk := diskList[0]
	if disk.TotalSize() < 4096 {
		t.Fatalf("incorrect disk size on the backend: %d", disk.TotalSize())
	}
}

func TestDeleteVolume(t *testing.T) {
	helper := getMockHelper(t)
	controller := getController(helper)
	createVolumeResponse, err := createTestVolume(helper, controller)
	if err != nil {
		t.Fatalf("failed to create test volume (%v)", err)
	}

	_, err = controller.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: createVolumeResponse.Volume.VolumeId,
	})
	if err != nil {
		t.Fatalf("failed to delete volume (%v)", err)
	}
	diskList, err := helper.GetClient().ListDisks()
	if err != nil {
		t.Fatalf("failed to list disks (%v)", err)
	}
	if len(diskList) != 0 {
		t.Fatalf("disk wasn't deleted, number of disks on backend: %d", len(diskList))
	}
}

func TestDeleteNonExistentVolume(t *testing.T) {
	helper := getMockHelper(t)
	controller := getController(helper)

	_, err := controller.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: "doesn't exists",
	})
	if err != nil {
		t.Fatalf("deleting volume which doesn't exist failed (%v)", err)
	}
}

func TestControllerPublishVolume(t *testing.T) {
	helper := getMockHelper(t)
	publishRequest := &csi.ControllerPublishVolumeRequest{
		VolumeId: "",
		NodeId:   "",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		Readonly: true,
	}
	vmId, diskId, err := publishTestVolume(helper, publishRequest)
	if err != nil {
		t.Fatalf("failed to publish the volume (%v)", err)
	}

	attachments, err := helper.GetClient().ListDiskAttachments(vmId, ovirtclient.ContextStrategy(context.Background()))
	if err != nil {
		t.Fatalf("failed to list disk attachments (%v)", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("incorrect number of disk attachmanets (%d)", len(attachments))
	}
	attachment := attachments[0]
	if attachment.DiskID() != diskId {
		t.Fatalf("incorrect disk ID: %s", attachment.DiskID())
	}
	disk, err := attachment.Disk()
	if err != nil {
		t.Fatalf("failed to get disk (%v)", err)
	}
	if disk.TotalSize() < 4096 {
		t.Fatalf("incorrect disk size on the backend: %d", disk.TotalSize())
	}
}

func TestControllerPublishVolumeTwice(t *testing.T) {
	helper := getMockHelper(t)

	publishRequest := &csi.ControllerPublishVolumeRequest{
		VolumeId: "",
		NodeId:   "",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		Readonly: true,
	}

	// Publish volume.
	_, _, err := publishTestVolume(helper, publishRequest)
	if err != nil {
		t.Fatalf("failed to publish the volume (%v)", err)
	}

	// Publish volume again, should succeed.
	_, _, err = publishTestVolume(helper, publishRequest)
	if err != nil {
		t.Fatalf("failed to publish volume which was already published (%v)", err)
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	helper := getMockHelper(t)
	publishRequest := &csi.ControllerPublishVolumeRequest{
		VolumeId: "",
		NodeId:   "",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		Readonly: true,
	}
	vmId, diskId, err := publishTestVolume(helper, publishRequest)
	if err != nil {
		t.Fatalf("failed to publish the volume (%v)", err)
	}

	controller := getController(helper)
	_, err = controller.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: diskId,
		NodeId:   vmId,
	})
	if err != nil {
		t.Fatalf("failed to unpublish the volume (%v)", err)
	}

	attachments, err := helper.GetClient().ListDiskAttachments(vmId, ovirtclient.ContextStrategy(context.Background()))
	if err != nil {
		t.Fatalf("failed to list disk attachments (%v)", err)
	}
	if len(attachments) != 0 {
		t.Fatalf("volume is still published")
	}
}

func TestControllerUnpublishVolumeTwice(t *testing.T) {
	helper := getMockHelper(t)
	publishRequest := &csi.ControllerPublishVolumeRequest{
		VolumeId: "",
		NodeId:   "",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		Readonly: true,
	}

	// Publish volume.
	vmId, diskId, err := publishTestVolume(helper, publishRequest)
	if err != nil {
		t.Fatalf("failed to publish the volume (%v)", err)
	}

	controller := getController(helper)

	// Unpublish volume.
	_, err = controller.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: diskId,
		NodeId:   vmId,
	})
	if err != nil {
		t.Fatalf("failed to unpublish the volume (%v)", err)
	}

	// Unpublish volume second time, should succeed.
	_, err = controller.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: diskId,
		NodeId:   vmId,
	})
	if err != nil {
		t.Fatalf("failed to unpublish volume which was already unpublished (%v)", err)
	}
}

func TestControllerExpandVolume(t *testing.T) {
	helper := getMockHelper(t)

	publishRequest := &csi.ControllerPublishVolumeRequest{
		VolumeId: "",
		NodeId:   "",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{
					FsType: "ext4",
				},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		Readonly: true,
	}
	_, diskId, err := publishTestVolume(helper, publishRequest)
	controller := getController(helper)
	disk, err := helper.GetClient().GetDisk(diskId)
	if err != nil {
		t.Fatalf("failed to get disks (%v)", err)
	}

	expandedSize := int64(2 * disk.TotalSize())
	expandResp, err := controller.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: diskId,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: expandedSize,
			LimitBytes:    expandedSize,
		},
	})
	if err != nil {
		t.Fatalf("failed to expand volume (%v)", err)
	}

	if !expandResp.NodeExpansionRequired {
		t.Fatalf("node expansion not required (%v)", err)
	}
	if expandResp.CapacityBytes < expandedSize {
		t.Fatalf("volume wasn't expanded (%v)", err)
	}
	disk, err = helper.GetClient().GetDisk(diskId)
	if err != nil {
		t.Fatalf("failed to get disks (%v)", err)
	}
	if disk.TotalSize() < uint64(expandedSize) {
		t.Fatalf("incorrect disk size on the backend: %d, expanded size: %d", disk.TotalSize(), expandedSize)
	}
}

func TestControllerExpandVolumeToSmallerSize(t *testing.T) {
	helper := getMockHelper(t)
	controller := getController(helper)

	createVolumeResponse, err := createTestVolume(helper, controller)
	if err != nil {
		t.Fatalf("failed to create test volume (%v)", err)
	}

	expandResp, err := controller.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: createVolumeResponse.Volume.VolumeId,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1024,
			LimitBytes:    1024,
		},
	})
	if err != nil {
		t.Fatalf("failed to expand volume (%v)", err)
	}

	if expandResp.CapacityBytes < 4096 {
		t.Fatalf("volume was shrunk, which shouln't be possible (%v)", err)
	}
	if expandResp.NodeExpansionRequired {
		t.Fatalf("node requires expansion, while the request was to shrink the volume (%v)", err)
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	helper := getMockHelper(t)
	controller := getController(helper)
	capsResp, err := controller.ControllerGetCapabilities(context.Background(), &csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("failed to get controller capabilities (%v)", err)
	}
	caps := capsResp.GetCapabilities()
	assertHasCapability(caps, csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME, t)
	assertHasCapability(caps, csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME, t)
	assertHasCapability(caps, csi.ControllerServiceCapability_RPC_EXPAND_VOLUME, t)
}

func createTestVolume(helper ovirtclient.TestHelper, controller csi.ControllerServer) (*csi.CreateVolumeResponse, error) {
	testStorageDomain, err := helper.GetClient().GetStorageDomain(helper.GetStorageDomainID())
	if err != nil {
		return nil, err
	}

	createVolumeResponse, err := controller.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test",
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 4096,
			LimitBytes:    4096,
		},
		Parameters: map[string]string{
			"storageClass":      "ovirt-test-domain",
			"storageDomainName": testStorageDomain.Name(),
			"thinProvisioning":  "true",
		},
	})
	if err != nil {
		return nil, err
	}

	return createVolumeResponse, nil
}

func publishTestVolume(helper ovirtclient.TestHelper, publishReq *csi.ControllerPublishVolumeRequest) (string, string, error) {
	if publishReq.NodeId == "" {
		vm, err := helper.GetClient().CreateVM(
			helper.GetClusterID(),
			helper.GetBlankTemplateID(),
			"test",
			ovirtclient.CreateVMParams(),
		)
		if err != nil {
			return "", "", err
		}
		publishReq.NodeId = vm.ID()
	}
	controller := getController(helper)
	if publishReq.VolumeId == "" {
		createVolumeResponse, err := createTestVolume(helper, controller)
		if err != nil {
			return "", "", err
		}
		publishReq.VolumeId = createVolumeResponse.Volume.VolumeId
	}

	_, err := controller.ControllerPublishVolume(context.Background(), publishReq)
	if err != nil {
		return "", "", err
	}

	return publishReq.NodeId, publishReq.VolumeId, nil
}

func assertHasCapability(caps []*csi.ControllerServiceCapability, capType csi.ControllerServiceCapability_RPC_Type, t *testing.T) {
	for _, cap := range caps {
		if cap.GetRpc().GetType() == capType {
			return
		}
	}
	t.Fatalf("missing capability: %v", capType)
}
