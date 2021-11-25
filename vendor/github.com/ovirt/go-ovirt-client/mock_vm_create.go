package ovirtclient

import (
	"github.com/google/uuid"
)

func (m *mockClient) CreateVM(clusterID string, templateID TemplateID, name string, params OptionalVMParameters, _ ...RetryStrategy) (VM, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if err := validateVMCreationParameters(clusterID, templateID, name, params); err != nil {
		return nil, err
	}
	if _, ok := m.clusters[clusterID]; !ok {
		return nil, newError(ENotFound, "cluster with ID %s not found", clusterID)
	}
	tpl, ok := m.templates[templateID]
	if !ok {
		return nil, newError(ENotFound, "template with ID %s not found", templateID)
	}

	if params == nil {
		params = &vmParams{}
	}
	if name == "" {
		return nil, newError(EBadArgument, "The name parameter is required for VM creation.")
	}

	for _, vm := range m.vms {
		if vm.name == name {
			return nil, newError(EConflict, "A VM with the name \"%s\" already exists.", name)
		}
	}

	cpu := m.createVMCPU(params, tpl)

	id := uuid.Must(uuid.NewUUID()).String()
	vm := &vm{
		client:     m,
		id:         id,
		name:       name,
		comment:    params.Comment(),
		clusterID:  clusterID,
		templateID: templateID,
		status:     VMStatusDown,
		cpu:        cpu,
	}
	m.vms[id] = vm
	m.diskAttachmentsByVM[id] = map[string]*diskAttachment{}
	return vm, nil
}

func (m *mockClient) createVMCPU(params OptionalVMParameters, tpl *template) *vmCPU {
	var cpu *vmCPU
	cpuParams := params.CPU()
	switch {
	case cpuParams != nil:
		cpu = &vmCPU{
			topo: &vmCPUTopo{
				cores:   cpuParams.Cores(),
				sockets: cpuParams.Sockets(),
				threads: cpuParams.Threads(),
			},
		}
	case tpl.cpu != nil:
		cpu = tpl.cpu.clone()
	default:
		cpu = &vmCPU{
			topo: &vmCPUTopo{
				cores:   1,
				sockets: 1,
				threads: 1,
			},
		}
	}
	return cpu
}
