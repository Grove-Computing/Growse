package javascript

import (
	"fmt"
	"strings"

	"github.com/Grove-Computing/Growse/internal/events"
	domapi "github.com/Grove-Computing/Growse/internal/webapi/dom"
	"github.com/dop251/goja"
)

func (runtime *Runtime) installEventConstructors(vm *goja.Runtime) error {
	_, err := vm.RunString(`
		(function (global) {
			function initEvent(target, type, init) {
				if (type === undefined) throw new TypeError("Event type is required");
				init = init == null ? {} : Object(init);
				target.type = String(type);
				target.bubbles = Boolean(init.bubbles);
				target.cancelable = Boolean(init.cancelable);
				target.composed = Boolean(init.composed);
				target.defaultPrevented = false;
				target.eventPhase = 0;
				target.target = null;
				target.currentTarget = null;
				target.isTrusted = false;
				target.timeStamp = 0;
			}
			function Event(type, init) {
				if (!(this instanceof Event)) throw new TypeError("Event constructor requires new");
				initEvent(this, type, init);
			}
			Event.prototype.preventDefault = function () {};
			Event.prototype.stopPropagation = function () {};
			Event.prototype.stopImmediatePropagation = function () {};
			function CustomEvent(type, init) {
				if (!(this instanceof CustomEvent)) throw new TypeError("CustomEvent constructor requires new");
				initEvent(this, type, init);
				this.detail = init && "detail" in Object(init) ? init.detail : null;
			}
			function MouseEvent(type, init) {
				if (!(this instanceof MouseEvent)) throw new TypeError("MouseEvent constructor requires new");
				initEvent(this, type, init);
				init = init == null ? {} : Object(init);
				this.clientX = Number(init.clientX) || 0;
				this.clientY = Number(init.clientY) || 0;
				this.button = Number(init.button) || 0;
				this.buttons = Number(init.buttons) || 0;
			}
			function KeyboardEvent(type, init) {
				if (!(this instanceof KeyboardEvent)) throw new TypeError("KeyboardEvent constructor requires new");
				initEvent(this, type, init);
				init = init == null ? {} : Object(init);
				this.key = init.key === undefined ? "" : String(init.key);
				this.code = init.code === undefined ? "" : String(init.code);
				this.repeat = Boolean(init.repeat);
			}
			function FocusEvent(type, init) {
				if (!(this instanceof FocusEvent)) throw new TypeError("FocusEvent constructor requires new");
				initEvent(this, type, init);
				init = init == null ? {} : Object(init);
				this.relatedTarget = init.relatedTarget == null ? null : init.relatedTarget;
			}
			function PointerEvent(type, init) {
				if (!(this instanceof PointerEvent)) throw new TypeError("PointerEvent constructor requires new");
				MouseEvent.call(this, type, init);
				init = init == null ? {} : Object(init);
				this.pointerId = Number(init.pointerId) || 0;
				this.width = Number(init.width) || 1;
				this.height = Number(init.height) || 1;
				this.pressure = Number(init.pressure) || 0;
				this.tiltX = Number(init.tiltX) || 0;
				this.tiltY = Number(init.tiltY) || 0;
				this.pointerType = init.pointerType === undefined ? "" : String(init.pointerType);
				this.isPrimary = Boolean(init.isPrimary);
			}
			Object.setPrototypeOf(CustomEvent.prototype, Event.prototype);
			Object.setPrototypeOf(MouseEvent.prototype, Event.prototype);
			Object.setPrototypeOf(KeyboardEvent.prototype, Event.prototype);
			Object.setPrototypeOf(FocusEvent.prototype, Event.prototype);
			Object.setPrototypeOf(PointerEvent.prototype, MouseEvent.prototype);
			Object.defineProperty(CustomEvent.prototype, "constructor", { value: CustomEvent, writable: true, configurable: true });
			Object.defineProperty(MouseEvent.prototype, "constructor", { value: MouseEvent, writable: true, configurable: true });
			Object.defineProperty(KeyboardEvent.prototype, "constructor", { value: KeyboardEvent, writable: true, configurable: true });
			Object.defineProperty(FocusEvent.prototype, "constructor", { value: FocusEvent, writable: true, configurable: true });
			Object.defineProperty(PointerEvent.prototype, "constructor", { value: PointerEvent, writable: true, configurable: true });
			global.Event = Event;
			global.CustomEvent = CustomEvent;
			global.MouseEvent = MouseEvent;
			global.KeyboardEvent = KeyboardEvent;
			global.FocusEvent = FocusEvent;
			global.PointerEvent = PointerEvent;
		})(globalThis);
	`)
	if err != nil {
		return fmt.Errorf("install Event constructors: %w", err)
	}
	return nil
}

func (runtime *Runtime) dispatchJSEvent(vm *goja.Runtime, target *domapi.Element, value goja.Value) bool {
	object, ok := value.(*goja.Object)
	if !ok || target == nil {
		panic(vm.NewTypeError("dispatchEvent requires an Event"))
	}
	eventType := strings.TrimSpace(object.Get("type").String())
	if eventType == "" || len(eventType) > 128 {
		panic(vm.NewTypeError("Event type is invalid"))
	}
	runtime.mu.Lock()
	runtime.nextJSEventID++
	id := runtime.nextJSEventID
	runtime.jsEventObjects[id] = object
	environment := runtime.environment
	runtime.mu.Unlock()
	defer func() {
		runtime.mu.Lock()
		delete(runtime.jsEventObjects, id)
		runtime.mu.Unlock()
	}()
	if environment.Events == nil || environment.Document == nil {
		return true
	}
	event := events.New(events.Type(eventType), target.ID(), jsBoolean(object.Get("bubbles")), jsBoolean(object.Get("cancelable")))
	event.RuntimeID = id
	if value := object.Get("clientX"); value != nil {
		event.X = float32(value.ToFloat())
	}
	if value := object.Get("clientY"); value != nil {
		event.Y = float32(value.ToFloat())
	}
	environment.Events.DispatchTree(environment.Document, event)
	_ = object.Set("eventPhase", 0)
	_ = object.Set("currentTarget", goja.Null())
	return !event.DefaultPrevented()
}
