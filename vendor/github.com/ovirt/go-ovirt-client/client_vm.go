package ovirtclient

import (
	"regexp"
	"sync"

	ovirtsdk "github.com/ovirt/go-ovirt"
)

//go:generate go run scripts/rest.go -i "Vm" -n "vm" -o "VM"

// VMClient includes the methods required to deal with virtual machines.
type VMClient interface {
	// CreateVM creates a virtual machine.
	CreateVM(
		clusterID string,
		templateID TemplateID,
		name string,
		optional OptionalVMParameters,
		retries ...RetryStrategy,
	) (VM, error)
	// GetVM returns a single virtual machine based on an ID.
	GetVM(id string, retries ...RetryStrategy) (VM, error)
	// UpdateVM updates the virtual machine with the given parameters.
	// Use UpdateVMParams to obtain a builder for the params.
	UpdateVM(id string, params UpdateVMParameters, retries ...RetryStrategy) (VM, error)
	// StartVM triggers a VM start. The actual VM startup will take time and should be waited for via the
	// WaitForVMStatus call.
	StartVM(id string, retries ...RetryStrategy) error
	// StopVM triggers a VM power-off. The actual VM stop will take time and should be waited for via the
	// WaitForVMStatus call. The force parameter will cause the shutdown to proceed even if a backup is currently
	// running.
	StopVM(id string, force bool, retries ...RetryStrategy) error
	// ShutdownVM triggers a VM shutdown. The actual VM shutdown will take time and should be waited for via the
	// WaitForVMStatus call. The force parameter will cause the shutdown to proceed even if a backup is currently
	// running.
	ShutdownVM(id string, force bool, retries ...RetryStrategy) error
	// WaitForVMStatus waits for the VM to reach the desired status.
	WaitForVMStatus(id string, status VMStatus, retries ...RetryStrategy) (VM, error)
	// ListVMs returns a list of all virtual machines.
	ListVMs(retries ...RetryStrategy) ([]VM, error)
	// SearchVMs lists all virtual machines matching a certain criteria specified in params.
	SearchVMs(params VMSearchParameters, retries ...RetryStrategy) ([]VM, error)
	// RemoveVM removes a virtual machine specified by id.
	RemoveVM(id string, retries ...RetryStrategy) error
}

// VMData is the core of VM providing only data access functions.
type VMData interface {
	// ID returns the unique identifier (UUID) of the current virtual machine.
	ID() string
	// Name is the user-defined name of the virtual machine.
	Name() string
	// Comment is the comment added to the VM.
	Comment() string
	// ClusterID returns the cluster this machine belongs to.
	ClusterID() string
	// TemplateID returns the ID of the base template for this machine.
	TemplateID() TemplateID
	// Status returns the current status of the VM.
	Status() VMStatus
	// CPU returns the CPU structure of a VM.
	CPU() VMCPU
}

// VMCPU is the CPU configuration of a VM.
type VMCPU interface {
	// Topo is the desired CPU topology for this VM.
	Topo() VMCPUTopo
}

type vmCPU struct {
	topo *vmCPUTopo
}

func (v vmCPU) Topo() VMCPUTopo {
	return v.topo
}

func (v *vmCPU) clone() *vmCPU {
	if v == nil {
		return nil
	}
	return &vmCPU{
		topo: v.topo.clone(),
	}
}

