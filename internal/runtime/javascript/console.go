package javascript

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

func (runtime *Runtime) installConsole(vm *goja.Runtime) error {
	console := vm.NewObject()
	for _, level := range []string{"log", "info", "warn", "error"} {
		current := level
		if err := console.Set(current, func(call goja.FunctionCall) goja.Value {
			parts := make([]string, len(call.Arguments))
			for index, argument := range call.Arguments {
				parts[index] = safeConsoleValue(argument)
			}
			runtime.mu.Lock()
			record := runtime.environment.ConsoleRecord
			fallback := runtime.environment.ConsoleLog
			runtime.mu.Unlock()
			message := strings.Join(parts, " ")
			if record != nil {
				record(current, message)
			} else if fallback != nil {
				fallback(message)
			}
			return goja.Undefined()
		}); err != nil {
			return err
		}
	}
	return vm.Set("console", console)
}

func safeConsoleValue(value goja.Value) string {
	switch {
	case value == nil || goja.IsUndefined(value):
		return "undefined"
	case goja.IsNull(value):
		return "null"
	}
	if object, ok := value.(*goja.Object); ok {
		class := object.ClassName()
		if class == "" {
			class = "Object"
		}
		return "[object " + class + "]"
	}
	switch exported := value.Export().(type) {
	case string:
		return exported
	case bool:
		return strconv.FormatBool(exported)
	case int64:
		return strconv.FormatInt(exported, 10)
	case float64:
		return strconv.FormatFloat(exported, 'g', -1, 64)
	default:
		return fmt.Sprint(exported)
	}
}
