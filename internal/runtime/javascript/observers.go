package javascript

import (
	"fmt"
	"math"
	"sort"
	"strings"

	dommodel "github.com/Grove-Computing/Growse/internal/dom"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	"github.com/dop251/goja"
)

const (
	maxObserversPerPage       = 2_048
	maxPendingObserverRecords = 16_384
	maxObserverCallbacks      = 1_024
	maxResizeObserverLoops    = 32
)

type mutationObserveOptions struct {
	childList, attributes, characterData bool
	subtree                              bool
	attributeOldValue, characterOldValue bool
	attributeFilter                      map[string]bool
}

type mutationRecord struct {
	typeName, attributeName string
	target                  dommodel.NodeID
	added, removed          []dommodel.NodeID
	oldValue                *string
}

type mutationObserverRecord struct {
	object   *goja.Object
	callback goja.Callable
	targets  map[dommodel.NodeID]mutationObserveOptions
	records  []mutationRecord
}

type resizeObserverRecord struct {
	object   *goja.Object
	callback goja.Callable
	targets  map[dommodel.NodeID]*runtimemodel.RenderSnapshot
}

type intersectionSample struct {
	ratio float32
	rect  runtimemodel.DOMRect
}

type intersectionObserverRecord struct {
	object     *goja.Object
	callback   goja.Callable
	targets    map[dommodel.NodeID]*intersectionSample
	thresholds []float32
}

