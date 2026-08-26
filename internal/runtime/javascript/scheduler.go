package javascript

import (
	"fmt"
	"math"
	"time"

	schedulerapi "github.com/Grove-Computing/Growse/internal/webapi/scheduler"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installScheduler(vm *goja.Runtime) error {
	if err := vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		return runtime.scheduleTimer(vm, call, false)
	}); err != nil {
		return err
	}
	if err := vm.Set("setInterval", func(call goja.FunctionCall) goja.Value {
		return runtime.scheduleTimer(vm, call, true)
	}); err != nil {
		return err
	}
	clear := func(call goja.FunctionCall) goja.Value {
		runtime.schedulerAPI.ClearTimer(schedulerapi.TimerID(call.Argument(0).ToInteger()))
		return goja.Undefined()
	}
	if err := vm.Set("clearTimeout", clear); err != nil {
		return err
	}
	if err := vm.Set("clearInterval", clear); err != nil {
		return err
	}
	if err := vm.Set("requestAnimationFrame", func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(vm.NewTypeError("animation frame callback must be a function"))
		}
		id, err := runtime.schedulerAPI.RequestAnimationFrame(func(timestamp schedulerapi.Timestamp) {
			if _, callbackErr := callback(goja.Undefined(), vm.ToValue(float64(time.Duration(timestamp))/float64(time.Millisecond))); callbackErr != nil {
				runtime.recordError(fmt.Sprintf("JavaScript animation frame callback: %v", callbackErr))
			}
		})
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		return vm.ToValue(uint64(id))
	}); err != nil {
		return err
	}
	return vm.Set("cancelAnimationFrame", func(call goja.FunctionCall) goja.Value {
		runtime.schedulerAPI.CancelAnimationFrame(schedulerapi.FrameID(call.Argument(0).ToInteger()))
		return goja.Undefined()
	})
}

func (runtime *Runtime) scheduleTimer(vm *goja.Runtime, call goja.FunctionCall, repeat bool) goja.Value {
	callback, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(vm.NewTypeError("timer callback must be a function"))
	}
	delay, err := timerDelay(call.Argument(1))
	if err != nil {
		panic(vm.NewTypeError(err.Error()))
	}
	arguments := append([]goja.Value(nil), call.Arguments[2:]...)
	invoke := func() {
		if _, callbackErr := callback(goja.Undefined(), arguments...); callbackErr != nil {
			runtime.recordError(fmt.Sprintf("JavaScript timer callback: %v", callbackErr))
		}
	}
	var id schedulerapi.TimerID
	if repeat {
		id, err = runtime.schedulerAPI.SetInterval(delay, invoke)
	} else {
		id, err = runtime.schedulerAPI.SetTimeout(delay, invoke)
	}
	if err != nil {
		panic(vm.NewTypeError(err.Error()))
	}
	return vm.ToValue(uint64(id))
}

func timerDelay(value goja.Value) (time.Duration, error) {
	if value == nil || goja.IsUndefined(value) {
		return 0, nil
	}
	milliseconds := value.ToFloat()
	if math.IsNaN(milliseconds) || milliseconds < 0 {
		milliseconds = 0
	}
	maximum := float64(schedulerapi.MaxTimerDuration / time.Millisecond)
	if math.IsInf(milliseconds, 0) || milliseconds > maximum {
		return 0, fmt.Errorf("timer delay exceeds the safety limit")
	}
	return time.Duration(milliseconds * float64(time.Millisecond)), nil
}