// VM is the implementation of the virtual machine in oVirt.
type VM interface {
	VMData

	// Update updates the virtual machine with the given parameters. Use UpdateVMParams to
	// get a builder for the parameters.
	Update(params UpdateVMParameters, retries ...RetryStrategy) (VM, error)
	// Remove removes the current VM. This involves an API call and may be slow.
	Remove(retries ...RetryStrategy) error

	// Start will cause a VM to start. The actual start process takes some time and should be checked via WaitForStatus.
	Start(retries ...RetryStrategy) error
	// Stop will cause the VM to power-off. The force parameter will cause the VM to stop even if a backup is currently
	// running.
	Stop(force bool, retries ...RetryStrategy) error
	// Shutdown will cause the VM to shut down. The force parameter will cause the VM to shut down even if a backup
	// is currently running.
	Shutdown(force bool, retries ...RetryStrategy) error
	// WaitForStatus will wait until the VM reaches the desired status. If the status is not reached within the
	// specified amount of retries, an error will be returned. If the VM enters the desired state, an updated VM
	// object will be returned.
	WaitForStatus(status VMStatus, retries ...RetryStrategy) (VM, error)

	// CreateNIC creates a network interface on the current VM. This involves an API call and may be slow.
	CreateNIC(name string, vnicProfileID string, params OptionalNICParameters, retries ...RetryStrategy) (NIC, error)
	// GetNIC fetches a NIC with a specific ID on the current VM. This involves an API call and may be slow.
	GetNIC(id string, retries ...RetryStrategy) (NIC, error)
	// ListNICs fetches a list of network interfaces attached to this VM. This involves an API call and may be slow.
	ListNICs(retries ...RetryStrategy) ([]NIC, error)

	// AttachDisk attaches a disk to this VM.
	AttachDisk(
		diskID string,
		diskInterface DiskInterface,
		params CreateDiskAttachmentOptionalParams,
		retries ...RetryStrategy,
	) (DiskAttachment, error)
	// GetDiskAttachment returns a specific disk attachment for the current VM by ID.
	GetDiskAttachment(diskAttachmentID string, retries ...RetryStrategy) (DiskAttachment, error)
	// ListDiskAttachments lists all disk attachments for the current VM.
	ListDiskAttachments(retries ...RetryStrategy) ([]DiskAttachment, error)
	// DetachDisk removes a specific disk attachment by the disk attachment ID.
	DetachDisk(
		diskAttachmentID string,
		retries ...RetryStrategy,
	) error
}

// VMSearchParameters declares the parameters that can be passed to a VM search. Each parameter
// is declared as a pointer, where a nil value will mean that parameter will not be searched for.
// All parameters are used together as an AND filter.
type VMSearchParameters interface {
	// Name will match the name of the virtual machine exactly.
	Name() *string
	// Statuses will return a list of acceptable statuses for this VM search.
	Statuses() *VMStatusList
	// NotStatuses will return a list of not acceptable statuses for this VM search.
	NotStatuses() *VMStatusList
}

// BuildableVMSearchParameters is a buildable version of VMSearchParameters.
type BuildableVMSearchParameters interface {
	VMSearchParameters

	// WithName sets the name to search for.
	WithName(name string) BuildableVMSearchParameters
	// WithStatus adds a single status to the filter.
	WithStatus(status VMStatus) BuildableVMSearchParameters
	// WithNotStatus excludes a VM status from the search.
	WithNotStatus(status VMStatus) BuildableVMSearchParameters
	// WithStatuses will return the statuses the returned VMs should be in.
	WithStatuses(list VMStatusList) BuildableVMSearchParameters
	// WithNotStatuses will return the statuses the returned VMs should not be in.
	WithNotStatuses(list VMStatusList) BuildableVMSearchParameters
}

// VMSearchParams creates a buildable set of search parameters for easier use.
func VMSearchParams() BuildableVMSearchParameters {
	return &vmSearchParams{
		lock: &sync.Mutex{},
	}
}

type vmSearchParams struct {
	lock *sync.Mutex

	name        *string
	statuses    *VMStatusList
	notStatuses *VMStatusList
}

func (v *vmSearchParams) WithStatus(status VMStatus) BuildableVMSearchParameters {
	v.lock.Lock()
	defer v.lock.Unlock()
	newStatuses := append(*v.statuses, status)
	v.statuses = &newStatuses
	return v
}

func (v *vmSearchParams) WithNotStatus(status VMStatus) BuildableVMSearchParameters {
	v.lock.Lock()
	defer v.lock.Unlock()
	newNotStatuses := append(*v.notStatuses, status)
	v.statuses = &newNotStatuses
	return v
}

