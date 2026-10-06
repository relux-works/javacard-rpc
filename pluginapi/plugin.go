package pluginapi

// Options carries target naming and generation choices, never parsing policy.
type Options struct {
	Namespace           string
	StreamMemory        string
	SimulatorDependency string
}

// File is one in-memory package file, including source and build manifests.
// Name is a canonical slash-separated relative path under the facade-selected
// target root, local on the writing host. No absolute paths, dot segments, NUL,
// native separator aliases or duplicates are permitted. On POSIX, backslashes
// and colons are literal filename characters. Data must be non-nil; an empty
// non-nil slice is an empty file.
type File struct {
	Name string
	Data []byte
}

// Plugin is a compile-time target backend. Generate consumes a validated model
// and renders the whole target package in ordered memory. Implementations must
// not write to the filesystem, including on error. The facade validates every
// returned file before creating or writing any part of that target package.
type Plugin interface {
	Generate(*Schema, Options) ([]File, error)
}
