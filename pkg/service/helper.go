package service

import (
	"fmt"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	ovirtsdk "github.com/ovirt/go-ovirt"
)

const (
	globalInfrastuctureName = "cluster"
)

func diskAttachmentByVmAndDisk(connection *ovirtsdk.Connection, vmId string, diskId string) (*ovirtsdk.DiskAttachment, error) {
	vmService := connection.SystemService().VmsService().VmService(vmId)
	attachments, err := vmService.DiskAttachmentsService().List().Send()
	if err != nil {
		return nil, fmt.Errorf("failed to get disk attachment by disk %s for VM %s, error: %w", diskId, vmId, err)
	}

	for _, attachment := range attachments.MustAttachments().Slice() {
		if diskId == attachment.MustDisk().MustId() {
			return attachment, nil
		}
	}
	return nil, nil
}

func findDiskAttachmentByDiskInVMs(conn *ovirtsdk.Connection, diskId string, vms []*ovirtsdk.Vm) (*ovirtsdk.DiskAttachment, error) {
	for _, vm := range vms {
		diskAttachment, err := diskAttachmentByVmAndDisk(conn, vm.MustId(), diskId)
		if err != nil {
			klog.Error(err)
			return nil, errors.Wrap(err, "failed finding disk attachments")
		}
		if diskAttachment != nil {
			return diskAttachment, nil
		}
	}
	return nil, nil
}

func findDiskAttachmentByDiskInCluster(ctx context.Context, c client.Client, conn *ovirtsdk.Connection, diskId string) (*ovirtsdk.DiskAttachment, error) {
	var vms []*ovirtsdk.Vm
	searchTerm := ""
	tag, err := getClusterTag(ctx, c)
	if err == nil {
		searchTerm = fmt.Sprintf("tag=%s", tag)
	}
	vms, err = getVms(conn, searchTerm)
	if err != nil {
		klog.Error(err)
		return nil, errors.Wrap(err, "failed searching VMs")
	}
	if len(vms) == 0 {
		klog.Error(err)
		return nil, errors.New("Didn't found any vms in engine")
	}
	return findDiskAttachmentByDiskInVMs(conn, diskId, vms)
}

// getVms return a list of VMs available on the oVirt engine.
// searchTerm is an optional search term in the oVirt engine, much by a valid search term for example `tag=example`
func getVms(connection *ovirtsdk.Connection, searchTerm string) ([]*ovirtsdk.Vm, error) {
	vmsListRequest := connection.SystemService().VmsService().List()
	if searchTerm != "" {
		vmsListRequest = vmsListRequest.Search(searchTerm)
	}
	vmsResp, err := vmsListRequest.Send()
	if err != nil {
		return nil, fmt.Errorf("failed listing VMs, error: %w", err)
	}
	vms, ok := vmsResp.Vms()
	if !ok {
		return nil, errors.New("failed getting VMs from request")
	}
	return vms.Slice(), nil
}

func getClusterTag(ctx context.Context, c client.Client) (string, error) {
	infra := &configv1.Infrastructure{}
	objKey := client.ObjectKey{Name: globalInfrastuctureName}
	if err := c.Get(ctx, objKey, infra); err != nil {
		return "", errors.Wrap(err, "error getting infrastucture data")
	}
	return infra.Status.InfrastructureName, nil
}

func expandDisk(ctx context.Context, conn *ovirtsdk.Connection, disk *ovirtsdk.Disk, bytes int64) error {
	correlationID := fmt.Sprintf("disk_resize_%s", utilrand.String(5))

	// Check That the disk is ready for expansion
	if err := waitForDiskStatusOk(ctx, conn, disk.MustId()); err != nil {
		return fmt.Errorf("failed to expand disk attachment %s, error: %w", disk.MustId(), err)
	}
	disk.SetProvisionedSize(bytes)
	_, err := conn.SystemService().DisksService().DiskService(disk.MustId()).
		Update().Disk(disk).
		Query("correlation_id", correlationID).
		Send()
	if err != nil {
		return fmt.Errorf("failed to expand disk %s, error: %w", disk.MustId(), err)
	}

	finished, err := checkJobFinished(ctx, conn, correlationID)
	if err != nil {
		return fmt.Errorf("failed to expand disk %s, error: %w", disk.MustId(), err)
	}
	if !finished {
		return fmt.Errorf("expand disk %s expansion job didn't finish, error: %w",
			disk.MustId(), err)
	}
	if err = waitForDiskStatusOk(ctx, conn, disk.MustId()); err != nil {
		return fmt.Errorf("failed to expand disk %s, error: %w", disk.MustId(), err)
	}
	return nil
}

func attachDiskToVM(ctx context.Context, conn *ovirtsdk.Connection, diskID string, vmID string) error {
	vmService := conn.SystemService().VmsService().VmService(vmID)

	attachmentBuilder := ovirtsdk.NewDiskAttachmentBuilder().
		DiskBuilder(ovirtsdk.NewDiskBuilder().Id(diskID)).
		Interface(ovirtsdk.DISKINTERFACE_VIRTIO_SCSI).
		Bootable(false).
		Active(true)

	correlationID := fmt.Sprintf("disk_attach_%s", utilrand.String(5))
	_, err := vmService.
		DiskAttachmentsService().
		Add().
		Attachment(attachmentBuilder.MustBuild()).
		Query("correlation_id", correlationID).
		Send()
	if err != nil {
		return fmt.Errorf("failed attaching disk %s to VM %s, error: %w", diskID, vmID, err)
	}
	finished, err := checkJobFinished(ctx, conn, correlationID)
	if err != nil {
		return fmt.Errorf("failed attaching disk %s to VM %s, error: %w", diskID, vmID, err)
	}
	if !finished {
		return fmt.Errorf("failed attaching disk %s to VM %s, job %s didn't finish, error: %w",
			diskID, vmID, correlationID, err)
	}
	return nil
}

