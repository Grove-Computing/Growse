package devtools

// RuntimeScript is secret-free metadata for one selected page script.
type RuntimeScript struct {
	Kind     string
	Schedule string
	Location string
}

// RuntimeSandbox is the bounded capability state reported by one worker.
type RuntimeSandbox struct {
	Ready           bool
	ProcessBoundary bool
	BrokeredHostIO  bool
	Generation      uint64
	ConstraintCount int
	Failure         bool
}

// RuntimeContext is a secret-free diagnostic row for one execution context.
// URLs are credential- and query-redacted before this value is constructed.
type RuntimeContext struct {
	Kind               string
	ID                 uint64
	ParentID           uint64
	BrowsingGeneration uint64
	URL                string
	Engine             string
	State              string
	Scripts            []RuntimeScript
	ErrorCategories    []string
	Sandbox            RuntimeSandbox
}