func (v *vmSearchParams) Name() *string {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.name
}

func (v *vmSearchParams) Statuses() *VMStatusList {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.statuses
}

func (v *vmSearchParams) NotStatuses() *VMStatusList {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.notStatuses
}

func (v *vmSearchParams) WithName(name string) BuildableVMSearchParameters {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.name = &name
	return v
}

func (v *vmSearchParams) WithStatuses(list VMStatusList) BuildableVMSearchParameters {
	v.lock.Lock()
	defer v.lock.Unlock()
	newStatuses := list.Copy()
	v.statuses = &newStatuses
	return v
}

func (v *vmSearchParams) WithNotStatuses(list VMStatusList) BuildableVMSearchParameters {
	v.lock.Lock()
	defer v.lock.Unlock()
	newNotStatuses := list.Copy()
	v.notStatuses = &newNotStatuses
	return v
}

// OptionalVMParameters are a list of parameters that can be, but must not necessarily be added on VM creation. This
// interface is expected to be extended in the future.
type OptionalVMParameters interface {
	// Comment returns the comment for the VM.
	Comment() string

	// CPU contains the CPU topology, if any.
	CPU() VMCPUTopo
}

// BuildableVMParameters is a variant of OptionalVMParameters that can be changed using the supplied
// builder functions. This is placed here for future use.
type BuildableVMParameters interface {
	OptionalVMParameters

	// WithComment adds a common to the VM.
	WithComment(comment string) (BuildableVMParameters, error)
	// MustWithComment is identical to WithComment, but panics instead of returning an error.
	MustWithComment(comment string) BuildableVMParameters

	// WithCPU adds a VMCPUTopo to the VM.
	WithCPU(cpu VMCPUTopo) (BuildableVMParameters, error)
	// MustWithCPU adds a VMCPUTopo and panics if an error happens.
	MustWithCPU(cpu VMCPUTopo) BuildableVMParameters
	// WithCPUParameters is a simplified function that calls NewVMCPUTopo and adds the CPU topology to
	// the VM.
	WithCPUParameters(cores, threads, sockets uint) (BuildableVMParameters, error)
	// MustWithCPUParameters is a simplified function that calls MustNewVMCPUTopo and adds the CPU topology to
	// the VM.
	MustWithCPUParameters(cores, threads, sockets uint) BuildableVMParameters
}

// UpdateVMParameters returns a set of parameters to change on a VM.
type UpdateVMParameters interface {
	// Name returns the name for the VM. Return nil if the name should not be changed.
	Name() *string
	// Comment returns the comment for the VM. Return nil if the name should not be changed.
	Comment() *string
}

// VMCPUTopo contains the CPU topology information about a VM.
type VMCPUTopo interface {
	// Cores is the number of CPU cores.
	Cores() uint
	// Threads is the number of CPU threads in a core.
	Threads() uint
	// Sockets is the number of sockets.
	Sockets() uint
}

// NewVMCPUTopo creates a new VMCPUTopo from the specified parameters.
func NewVMCPUTopo(cores uint, threads uint, sockets uint) (VMCPUTopo, error) {
	if cores == 0 {
		return nil, newError(EBadArgument, "number of cores must be positive")
	}
	if threads == 0 {
		return nil, newError(EBadArgument, "number of threads must be positive")
	}
	if sockets == 0 {
		return nil, newError(EBadArgument, "number of sockets must be positive")
	}
	return &vmCPUTopo{
		cores:   cores,
		threads: threads,
		sockets: sockets,
	}, nil
}

// MustNewVMCPUTopo is equivalent to NewVMCPUTopo, but panics instead of returning an error.
func MustNewVMCPUTopo(cores uint, threads uint, sockets uint) VMCPUTopo {
	topo, err := NewVMCPUTopo(cores, threads, sockets)
	if err != nil {
		panic(err)
	}
	return topo
}

