package ovirtclient

import (
	"fmt"
)

func (m *mockClient) RemoveTemplate(id TemplateID, retries ...RetryStrategy) (err error) {
	retries = defaultRetries(retries, defaultReadTimeouts())
	err = retry(
		fmt.Sprintf("removing template %s", id),
		nil,
		retries,
		func() error {
			m.lock.Lock()
			defer m.lock.Unlock()
			tpl, ok := m.templates[id]
			if !ok {
				return newError(ENotFound, "Template with ID %s was not found", id)
			}

			for _, vm := range m.vms {
				if vm.templateID == id {
					return newError(
						EConflict,
						"Template %s cannot be removed because it is in use by VM %s.",
						id,
						vm.id,
					)
				}
			}

			if tpl.status != TemplateStatusOK {
				return newError(EConflict, "Template %s is in status %s, not %s.", id, tpl.status, TemplateStatusOK)
			}

			delete(m.templates, id)
			return nil
		})
	return err
}
