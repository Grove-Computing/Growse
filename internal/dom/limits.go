package dom

// Browser-visible DOM limits are shared by initial HTML, runtime mutation, and
// isolated worker snapshots.
const (
	MaxNodesPerDocument  = 50_000
	MaxTreeDepth         = 256
	MaxAttributesPerNode = 256
	MaxDOMStringBytes    = 1 << 20
	MaxScriptTextBytes   = 2 << 20
	MaxStyleTextBytes    = 4 << 20
)

// TextLimitForParent preserves the separately specified script and stylesheet
// resource limits while ordinary DOM strings remain capped at 1 MiB.
func TextLimitForParent(parentTag string) int {
	switch parentTag {
	case "script":
		return MaxScriptTextBytes
	case "style":
		return MaxStyleTextBytes
	default:
		return MaxDOMStringBytes
	}
}

func attachmentWithinDepth(parent, child *Node) bool {
	parentDepth := 0
	for current := parent; current != nil && current.Parent != nil; current = current.Parent {
		parentDepth++
	}
	return parentDepth+1+subtreeHeight(child, 0) <= MaxTreeDepth
}

func subtreeHeight(node *Node, depth int) int {
	maximum := depth
	for _, child := range node.Children {
		if height := subtreeHeight(child, depth+1); height > maximum {
			maximum = height
		}
	}
	return maximum
}
