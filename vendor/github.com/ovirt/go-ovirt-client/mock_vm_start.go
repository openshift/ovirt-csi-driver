package ovirtclient

import (
	"time"
)

func (m *mockClient) StartVM(id string, _ ...RetryStrategy) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if item, ok := m.vms[id]; ok {
		if item.Status() != VMStatusUp {
			item.status = VMStatusWaitForLaunch
			go func() {
				time.Sleep(2 * time.Second)
				m.lock.Lock()
				item.status = VMStatusPoweringUp
				m.lock.Unlock()
				time.Sleep(2 * time.Second)
				m.lock.Lock()
				defer m.lock.Unlock()
				item.status = VMStatusUp
			}()
		}
		return nil
	}
	return newError(ENotFound, "vm with ID %s not found", id)
}
