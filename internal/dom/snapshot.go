package dom

import (
	"errors"
	"fmt"
)

const maxSnapshotNodes = 100_000

// DocumentSnapshot is the pointer-free representation exchanged with an
// isolated runtime worker. Node IDs remain stable across snapshots.
type DocumentSnapshot struct {
	Root       NodeSnapshot `json:"root"`
	NextID     NodeID       `json:"nextId"`
	ReadyState string       `json:"readyState,omitempty"`
}

// NodeSnapshot contains only browser-visible DOM state. Parent and document
// pointers are reconstructed by ApplySnapshot.
type NodeSnapshot struct {
	ID                  NodeID            `json:"id"`
	Type                NodeType          `json:"type"`
	TagName             string            `json:"tagName,omitempty"`
	Text                string            `json:"text,omitempty"`
	Attributes          map[string]string `json:"attributes,omitempty"`
	ControlValue        string            `json:"controlValue,omitempty"`
	ControlValueDirty   bool              `json:"controlValueDirty,omitempty"`
	ControlChecked      bool              `json:"controlChecked,omitempty"`
	ControlCheckedDirty bool              `json:"controlCheckedDirty,omitempty"`
	Children            []NodeSnapshot    `json:"children,omitempty"`
}

// Snapshot returns a detached, serializable copy of the current document.
func (d *Document) Snapshot() DocumentSnapshot {
	if d == nil || d.Root == nil {
		return DocumentSnapshot{}
	}
	return DocumentSnapshot{Root: snapshotNode(d.Root), NextID: d.nextID, ReadyState: d.ReadyState()}
}

// NewDocumentFromSnapshot validates snapshot and constructs an independent
// document with the same stable Node IDs.
func NewDocumentFromSnapshot(snapshot DocumentSnapshot) (*Document, error) {
	document := &Document{byID: make(map[string]*Node), nodes: make(map[NodeID]*Node)}
	if err := document.ApplySnapshot(snapshot); err != nil {
		return nil, err
	}
	return document, nil
}

// ApplySnapshot updates a live document in place. Existing nodes with matching
// IDs and types are reused so runtime-side Element handles remain valid.
func (d *Document) ApplySnapshot(snapshot DocumentSnapshot) error {
	if d == nil {
		return errors.New("apply DOM snapshot to nil document")
	}
	if snapshot.Root.ID == 0 || snapshot.Root.Type != NodeDocument {
		return errors.New("DOM snapshot root must be a non-zero document node")
	}
	readyState := snapshot.ReadyState
	if readyState == "" {
		readyState = "loading"
	}
	if readyState != "loading" && readyState != "interactive" && readyState != "complete" {
		return fmt.Errorf("DOM snapshot has invalid ready state %q", readyState)
	}
	flat := make(map[NodeID]NodeSnapshot)
	if err := flattenSnapshot(snapshot.Root, flat, 0); err != nil {
		return err
	}

	previous := d.nodes
	if previous == nil {
		previous = make(map[NodeID]*Node)
	}
	nodes := make(map[NodeID]*Node, len(flat))
	var maximum NodeID
	for id, state := range flat {
		node := previous[id]
		if node == nil || node.Type != state.Type {
			node = &Node{ID: id, Type: state.Type}
		}
		node.ID = id
		node.Type = state.Type
		node.TagName = state.TagName
		node.Text = state.Text
		node.Attributes = cloneAttributes(state.Attributes)
		node.ControlValue = state.ControlValue
		node.ControlValueDirty = state.ControlValueDirty
		node.ControlChecked = state.ControlChecked
		node.ControlCheckedDirty = state.ControlCheckedDirty
		node.Parent = nil
		node.Children = nil
		node.document = d
		nodes[id] = node
		if id > maximum {
			maximum = id
		}
	}
	for id, old := range previous {
		if _, retained := nodes[id]; !retained {
			old.Parent = nil
			old.Children = nil
		}
	}
	if err := linkSnapshot(snapshot.Root, nodes, nil); err != nil {
		return err
	}
	d.Root = nodes[snapshot.Root.ID]
	d.nodes = nodes
	d.nextID = snapshot.NextID
	d.readyState = readyState
	if d.nextID <= maximum {
		d.nextID = maximum + 1
	}
	d.rebuildIDIndex()
	return nil
}

func snapshotNode(node *Node) NodeSnapshot {
	state := NodeSnapshot{
		ID: node.ID, Type: node.Type, TagName: node.TagName, Text: node.Text,
		Attributes: cloneAttributes(node.Attributes), ControlValue: node.ControlValue,
		ControlValueDirty: node.ControlValueDirty, ControlChecked: node.ControlChecked,
		ControlCheckedDirty: node.ControlCheckedDirty,
	}
	if len(node.Children) != 0 {
		state.Children = make([]NodeSnapshot, len(node.Children))
		for index, child := range node.Children {
			state.Children[index] = snapshotNode(child)
		}
	}
	return state
}

func flattenSnapshot(node NodeSnapshot, flat map[NodeID]NodeSnapshot, depth int) error {
	if depth > maxSnapshotNodes {
		return errors.New("DOM snapshot depth exceeds safety limit")
	}
	if node.ID == 0 {
		return errors.New("DOM snapshot contains a zero node ID")
	}
	if node.Type > NodeDocumentFragment {
		return fmt.Errorf("DOM snapshot node %d has invalid type %d", node.ID, node.Type)
	}
	if _, duplicate := flat[node.ID]; duplicate {
		return fmt.Errorf("DOM snapshot contains duplicate node ID %d", node.ID)
	}
	if len(flat) >= maxSnapshotNodes {
		return errors.New("DOM snapshot node count exceeds safety limit")
	}
	flat[node.ID] = node
	for _, child := range node.Children {
		if err := flattenSnapshot(child, flat, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func linkSnapshot(state NodeSnapshot, nodes map[NodeID]*Node, parent *Node) error {
	node := nodes[state.ID]
	if node == nil {
		return fmt.Errorf("DOM snapshot node %d is unavailable", state.ID)
	}
	node.Parent = parent
	if len(state.Children) != 0 {
		node.Children = make([]*Node, len(state.Children))
		for index, childState := range state.Children {
			child := nodes[childState.ID]
			if child == nil {
				return fmt.Errorf("DOM snapshot child %d is unavailable", childState.ID)
			}
			node.Children[index] = child
			if err := linkSnapshot(childState, nodes, node); err != nil {
				return err
			}
		}
	}
	return nil
}
