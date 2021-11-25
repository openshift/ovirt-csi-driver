package ovirtclient

func (m *mockClient) RemoveDisk(diskID string, _ ...RetryStrategy) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if _, ok := m.disks[diskID]; !ok {
		return newError(ENotFound, "disk with ID %s not found", diskID)
	}

	// Check if disk is attached to a running VM
	if diskAttachment, ok := m.diskAttachmentsByDisk[diskID]; ok {
		vm := m.vms[diskAttachment.vmid]
		if vm.status != VMStatusDown {
			return newError(EConflict, "Disk %s is attached to VM %s", diskID, vm.id)
		}
	}

	delete(m.diskAttachmentsByDisk, diskID)
	delete(m.disks, diskID)

	return nil
}