func (runtime *Runtime) installObservers(vm *goja.Runtime) error {
	if err := vm.Set("MutationObserver", func(call goja.ConstructorCall) *goja.Object {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(vm.NewTypeError("MutationObserver callback must be a function"))
		}
		runtime.reserveObserver(vm)
		record := &mutationObserverRecord{object: call.This, callback: callback, targets: make(map[dommodel.NodeID]mutationObserveOptions)}
		_ = call.This.Set("observe", func(arguments goja.FunctionCall) goja.Value {
			target := runtime.domElementForThis(vm, arguments.Argument(0))
			options, err := parseMutationOptions(vm, arguments.Argument(1))
			if err != nil {
				panic(vm.NewTypeError(err.Error()))
			}
			record.targets[target.ID()] = options
			return goja.Undefined()
		})
		_ = call.This.Set("disconnect", func(goja.FunctionCall) goja.Value {
			runtime.pendingMutationRecords -= len(record.records)
			record.records = nil
			record.targets = make(map[dommodel.NodeID]mutationObserveOptions)
			return goja.Undefined()
		})
		_ = call.This.Set("takeRecords", func(goja.FunctionCall) goja.Value {
			return runtime.takeMutationRecords(vm, record)
		})
		runtime.mutationObservers = append(runtime.mutationObservers, record)
		return call.This
	}); err != nil {
		return err
	}
	if err := vm.Set("ResizeObserver", func(call goja.ConstructorCall) *goja.Object {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(vm.NewTypeError("ResizeObserver callback must be a function"))
		}
		runtime.reserveObserver(vm)
		record := &resizeObserverRecord{object: call.This, callback: callback, targets: make(map[dommodel.NodeID]*runtimemodel.RenderSnapshot)}
		runtime.installFrameObserverMethods(vm, call.This, func(id dommodel.NodeID) { record.targets[id] = nil }, func(id dommodel.NodeID) { delete(record.targets, id) }, func() { record.targets = make(map[dommodel.NodeID]*runtimemodel.RenderSnapshot) })
		runtime.resizeObservers = append(runtime.resizeObservers, record)
		return call.This
	}); err != nil {
		return err
	}
	return vm.Set("IntersectionObserver", func(call goja.ConstructorCall) *goja.Object {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(vm.NewTypeError("IntersectionObserver callback must be a function"))
		}
		thresholds, err := intersectionThresholds(vm, call.Argument(1))
		if err != nil {
			panic(vm.NewTypeError(err.Error()))
		}
		runtime.reserveObserver(vm)
		record := &intersectionObserverRecord{object: call.This, callback: callback, targets: make(map[dommodel.NodeID]*intersectionSample), thresholds: thresholds}
		runtime.installFrameObserverMethods(vm, call.This, func(id dommodel.NodeID) { record.targets[id] = nil }, func(id dommodel.NodeID) { delete(record.targets, id) }, func() { record.targets = make(map[dommodel.NodeID]*intersectionSample) })
		_ = call.This.DefineDataProperty("root", goja.Null(), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		_ = call.This.DefineDataProperty("rootMargin", vm.ToValue("0px 0px 0px 0px"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		thresholdValues := make([]interface{}, len(thresholds))
		for index, threshold := range thresholds {
			thresholdValues[index] = threshold
		}
		_ = call.This.DefineDataProperty("thresholds", vm.NewArray(thresholdValues...), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE)
		runtime.intersectionObservers = append(runtime.intersectionObservers, record)
		return call.This
	})
}

func (runtime *Runtime) reserveObserver(vm *goja.Runtime) {
	if runtime.observerCount >= runtime.maxObservers {
		panic(vm.NewTypeError("observer limit exceeded"))
	}
	runtime.observerCount++
}

func parseMutationOptions(vm *goja.Runtime, value goja.Value) (mutationObserveOptions, error) {
	object, ok := value.(*goja.Object)
	if !ok {
		return mutationObserveOptions{}, fmt.Errorf("MutationObserver options are required")
	}
	options := mutationObserveOptions{
		childList: jsBoolean(object.Get("childList")), attributes: jsBoolean(object.Get("attributes")),
		characterData: jsBoolean(object.Get("characterData")), subtree: jsBoolean(object.Get("subtree")),
		attributeOldValue: jsBoolean(object.Get("attributeOldValue")), characterOldValue: jsBoolean(object.Get("characterDataOldValue")),
	}
	filter := object.Get("attributeFilter")
	if filter != nil && !goja.IsUndefined(filter) {
		var names []string
		if err := vm.ExportTo(filter, &names); err != nil {
			return mutationObserveOptions{}, fmt.Errorf("attributeFilter must be a string sequence")
		}
		options.attributeFilter = make(map[string]bool, len(names))
		for _, name := range names {
			options.attributeFilter[strings.ToLower(name)] = true
		}
		options.attributes = true
	}
	if options.attributeOldValue {
		options.attributes = true
	}
	if options.characterOldValue {
		options.characterData = true
	}
	if !options.childList && !options.attributes && !options.characterData {
		return mutationObserveOptions{}, fmt.Errorf("MutationObserver requires childList, attributes, or characterData")
	}
	return options, nil
}

func (runtime *Runtime) installFrameObserverMethods(vm *goja.Runtime, object *goja.Object, observe, unobserve func(dommodel.NodeID), disconnect func()) {
	_ = object.Set("observe", func(call goja.FunctionCall) goja.Value {
		target := runtime.domElementForThis(vm, call.Argument(0))
		observe(target.ID())
		runtime.requestObserverFrame()
		return goja.Undefined()
	})
	_ = object.Set("unobserve", func(call goja.FunctionCall) goja.Value {
		target := runtime.domElementForThis(vm, call.Argument(0))
		unobserve(target.ID())
		return goja.Undefined()
	})
	_ = object.Set("disconnect", func(goja.FunctionCall) goja.Value { disconnect(); return goja.Undefined() })
}

func intersectionThresholds(vm *goja.Runtime, value goja.Value) ([]float32, error) {
	thresholds := []float32{0}
	if object, ok := value.(*goja.Object); ok {
		if root := object.Get("root"); root != nil && !goja.IsNull(root) && !goja.IsUndefined(root) {
			return nil, fmt.Errorf("IntersectionObserver root elements are unsupported")
		}
		if margin := object.Get("rootMargin"); margin != nil && !goja.IsUndefined(margin) {
			value := strings.TrimSpace(margin.String())
			if value != "" && value != "0" && value != "0px" && value != "0px 0px 0px 0px" {
				return nil, fmt.Errorf("only a zero rootMargin is supported")
			}
		}
		if threshold := object.Get("threshold"); threshold != nil && !goja.IsUndefined(threshold) {
			var values []float64
			if _, array := threshold.(*goja.Object); array {
				if err := vm.ExportTo(threshold, &values); err != nil {
					return nil, fmt.Errorf("invalid intersection threshold")
				}
			} else {
				values = []float64{threshold.ToFloat()}
			}
			thresholds = thresholds[:0]
			for _, current := range values {
				if math.IsNaN(current) || current < 0 || current > 1 {
					return nil, fmt.Errorf("intersection threshold must be between 0 and 1")
				}
				thresholds = append(thresholds, float32(current))
			}
			if len(thresholds) == 0 {
				thresholds = append(thresholds, 0)
			}
			sort.Slice(thresholds, func(left, right int) bool { return thresholds[left] < thresholds[right] })
		}
	}
	return thresholds, nil
}

func (runtime *Runtime) handleDOMMutation() {
	if runtime.environment.Document != nil {
		current := runtime.environment.Document.Snapshot()
		runtime.queueMutationDiff(runtime.mutationSnapshot, current)
		runtime.mutationSnapshot = current
	}
	if len(runtime.resizeObservers) != 0 || len(runtime.intersectionObservers) != 0 {
		runtime.requestObserverFrame()
	}
	if runtime.environment.OnMutation != nil {
		runtime.environment.OnMutation()
	}
}

type mutationNodeState struct {
	node     dommodel.NodeSnapshot
	parent   dommodel.NodeID
	children []dommodel.NodeID
}

func flattenMutationSnapshot(snapshot dommodel.DocumentSnapshot) map[dommodel.NodeID]mutationNodeState {
	result := make(map[dommodel.NodeID]mutationNodeState)
	var walk func(dommodel.NodeSnapshot, dommodel.NodeID)
	walk = func(node dommodel.NodeSnapshot, parent dommodel.NodeID) {
		children := make([]dommodel.NodeID, len(node.Children))
		for index, child := range node.Children {
			children[index] = child.ID
		}
		result[node.ID] = mutationNodeState{node: node, parent: parent, children: children}
		for _, child := range node.Children {
			walk(child, node.ID)
		}
	}
	if snapshot.Root.ID != 0 {
		walk(snapshot.Root, 0)
	}
	return result
}

func (runtime *Runtime) queueMutationDiff(previous, current dommodel.DocumentSnapshot) {
	before, after := flattenMutationSnapshot(previous), flattenMutationSnapshot(current)
	for id, oldState := range before {
		newState, exists := after[id]
		if !exists {
			continue
		}
		if !equalNodeIDs(oldState.children, newState.children) {
			added, removed := childDifference(oldState.children, newState.children)
			runtime.queueMutation(mutationRecord{typeName: "childList", target: id, added: added, removed: removed}, before, after)
		}
		if oldState.node.Type == dommodel.NodeText && oldState.node.Text != newState.node.Text {
			old := oldState.node.Text
			runtime.queueMutation(mutationRecord{typeName: "characterData", target: id, oldValue: &old}, before, after)
		}
		attributeNames := make(map[string]bool, len(oldState.node.Attributes)+len(newState.node.Attributes))
		for name := range oldState.node.Attributes {
			attributeNames[name] = true
		}
		for name := range newState.node.Attributes {
			attributeNames[name] = true
		}
		for name := range attributeNames {
			oldValue, oldOK := oldState.node.Attributes[name]
			newValue, newOK := newState.node.Attributes[name]
			if oldOK == newOK && oldValue == newValue {
				continue
			}
			var old *string
			if oldOK {
				copyValue := oldValue
				old = &copyValue
			}
			runtime.queueMutation(mutationRecord{typeName: "attributes", target: id, attributeName: name, oldValue: old}, before, after)
		}
	}
}

func equalNodeIDs(left, right []dommodel.NodeID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func childDifference(before, after []dommodel.NodeID) (added, removed []dommodel.NodeID) {
	beforeSet, afterSet := make(map[dommodel.NodeID]bool, len(before)), make(map[dommodel.NodeID]bool, len(after))
	for _, id := range before {
		beforeSet[id] = true
	}
	for _, id := range after {
		afterSet[id] = true
		if !beforeSet[id] {
			added = append(added, id)
		}
	}
	for _, id := range before {
		if !afterSet[id] {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func (runtime *Runtime) queueMutation(record mutationRecord, before, after map[dommodel.NodeID]mutationNodeState) {
	for _, observer := range runtime.mutationObservers {
		options, matched := mutationOptionsForTarget(observer.targets, record.target, before, after)
		if !matched || record.typeName == "childList" && !options.childList || record.typeName == "attributes" && !options.attributes || record.typeName == "characterData" && !options.characterData {
			continue
		}
		if record.typeName == "attributes" && len(options.attributeFilter) != 0 && !options.attributeFilter[record.attributeName] {
			continue
		}
		copyRecord := record
		if record.typeName == "attributes" && !options.attributeOldValue || record.typeName == "characterData" && !options.characterOldValue {
			copyRecord.oldValue = nil
		}
		if runtime.pendingMutationRecords >= runtime.maxObserverRecords {
			runtime.clearMutationRecords()
			runtime.recordError("JavaScript observer record limit exceeded")
			return
		}
		observer.records = append(observer.records, copyRecord)
		runtime.pendingMutationRecords++
	}
}

func mutationOptionsForTarget(targets map[dommodel.NodeID]mutationObserveOptions, target dommodel.NodeID, before, after map[dommodel.NodeID]mutationNodeState) (mutationObserveOptions, bool) {
	if options, ok := targets[target]; ok {
		return options, true
	}
	current := target
	for current != 0 {
		state, ok := after[current]
		if !ok {
			state, ok = before[current]
		}
		if !ok {
			break
		}
		current = state.parent
		if options, exists := targets[current]; exists && options.subtree {
			return options, true
		}
	}
	return mutationObserveOptions{}, false
}

func (runtime *Runtime) clearMutationRecords() {
	for _, observer := range runtime.mutationObservers {
		observer.records = nil
	}
	runtime.pendingMutationRecords = 0
}

func (runtime *Runtime) takeMutationRecords(vm *goja.Runtime, observer *mutationObserverRecord) *goja.Object {
	records := observer.records
	observer.records = nil
	runtime.pendingMutationRecords -= len(records)
	values := make([]interface{}, len(records))
	for index, record := range records {
		values[index] = runtime.mutationRecordValue(vm, record)
	}
	return vm.NewArray(values...)
}

func (runtime *Runtime) mutationRecordValue(vm *goja.Runtime, record mutationRecord) *goja.Object {
	object := vm.NewObject()
	_ = object.Set("type", record.typeName)
	_ = object.Set("target", runtime.nodeValue(vm, runtime.domAPI.NodeByID(record.target)))
	_ = object.Set("attributeName", nullableString(vm, record.attributeName))
	_ = object.Set("attributeNamespace", goja.Null())
	if record.oldValue == nil {
		_ = object.Set("oldValue", goja.Null())
	} else {
		_ = object.Set("oldValue", *record.oldValue)
	}
	_ = object.Set("addedNodes", runtime.nodeIDsValue(vm, record.added))
	_ = object.Set("removedNodes", runtime.nodeIDsValue(vm, record.removed))
	_ = object.Set("previousSibling", goja.Null())
	_ = object.Set("nextSibling", goja.Null())
	return object
}

func nullableString(vm *goja.Runtime, value string) goja.Value {
	if value == "" {
		return goja.Null()
	}
	return vm.ToValue(value)
}

func (runtime *Runtime) nodeIDsValue(vm *goja.Runtime, ids []dommodel.NodeID) *goja.Object {
	values := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		values = append(values, runtime.nodeValue(vm, runtime.domAPI.NodeByID(id)))
	}
	return vm.NewArray(values...)
}

func (runtime *Runtime) deliverMutationObservers(vm *goja.Runtime) {
	callbacks := 0
	for callbacks < runtime.maxObserverCallbacks {
		delivered := false
		for _, observer := range runtime.mutationObservers {
			if len(observer.records) == 0 {
				continue
			}
			records := runtime.takeMutationRecords(vm, observer)
			if _, err := observer.callback(goja.Undefined(), records, observer.object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript MutationObserver callback: %v", err))
			}
			callbacks++
			delivered = true
			if callbacks >= runtime.maxObserverCallbacks {
				break
			}
		}
		if !delivered {
			return
		}
	}
	if runtime.pendingMutationRecords != 0 {
		runtime.clearMutationRecords()
		runtime.recordError("JavaScript MutationObserver checkpoint callback limit exceeded")
	}
}

func (runtime *Runtime) requestObserverFrame() {
	runtime.frameObserversDirty.Store(true)
	if runtime.environment.RequestFrame != nil {
		runtime.environment.RequestFrame()
	}
}

func (runtime *Runtime) deliverFrameObservers(vm *goja.Runtime) bool {
	deliveredAny, callbacks := false, 0
	for loop := 0; loop < runtime.maxObserverLoops && callbacks < runtime.maxObserverCallbacks; loop++ {
		runtime.frameObserversDirty.Store(false)
		delivered := false
		for _, observer := range runtime.resizeObservers {
			entries := runtime.resizeEntries(vm, observer)
			if len(entries) == 0 {
				continue
			}
			if _, err := observer.callback(goja.Undefined(), vm.NewArray(entries...), observer.object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript ResizeObserver callback: %v", err))
			}
			callbacks++
			delivered, deliveredAny = true, true
		}
		for _, observer := range runtime.intersectionObservers {
			entries := runtime.intersectionEntries(vm, observer)
			if len(entries) == 0 {
				continue
			}
			if _, err := observer.callback(goja.Undefined(), vm.NewArray(entries...), observer.object); err != nil {
				runtime.recordError(fmt.Sprintf("JavaScript IntersectionObserver callback: %v", err))
			}
			callbacks++
			delivered, deliveredAny = true, true
		}
		if !delivered || !runtime.frameObserversDirty.Load() {
			return deliveredAny
		}
	}
	runtime.frameObserversDirty.Store(false)
	runtime.recordError("JavaScript observer frame loop limit exceeded")
	return deliveredAny
}

func (runtime *Runtime) resizeEntries(vm *goja.Runtime, observer *resizeObserverRecord) []interface{} {
	if runtime.environment.ReadRender == nil {
		return nil
	}
	var entries []interface{}
	for id, previous := range observer.targets {
		element := runtime.domAPI.NodeByID(id)
		if element == nil || !element.IsConnected() {
			continue
		}
		snapshot, err := runtime.environment.ReadRender(runtime.runtimeCtx, id)
		if err != nil {
			continue
		}
		if previous != nil && previous.Rect.Width == snapshot.Rect.Width && previous.Rect.Height == snapshot.Rect.Height {
			continue
		}
		copySnapshot := snapshot
		observer.targets[id] = &copySnapshot
		entry := vm.NewObject()
		_ = entry.Set("target", runtime.nodeValue(vm, element))
		_ = entry.Set("contentRect", domRectValue(vm, snapshot.Rect))
		size := vm.NewObject()
		_ = size.Set("inlineSize", snapshot.ClientWidth)
		_ = size.Set("blockSize", snapshot.ClientHeight)
		_ = entry.Set("contentBoxSize", vm.NewArray(size))
		_ = entry.Set("borderBoxSize", vm.NewArray(size))
		entries = append(entries, entry)
	}
	return entries
}

func (runtime *Runtime) intersectionEntries(vm *goja.Runtime, observer *intersectionObserverRecord) []interface{} {
	if runtime.environment.ReadRender == nil {
		return nil
	}
	var entries []interface{}
	viewport := runtimemodel.DOMRect{Width: runtime.media.ViewportWidth, Height: runtime.media.ViewportHeight}
	for id, previous := range observer.targets {
		element := runtime.domAPI.NodeByID(id)
		if element == nil || !element.IsConnected() {
			continue
		}
		snapshot, err := runtime.environment.ReadRender(runtime.runtimeCtx, id)
		if err != nil {
			continue
		}
		intersection := intersectDOMRect(snapshot.Rect, viewport)
		area := snapshot.Rect.Width * snapshot.Rect.Height
		ratio := float32(0)
		if area > 0 {
			ratio = intersection.Width * intersection.Height / area
		}
		if previous != nil && !crossesThreshold(previous.ratio, ratio, observer.thresholds) {
			continue
		}
		observer.targets[id] = &intersectionSample{ratio: ratio, rect: intersection}
		entry := vm.NewObject()
		_ = entry.Set("target", runtime.nodeValue(vm, element))
		_ = entry.Set("boundingClientRect", domRectValue(vm, snapshot.Rect))
		_ = entry.Set("intersectionRect", domRectValue(vm, intersection))
		_ = entry.Set("rootBounds", domRectValue(vm, viewport))
		_ = entry.Set("intersectionRatio", ratio)
		_ = entry.Set("isIntersecting", intersection.Width > 0 && intersection.Height > 0)
		_ = entry.Set("time", 0)
		entries = append(entries, entry)
	}
	return entries
}

func intersectDOMRect(left, right runtimemodel.DOMRect) runtimemodel.DOMRect {
	x := max32(left.X, right.X)
	y := max32(left.Y, right.Y)
	r := min32(left.X+left.Width, right.X+right.Width)
	b := min32(left.Y+left.Height, right.Y+right.Height)
	if r <= x || b <= y {
		return runtimemodel.DOMRect{X: x, Y: y}
	}
	return runtimemodel.DOMRect{X: x, Y: y, Width: r - x, Height: b - y}
}

func crossesThreshold(previous, current float32, thresholds []float32) bool {
	if previous == current {
		return false
	}
	for _, threshold := range thresholds {
		if previous < threshold && current >= threshold || previous >= threshold && current < threshold || threshold == 0 && (previous == 0) != (current == 0) {
			return true
		}
	}
	return false
}

func max32(left, right float32) float32 {
	if left > right {
		return left
	}
	return right
}
func min32(left, right float32) float32 {
	if left < right {
		return left
	}
	return right
}
