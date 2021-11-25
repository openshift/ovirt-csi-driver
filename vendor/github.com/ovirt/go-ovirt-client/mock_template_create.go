package ovirtclient

import (
	"time"
)

func (m *mockClient) CreateTemplate(
	vmID string,
	name string,
	params OptionalTemplateCreateParameters,
	_ ...RetryStrategy,
) (Template, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	vm, ok := m.vms[vmID]
	if !ok {
		return nil, newError(ENotFound, "VM with ID %s not found", vmID)
	}

	for _, tpl := range m.templates {
		if tpl.name == name {
			return nil, newError(EConflict, "A template with the name \"%s\" already exists.", name)
		}
	}

	if params == nil {
		params = &templateCreateParameters{}
	}

	description := ""
	if desc := params.Description(); desc != nil {
		description = *desc
	}
	tpl := &template{
		client:      m,
		id:          TemplateID(m.GenerateUUID()),
		name:        name,
		description: description,
		status:      TemplateStatusLocked,
		cpu:         vm.cpu.clone(),
	}
	m.templates[tpl.ID()] = tpl
	go func() {
		time.Sleep(2 * time.Second)
		m.lock.Lock()
		defer m.lock.Unlock()
		tpl.status = TemplateStatusOK
	}()
	return tpl, nil
}
