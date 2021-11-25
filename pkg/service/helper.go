package service

import (
	"fmt"

	ovirtclient "github.com/ovirt/go-ovirt-client"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func isNotFound(err error) bool {
	return ovirtclient.HasErrorCode(err, ovirtclient.ENotFound)
}

func diskAttachmentByVmAndDisk(ctx context.Context, ovirtClient ovirtclient.Client,
	vmId string, diskId string) (ovirtclient.DiskAttachment, error) {
	attachments, err := ovirtClient.ListDiskAttachments(vmId, ovirtclient.ContextStrategy(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get disk attachment by disk %s for VM %s, error: %w", diskId, vmId, err)
	}
	for _, attachment := range attachments {
		if diskId == attachment.DiskID() {
			return attachment, nil
		}
	}
	return nil, nil
}

func findDiskAttachmentByDiskInVMs(ctx context.Context, ovirtClient ovirtclient.Client, diskId string, vms []ovirtclient.VM) (ovirtclient.DiskAttachment, error) {
	for _, vm := range vms {
		diskAttachment, err := diskAttachmentByVmAndDisk(ctx, ovirtClient, vm.ID(), diskId)
		if err != nil {
			return nil, fmt.Errorf("error while searching disk attachment for disk %s in VM %s, error: %w", diskId, vm.ID(), err)
		}
		if diskAttachment != nil {
			return diskAttachment, nil
		}
	}
	return nil, nil
}

func findDiskAttachmentByDiskInCluster(ctx context.Context, ovirtClient ovirtclient.Client, diskId string) (ovirtclient.DiskAttachment, error) {
	vms, err := ovirtClient.ListVMs(ovirtclient.ContextStrategy(ctx))
	if err != nil {
		return nil, fmt.Errorf("error while listing VMs %w", err)
	}
	if len(vms) == 0 {
		return nil, errors.New("Didn't found any vms in engine")
	}
	return findDiskAttachmentByDiskInVMs(ctx, ovirtClient, diskId, vms)
}

func getStorageDomainByName(
	ctx context.Context,
	ovirtClient ovirtclient.Client,
	storageDomainName string) (ovirtclient.StorageDomain, error) {
	sds, err := ovirtClient.ListStorageDomains(ovirtclient.ContextStrategy(ctx))
	if err != nil {
		return nil, err
	}
	for _, sd := range sds {
		if sd.Name() == storageDomainName {
			return sd, nil
		}
	}
	return nil, nil
}

func isFileDomain(storageType ovirtclient.StorageDomainType) bool {
	for _, v := range ovirtclient.FileStorageDomainTypeValues() {
		if storageType == v {
			return true
		}
	}
	return false
}
