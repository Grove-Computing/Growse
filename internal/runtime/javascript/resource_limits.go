package javascript

import (
	"errors"
	"fmt"
)

func (runtime *Runtime) claimDynamicScript(id uint64) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if _, prepared := runtime.dynamicScripts[id]; prepared {
		return errResourceAlreadyPrepared
	}
	if runtime.scriptCount >= maxPageScripts {
		return fmt.Errorf("JavaScript Page exceeds %d scripts", maxPageScripts)
	}
	runtime.dynamicScripts[id] = struct{}{}
	runtime.scriptCount++
	return nil
}

func (runtime *Runtime) reserveScriptBytes(size int) bool {
	if size < 0 || size > maxModuleBytes {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if size > maxPageScriptBytes-runtime.scriptBytes {
		return false
	}
	runtime.scriptBytes += size
	return true
}

func (runtime *Runtime) beginDynamicInsertion() bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.dynamicInsertDepth >= maxDynamicInsertDepth {
		return false
	}
	runtime.dynamicInsertDepth++
	return true
}

func (runtime *Runtime) endDynamicInsertion() {
	runtime.mu.Lock()
	if runtime.dynamicInsertDepth > 0 {
		runtime.dynamicInsertDepth--
	}
	runtime.mu.Unlock()
}

func (runtime *Runtime) allowResourceAttempt(target string) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.resourceFailures[target] < maxResourceFailureRetries
}

func (runtime *Runtime) recordResourceFailure(target string, resourceErr error) {
	if resourceErr == nil || target == "" {
		return
	}
	runtime.mu.Lock()
	runtime.resourceFailures[target]++
	runtime.mu.Unlock()
}

func (runtime *Runtime) updateResourceSignature(states map[uint64]string, id uint64, signature string, maxResources int) (bool, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if states[id] == signature {
		return false, nil
	}
	if _, exists := states[id]; !exists && len(states) >= maxResources {
		return false, fmt.Errorf("dynamic resource exceeds count limit %d", maxResources)
	}
	if runtime.resourcePrepareCounts[id] >= maxResourceReprepares {
		return false, fmt.Errorf("resource node exceeds %d prepare attempts", maxResourceReprepares)
	}
	runtime.resourcePrepareCounts[id]++
	states[id] = signature
	return true, nil
}

var errResourceAlreadyPrepared = errors.New("resource was already prepared")