type vmCPUTopo struct {
	cores   uint
	threads uint
	sockets uint
}

func (v *vmCPUTopo) Cores() uint {
	return v.cores
}

func (v *vmCPUTopo) Threads() uint {
	return v.threads
}

func (v *vmCPUTopo) Sockets() uint {
	return v.sockets
}

func (v *vmCPUTopo) clone() *vmCPUTopo {
	if v == nil {
		return nil
	}
	return &vmCPUTopo{
		cores:   v.cores,
		threads: v.threads,
		sockets: v.sockets,
	}
}

// BuildableUpdateVMParameters is a buildable version of UpdateVMParameters.
type BuildableUpdateVMParameters interface {
	UpdateVMParameters

	// WithName adds an updated name to the request.
	WithName(name string) (BuildableUpdateVMParameters, error)

	// MustWithName is identical to WithName, but panics instead of returning an error
	MustWithName(name string) BuildableUpdateVMParameters

	// WithComment adds a comment to the request
	WithComment(comment string) (BuildableUpdateVMParameters, error)

	// MustWithComment is identical to WithComment, but panics instead of returning an error.
	MustWithComment(comment string) BuildableUpdateVMParameters
}

// UpdateVMParams returns a buildable set of update parameters.
func UpdateVMParams() BuildableUpdateVMParameters {
	return &updateVMParams{}
}

type updateVMParams struct {
	name    *string
	comment *string
}

func (u *updateVMParams) MustWithName(name string) BuildableUpdateVMParameters {
	builder, err := u.WithName(name)
	if err != nil {
		panic(err)
	}
	return builder
}

func (u *updateVMParams) MustWithComment(comment string) BuildableUpdateVMParameters {
	builder, err := u.WithComment(comment)
	if err != nil {
		panic(err)
	}
	return builder
}

func (u *updateVMParams) Name() *string {
	return u.name
}

func (u *updateVMParams) Comment() *string {
	return u.comment
}

func (u *updateVMParams) WithName(name string) (BuildableUpdateVMParameters, error) {
	if err := validateVMName(name); err != nil {
		return nil, err
	}
	u.name = &name
	return u, nil
}

func (u *updateVMParams) WithComment(comment string) (BuildableUpdateVMParameters, error) {
	u.comment = &comment
	return u, nil
}

// CreateVMParams creates a set of BuildableVMParameters that can be used to construct the optional VM parameters.
func CreateVMParams() BuildableVMParameters {
	return &vmParams{
		lock: &sync.Mutex{},
	}
}

type vmParams struct {
	lock *sync.Mutex

	name    string
	comment string
	cpu     VMCPUTopo
}

func (v *vmParams) CPU() VMCPUTopo {
	return v.cpu
}

func (v *vmParams) WithCPU(cpu VMCPUTopo) (BuildableVMParameters, error) {
	v.cpu = cpu
	return v, nil
}

func (v *vmParams) MustWithCPU(cpu VMCPUTopo) BuildableVMParameters {
	builder, err := v.WithCPU(cpu)
	if err != nil {
		panic(err)
	}
	return builder
}

func (v *vmParams) WithCPUParameters(cores, threads, sockets uint) (BuildableVMParameters, error) {
	cpu, err := NewVMCPUTopo(cores, threads, sockets)
	if err != nil {
		return nil, err
	}
	return v.WithCPU(cpu)
}

func (v *vmParams) MustWithCPUParameters(cores, threads, sockets uint) BuildableVMParameters {
	return v.MustWithCPU(MustNewVMCPUTopo(cores, threads, sockets))
}

func (v *vmParams) MustWithName(name string) BuildableVMParameters {
	builder, err := v.WithName(name)
	if err != nil {
		panic(err)
	}
	return builder
}

func (v *vmParams) MustWithComment(comment string) BuildableVMParameters {
	builder, err := v.WithComment(comment)
	if err != nil {
		panic(err)
	}
	return builder
}

