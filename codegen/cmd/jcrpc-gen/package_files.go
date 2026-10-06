package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

// Validate the entire returned target package before any directory or file is
// created. This is a path contract, not a concurrent hostile-filesystem sandbox.
func validatePackageFiles(root string, files []pluginapi.File) error {
	if len(files) == 0 {
		return fmt.Errorf("empty package")
	}
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if !fs.ValidPath(file.Name) || file.Name == "." || strings.ContainsRune(file.Name, '\x00') ||
			!filepath.IsLocal(filepath.FromSlash(file.Name)) || filepath.ToSlash(file.Name) != file.Name {
			return fmt.Errorf("invalid path %q", file.Name)
		}
		if seen[file.Name] {
			return fmt.Errorf("duplicate path %q", file.Name)
		}
		if file.Data == nil {
			return fmt.Errorf("nil data for %q", file.Name)
		}
		seen[file.Name] = true
	}
	for _, file := range files {
		for parent := filepath.Dir(file.Name); parent != "."; parent = filepath.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf("file/directory conflict %q", parent)
			}
		}
		// Existing symlinks inside the selected root (including root itself) cannot
		// redirect a returned path outside it. Ordinary I/O failures remain writer
		// errors, not fabricated generation refusals.
		path := root
		for _, component := range append([]string{""}, strings.Split(file.Name, "/")...) {
			if component != "" {
				path = filepath.Join(path, component)
			}
			info, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					break
				}
				if errors.Is(err, syscall.ENOTDIR) {
					break // Writer retains its existing not-a-directory diagnostic.
				}
				return err // An unreadable path is not an absent path.
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink output path %q", path)
			}
		}
	}
	return nil
}
