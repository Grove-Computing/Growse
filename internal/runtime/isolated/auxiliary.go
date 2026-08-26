package isolated

import "os/exec"

// ConfigureAuxiliaryWorkerCommand applies the same parent-side process
// containment used by page Runtime workers.
func ConfigureAuxiliaryWorkerCommand(command *exec.Cmd) error {
	return configureWorkerCommand(command)
}

// ApplyAuxiliaryWorkerSandbox applies and verifies the child-side platform
// restrictions used by page Runtime workers.
func ApplyAuxiliaryWorkerSandbox() ([]string, error) {
	return applyWorkerPlatformSandbox()
}

// AcquireAuxiliaryWorkerSlot reserves one process from the Browser Session
// worker budget shared with page and frame runtimes.
func AcquireAuxiliaryWorkerSlot() bool {
	if activeWorkers.Add(1) > maxSessionWorkers {
		activeWorkers.Add(-1)
		return false
	}
	return true
}

// ReleaseAuxiliaryWorkerSlot returns a previously acquired process slot.
func ReleaseAuxiliaryWorkerSlot() { activeWorkers.Add(-1) }
