package isolated

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/Grove-Computing/Growse/internal/runtime/workerproto"
)

const sandboxProfileVersion = 1

func workerSandboxStatus(constraints []string) sandboxStatusResponse {
	status := runtimemodel.SandboxStatus{
		Platform:           runtime.GOOS,
		ProcessBoundary:    os.Getpid() > 0 && os.Getppid() > 0,
		BrokeredHostIO:     true,
		MinimalEnvironment: hasMinimalWorkerEnvironment(os.Environ()),
		ParentLifecycle:    true,
		MemoryLimitBytes:   maxWorkerHeapBytes,
		Constraints:        append([]string(nil), constraints...),
	}
	status.Ready = status.ProcessBoundary && status.BrokeredHostIO && status.MinimalEnvironment && status.ParentLifecycle
	return sandboxStatusResponse{
		SandboxStatus: status, ProfileVersion: sandboxProfileVersion,
		ProtocolVersion: workerproto.Version, WorkerPID: os.Getpid(),
	}
}

func validateSandboxStatus(status sandboxStatusResponse, browserPID int) error {
	if status.ProfileVersion != sandboxProfileVersion {
		return fmt.Errorf("sandbox profile version %d is unsupported", status.ProfileVersion)
	}
	if status.ProtocolVersion != workerproto.Version {
		return fmt.Errorf("sandbox protocol version %d is unsupported", status.ProtocolVersion)
	}
	if status.Platform != runtime.GOOS {
		return fmt.Errorf("sandbox platform %q does not match %q", status.Platform, runtime.GOOS)
	}
	if status.WorkerPID <= 0 || status.WorkerPID == browserPID {
		return errorsForMissingSandbox("separate worker process")
	}
	missing := make([]string, 0, 4)
	if !status.ProcessBoundary {
		missing = append(missing, "process boundary")
	}
	if !status.BrokeredHostIO {
		missing = append(missing, "brokered host I/O")
	}
	if !status.MinimalEnvironment {
		missing = append(missing, "minimal environment")
	}
	if !status.ParentLifecycle {
		missing = append(missing, "parent lifecycle")
	}
	if status.MemoryLimitBytes != maxWorkerHeapBytes {
		missing = append(missing, "worker memory limit")
	}
	if !status.Ready || len(missing) != 0 {
		return errorsForMissingSandbox(missing...)
	}
	return nil
}

func errorsForMissingSandbox(missing ...string) error {
	if len(missing) == 0 {
		missing = []string{"required boundary"}
	}
	return fmt.Errorf("runtime sandbox is unavailable: %s", strings.Join(missing, ", "))
}

func hasMinimalWorkerEnvironment(environment []string) bool {
	allowed := map[string]bool{workerEnvironmentKey: true, "GOMAXPROCS": true}
	if runtime.GOOS == "windows" {
		allowed["SYSTEMROOT"] = true
		allowed["WINDIR"] = true
	}
	names := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, found := strings.Cut(entry, "=")
		name = strings.ToUpper(strings.TrimSpace(name))
		if !found || name == "" || !allowed[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return len(names) == 0
}
