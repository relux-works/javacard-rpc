package pluginapi

// Options carries target naming and generation choices, never parsing policy.
type Options struct {
	Namespace    string
	StreamMemory string
}

// File is a rendered source file. The facade owns layout and filesystem writes.
type File struct {
	Name string
	Data []byte
}

// Plugin is a compile-time target backend. Generate consumes a validated model
// and renders in memory; an error must not write output to the filesystem.
type Plugin interface {
	Generate(*Schema, Options) ([]File, error)
}