func detachDiskFromVMByDiskAttachment(ctx context.Context, conn *ovirtsdk.Connection, attachment *ovirtsdk.DiskAttachment, vmID string) error {
	correlationID := fmt.Sprintf("disk_detach_%s", utilrand.String(5))
	_, err := conn.SystemService().VmsService().VmService(vmID).
		DiskAttachmentsService().
		AttachmentService(attachment.MustId()).
		Remove().
		Query("correlation_id", correlationID).
		Send()
	if err != nil {
		return fmt.Errorf("failed to detach disk %s from VM %s, error: %w", attachment.MustId(), vmID, err)
	}
	finished, err := checkJobFinished(ctx, conn, correlationID)
	if err != nil {
		return fmt.Errorf("failed to detach disk %s from VM %s, error: %w", attachment.MustId(), vmID, err)
	}
	if !finished {
		return fmt.Errorf("failed to detach disk %s from VM %s, job %s didn't finish, error: %w",
			attachment.MustId(), vmID, correlationID, err)
	}
	return nil
}

func getDiskFromDiskAttachment(conn *ovirtsdk.Connection, diskAttachment *ovirtsdk.DiskAttachment) (*ovirtsdk.Disk, error) {
	disk, err := conn.FollowLink(diskAttachment.MustDisk())
	if err != nil {
		return nil, fmt.Errorf("failed to following disk attachment %v disk href %w", diskAttachment.MustId(), err)
	}
	d, ok := disk.(*ovirtsdk.Disk)
	if !ok {
		return nil, errors.New("couldn't retrieve disk from attachment")
	}
	return d, nil
}

func getDiskByName(conn *ovirtsdk.Connection, diskName string) (*ovirtsdk.Disk, error) {
	diskByName, err := conn.SystemService().DisksService().List().Search(diskName).Send()
	if err != nil {
		return nil, err
	}
	disks, ok := diskByName.Disks()
	if !ok {
		return nil, fmt.Errorf(
			"error, failed searching for disk with name %s", diskName)
	}
	if len(disks.Slice()) > 1 {
		return nil, fmt.Errorf(
			"error, found more then one disk with the name %s, please use ID instead", diskName)
	}
	if len(disks.Slice()) == 0 {
		return nil, nil
	}
	return disks.Slice()[0], nil
}

func getStorageDomainByName(conn *ovirtsdk.Connection, storageDomainName string) (*ovirtsdk.StorageDomain, error) {
	sdByName, err := conn.SystemService().StorageDomainsService().List().Search(storageDomainName).Send()
	if err != nil {
		return nil, err
	}
	sd, ok := sdByName.StorageDomains()
	if !ok {
		return nil, fmt.Errorf(
			"error, failed searching for storage domain with name %s", storageDomainName)
	}
	if len(sd.Slice()) > 1 {
		return nil, fmt.Errorf(
			"error, found more then one storage domain with the name %s, please use ID instead", storageDomainName)
	}
	if len(sd.Slice()) == 0 {
		return nil, nil
	}
	return sd.Slice()[0], nil
}

func waitForDiskStatusOk(ctx context.Context, conn *ovirtsdk.Connection, diskID string) error {
	var lastStatus ovirtsdk.DiskStatus
	diskService := conn.SystemService().DisksService().DiskService(diskID)
	for {
		req, err := diskService.Get().Send()
		if err != nil {
			return fmt.Errorf("failed getting disk %s, error %w", diskID, err)
		}
		disk, ok := req.Disk()
		if !ok {
			return fmt.Errorf("disk with ID %s doesn't exist", diskID)
		}
		if disk.MustStatus() == ovirtsdk.DISKSTATUS_OK {
			return nil
		} else {
			lastStatus = disk.MustStatus()
		}
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return fmt.Errorf(
				"timeout while waiting for disk to be ok, last status: %s, error: %w)", lastStatus, ctx.Err())
		}
	}
}

func checkJobFinished(ctx context.Context, conn *ovirtsdk.Connection, correlationID string) (bool, error) {
	for {
		jobResp, err := conn.SystemService().JobsService().List().
			Search(fmt.Sprintf("correlation_id=%s", correlationID)).Send()
		if err != nil {
			return false, fmt.Errorf("faild searching jobs, error: %w", err)
		}
		if jobSlice, ok := jobResp.Jobs(); ok {
			if len(jobSlice.Slice()) == 0 {
				// Job wasn't found either an old job which has been cleaned or never started
				klog.Infof("didn't find job with correlation_id %s", correlationID)
				return true, nil
			}
			for _, job := range jobSlice.Slice() {
				if status, _ := job.Status(); status != ovirtsdk.JOBSTATUS_STARTED {
					return true, nil
				}
			}
		}
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return false, fmt.Errorf(
				"timeout while waiting for job with correlation_id %s to finish, error %w", correlationID, ctx.Err())
		}
	}
}

func isFileDomain(storageType ovirtsdk.StorageType) bool {
	switch storageType {
	case ovirtsdk.STORAGETYPE_NFS, ovirtsdk.STORAGETYPE_GLUSTERFS, ovirtsdk.STORAGETYPE_POSIXFS, ovirtsdk.STORAGETYPE_LOCALFS:
		return true
	default:
		return false
	}
}