func (v *vmParams) WithName(name string) (BuildableVMParameters, error) {
	if err := validateVMName(name); err != nil {
		return nil, err
	}
	v.name = name
	return v, nil
}

func (v *vmParams) WithComment(comment string) (BuildableVMParameters, error) {
	v.comment = comment
	return v, nil
}

func (v vmParams) Name() string {
	return v.name
}

func (v vmParams) Comment() string {
	return v.comment
}

type vm struct {
	client Client

	id         string
	name       string
	comment    string
	clusterID  string
	templateID TemplateID
	status     VMStatus
	cpu        *vmCPU
}

func (v *vm) Start(retries ...RetryStrategy) error {
	return v.client.StartVM(v.id, retries...)
}

func (v *vm) Stop(force bool, retries ...RetryStrategy) error {
	return v.client.StopVM(v.id, force, retries...)
}

func (v *vm) Shutdown(force bool, retries ...RetryStrategy) error {
	return v.client.ShutdownVM(v.id, force, retries...)
}

func (v *vm) WaitForStatus(status VMStatus, retries ...RetryStrategy) (VM, error) {
	return v.client.WaitForVMStatus(v.id, status, retries...)
}

func (v *vm) CPU() VMCPU {
	return v.cpu
}

// withName returns a copy of the VM with the new name. It does not change the original copy to avoid
// shared state issues.
func (v *vm) withName(name string) *vm {
	return &vm{
		client:     v.client,
		id:         v.id,
		name:       name,
		comment:    v.comment,
		clusterID:  v.clusterID,
		templateID: v.templateID,
		status:     v.status,
		cpu:        v.cpu,
	}
}

// withComment returns a copy of the VM with the new comment. It does not change the original copy to avoid
// shared state issues.
func (v *vm) withComment(comment string) *vm {
	return &vm{
		client:     v.client,
		id:         v.id,
		name:       v.name,
		comment:    comment,
		clusterID:  v.clusterID,
		templateID: v.templateID,
		status:     v.status,
		cpu:        v.cpu,
	}
}

func (v *vm) Update(params UpdateVMParameters, retries ...RetryStrategy) (VM, error) {
	return v.client.UpdateVM(v.id, params, retries...)
}

func (v *vm) Status() VMStatus {
	return v.status
}

func (v *vm) AttachDisk(
	diskID string,
	diskInterface DiskInterface,
	params CreateDiskAttachmentOptionalParams,
	retries ...RetryStrategy,
) (DiskAttachment, error) {
	return v.client.CreateDiskAttachment(v.id, diskID, diskInterface, params, retries...)
}

func (v *vm) GetDiskAttachment(diskAttachmentID string, retries ...RetryStrategy) (DiskAttachment, error) {
	return v.client.GetDiskAttachment(v.id, diskAttachmentID, retries...)
}

func (v *vm) ListDiskAttachments(retries ...RetryStrategy) ([]DiskAttachment, error) {
	return v.client.ListDiskAttachments(v.id, retries...)
}

func (v *vm) DetachDisk(diskAttachmentID string, retries ...RetryStrategy) error {
	return v.client.RemoveDiskAttachment(v.id, diskAttachmentID, retries...)
}

func (v *vm) Remove(retries ...RetryStrategy) error {
	return v.client.RemoveVM(v.id, retries...)
}

func (v *vm) CreateNIC(name string, vnicProfileID string, params OptionalNICParameters, retries ...RetryStrategy) (NIC, error) {
	return v.client.CreateNIC(v.id, vnicProfileID, name, params, retries...)
}

func (v *vm) GetNIC(id string, retries ...RetryStrategy) (NIC, error) {
	return v.client.GetNIC(v.id, id, retries...)
}

func (v *vm) ListNICs(retries ...RetryStrategy) ([]NIC, error) {
	return v.client.ListNICs(v.id, retries...)
}

