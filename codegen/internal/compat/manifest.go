// Package compat validates released backend/runtime tuples before consumer work.
package compat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

type Target struct {
	Target            string `json:"target"`
	Repository        string `json:"repository"`
	Tag               string `json:"tag"`
	TagObject         string `json:"tag_object"`
	Commit            string `json:"commit"`
	GoModule          string `json:"go_module"`
	BackendVersion    string `json:"backend_version"`
	BackendImport     string `json:"backend_import"`
	Runtime           string `json:"runtime"`
	SwiftPackage      string `json:"swift_package,omitempty"`
	SwiftExactVersion string `json:"swift_exact_version,omitempty"`
}
type Input struct {
	Path   string `json:"path"`
	Commit string `json:"commit"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Schema    int      `json:"schema"`
	Targets   []Target `json:"targets"`
	BSimInput Input    `json:"bsim_input"`
}

func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing JSON in %s", path)
	}
	return nil
}

// Check binds the public manifest to reviewed release receipts and the actual
// resolved module graph. No absent/read-failed evidence is treated as satisfied.
func Check(repo, manifest string) (Manifest, error) {
	var m Manifest
	if e := readJSON(manifest, &m); e != nil {
		return m, e
	}
	if m.Schema != 1 || len(m.Targets) != 3 {
		return m, fmt.Errorf("manifest must contain exactly three targets, schema 1")
	}
	seen := map[string]bool{}
	for _, p := range m.Targets {
		if seen[p.Target] {
			return m, fmt.Errorf("duplicate target %s", p.Target)
		}
		seen[p.Target] = true
		switch p.Target {
		case "javacard", "kotlin", "swift":
		default:
			return m, fmt.Errorf("unknown target %s", p.Target)
		}
		var receipt Target
		if e := readJSON(filepath.Join(repo, "compatibility", "releases", p.Target+".json"), &receipt); e != nil {
			return m, e
		}
		if !reflect.DeepEqual(p, receipt) {
			return m, fmt.Errorf("release receipt disagreement: %s", p.Target)
		}
		if p.Tag != p.BackendVersion {
			return m, fmt.Errorf("backend/runtime version disagreement: %s", p.Target)
		}
	}
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Dir = filepath.Join(repo, "codegen")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
	b, e := cmd.Output()
	if e != nil {
		return m, fmt.Errorf("resolved module graph: %w", e)
	}
	type module struct {
		Path, Version string
		Replace       *module
	}
	modules := map[string]module{}
	d := json.NewDecoder(bytes.NewReader(b))
	for {
		var p module
		e = d.Decode(&p)
		if e == io.EOF {
			break
		}
		if e != nil {
			return m, e
		}
		modules[p.Path] = p
	}
	for _, p := range m.Targets {
		q, ok := modules[p.GoModule]
		if !ok || q.Version != p.BackendVersion || q.Replace != nil {
			return m, fmt.Errorf("backend module disagreement: %s", p.Target)
		}
	}
	q := modules["github.com/relux-works/javacard-rpc/pluginapi"]
	if q.Version != "v0.1.0" || q.Replace != nil {
		return m, fmt.Errorf("pluginapi must resolve released v0.1.0 without replace")
	}
	want := Input{"compatibility/inputs/bsim-auth-2d23abd.toml", "2d23abdafa1e0f68c6003ab56274b2ac38378ef9", "1be1ed52ac9a85a62d5c5e371a9e22d38f834282681528476ca071e7bfc2cb66"}
	if m.BSimInput != want {
		return m, fmt.Errorf("bsim input identity disagreement")
	}
	b, e = os.ReadFile(filepath.Join(repo, m.BSimInput.Path))
	if e != nil {
		return m, e
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != m.BSimInput.SHA256 {
		return m, fmt.Errorf("bsim input digest disagreement")
	}
	return m, nil
}

func VerifyCheckout(repo, dir string, p Target) error {
	git := func(args ...string) (string, error) {
		c := exec.Command("git", args...)
		c.Dir = dir
		b, e := c.CombinedOutput()
		if e != nil {
			return "", fmt.Errorf("git %v: %w: %s", args, e, b)
		}
		return strings.TrimSpace(string(b)), nil
	}
	for ref, want := range map[string]string{"HEAD": p.Commit, "refs/tags/" + p.Tag: p.TagObject, "refs/tags/" + p.Tag + "^{}": p.Commit} {
		got, e := git("rev-parse", ref)
		if e != nil {
			return e
		}
		if got != want {
			return fmt.Errorf("checkout identity disagreement %s: %s", p.Target, ref)
		}
	}
	clean, e := git("status", "--porcelain", "--untracked-files=all")
	if e != nil {
		return e
	}
	if clean != "" {
		return fmt.Errorf("dirty pinned runtime %s", p.Target)
	}
	// A fresh release-tree index has neither caller index hints nor cached stat
	// data. Compare tracked working files without changing the consumer's index.
	indexDir, e := os.MkdirTemp("", "jcrpc-release-index-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(indexDir)
	tracked := func(args ...string) error {
		c := exec.Command("git", append([]string{"-c", "core.fsmonitor=false", "-c", "core.sparseCheckout=false"}, args...)...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_INDEX_FILE="+filepath.Join(indexDir, "index"))
		b, err := c.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %v: %w: %s", args, err, b)
		}
		return nil
	}
	if e = tracked("read-tree", p.Commit); e != nil {
		return e
	}
	if e = tracked("update-index", "--refresh"); e != nil {
		return fmt.Errorf("dirty pinned runtime %s: tracked release comparison: %w", p.Target, e)
	}
	signature, e := git("-c", "gpg.ssh.allowedSignersFile="+filepath.Join(repo, "compatibility", "allowed_signers"), "verify-tag", p.Tag)
	if e == nil {
		fmt.Printf("%s: %s\n", p.Target, signature)
	}
	return e
}