func (v *vm) Comment() string {
	return v.comment
}

func (v *vm) ClusterID() string {
	return v.clusterID
}

func (v *vm) TemplateID() TemplateID {
	return v.templateID
}

func (v *vm) ID() string {
	return v.id
}

func (v *vm) Name() string {
	return v.name
}

var vmNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_\-.]*$`)

func validateVMName(name string) error {
	if !vmNameRegexp.MatchString(name) {
		return newError(EBadArgument, "invalid VM name: %s", name)
	}
	return nil
}

func convertSDKVM(sdkObject *ovirtsdk.Vm, client Client) (VM, error) {
	id, ok := sdkObject.Id()
	if !ok {
		return nil, newError(EFieldMissing, "id field missing from VM object")
	}
	name, ok := sdkObject.Name()
	if !ok {
		return nil, newError(EFieldMissing, "name field missing from VM object")
	}
	comment, ok := sdkObject.Comment()
	if !ok {
		return nil, newError(EFieldMissing, "comment field missing from VM object")
	}
	cluster, ok := sdkObject.Cluster()
	if !ok {
		return nil, newError(EFieldMissing, "cluster field missing from VM object")
	}
	status, ok := sdkObject.Status()
	if !ok {
		return nil, newFieldNotFound("vm", "status")
	}
	clusterID, ok := cluster.Id()
	if !ok {
		return nil, newError(EFieldMissing, "ID field missing from cluster in VM object")
	}
	template, ok := sdkObject.Template()
	if !ok {
		return nil, newFieldNotFound("VM", "template")
	}
	templateID, ok := template.Id()
	if !ok {
		return nil, newFieldNotFound("template in VM", "template ID")
	}
	cpu, err := convertSDKVMCPU(sdkObject)
	if err != nil {
		return nil, err
	}

	return &vm{
		id:         id,
		name:       name,
		comment:    comment,
		clusterID:  clusterID,
		client:     client,
		templateID: TemplateID(templateID),
		status:     VMStatus(status),
		cpu:        cpu,
	}, nil
}

func convertSDKVMCPU(sdkObject *ovirtsdk.Vm) (*vmCPU, error) {
	sdkCPU, ok := sdkObject.Cpu()
	if !ok {
		return nil, newFieldNotFound("VM", "CPU")
	}
	cpuTopo, ok := sdkCPU.Topology()
	if !ok {
		return nil, newFieldNotFound("CPU in VM", "CPU topo")
	}
	cores, ok := cpuTopo.Cores()
	if !ok {
		return nil, newFieldNotFound("CPU topo in CPU in VM", "cores")
	}
	threads, ok := cpuTopo.Threads()
	if !ok {
		return nil, newFieldNotFound("CPU topo in CPU in VM", "threads")
	}
	sockets, ok := cpuTopo.Sockets()
	if !ok {
		return nil, newFieldNotFound("CPU topo in CPU in VM", "sockets")
	}
	cpu := &vmCPU{
		topo: &vmCPUTopo{
			uint(cores),
			uint(threads),
			uint(sockets),
		},
	}
	return cpu, nil
}

// VMStatus represents the status of a VM.
type VMStatus string

const (
	// VMStatusDown indicates that the VM is not running.
	VMStatusDown VMStatus = "down"
	// VMStatusImageLocked indicates that the virtual machine process is not running and there is some operation on the
	// disks of the virtual machine that prevents it from being started.
	VMStatusImageLocked VMStatus = "image_locked"
	// VMStatusMigrating indicates that the virtual machine process is running and the virtual machine is being migrated
	// from one host to another.
	VMStatusMigrating VMStatus = "migrating"
	// VMStatusNotResponding indicates that the hypervisor detected that the virtual machine is not responding.
	VMStatusNotResponding VMStatus = "not_responding"
	// VMStatusPaused indicates that the virtual machine process is running and the virtual machine is paused.
	// This may happen in two cases: when running a virtual machine is paused mode and when the virtual machine is being
	// automatically paused due to an error.
	VMStatusPaused VMStatus = "paused"
	// VMStatusPoweringDown indicates that the virtual machine process is running and it is about to stop running.
	VMStatusPoweringDown VMStatus = "powering_down"
	// VMStatusPoweringUp  indicates that the virtual machine process is running and the guest operating system is being
	// loaded. Note that if no guest-agent is installed, this status is set for a predefined period of time, that is by
	// default 60 seconds, when running a virtual machine.
	VMStatusPoweringUp VMStatus = "powering_up"
	// VMStatusRebooting indicates that the virtual machine process is running and the guest operating system is being
	// rebooted.
	VMStatusRebooting VMStatus = "reboot_in_progress"
	// VMStatusRestoringState indicates that the virtual machine process is about to run and the virtual machine is
	// going to awake from hibernation. In this status, the running state of the virtual machine is being restored.
	VMStatusRestoringState VMStatus = "restoring_state"
	// VMStatusSavingState indicates that the virtual machine process is running and the virtual machine is being
	// hibernated. In this status, the running state of the virtual machine is being saved. Note that this status does
	// not mean that the guest operating system is being hibernated.
	VMStatusSavingState VMStatus = "saving_state"
	// VMStatusSuspended indicates that the virtual machine process is not running and a running state of the virtual
	// machine was saved. This status is similar to Down, but when the VM is started in this status its saved running
	// state is restored instead of being booted using the normal procedure.
	VMStatusSuspended VMStatus = "suspended"
	// VMStatusUnassigned means an invalid status was received.
	VMStatusUnassigned VMStatus = "unassigned"
	// VMStatusUnknown indicates that the system failed to determine the status of the virtual machine.
	// The virtual machine process may be running or not running in this status.
	// For instance, when host becomes non-responsive the virtual machines that ran on it are set with this status.
	VMStatusUnknown VMStatus = "unknown"
	// VMStatusUp indicates that the virtual machine process is running and the guest operating system is loaded.
	// Note that if no guest-agent is installed, this status is set after a predefined period of time, that is by
	// default 60 seconds, when running a virtual machine.
	VMStatusUp VMStatus = "up"
	// VMStatusWaitForLaunch indicates that the virtual machine process is about to run.
	// This status is set when a request to run a virtual machine arrives to the host.
	// It is possible that the virtual machine process will fail to run.
	VMStatusWaitForLaunch VMStatus = "wait_for_launch"
)

// Validate validates if a VMStatus has a valid value.
func (s VMStatus) Validate() error {
	for _, v := range VMStatusValues() {
		if v == s {
			return nil
		}
	}
	return newError(EBadArgument, "invalid value for VM status: %s", s)
}

// VMStatusList is a list of VMStatus.
type VMStatusList []VMStatus

// Copy creates a separate copy of the current status list.
func (l VMStatusList) Copy() VMStatusList {
	result := make([]VMStatus, len(l))
	for i, s := range l {
		result[i] = s
	}
	return result
}

// Validate validates the list of statuses.
func (l VMStatusList) Validate() error {
	for _, s := range l {
		if err := s.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// VMStatusValues returns all possible VMStatus values.
func VMStatusValues() VMStatusList {
	return []VMStatus{
		VMStatusDown,
		VMStatusImageLocked,
		VMStatusMigrating,
		VMStatusNotResponding,
		VMStatusPaused,
		VMStatusPoweringDown,
		VMStatusPoweringUp,
		VMStatusRebooting,
		VMStatusRestoringState,
		VMStatusSavingState,
		VMStatusSuspended,
		VMStatusUnassigned,
		VMStatusUnknown,
		VMStatusUp,
		VMStatusWaitForLaunch,
	}
}

// Strings creates a string list of the values.
func (l VMStatusList) Strings() []string {
	result := make([]string, len(l))
	for i, status := range l {
		result[i] = string(status)
	}
	return result
}
