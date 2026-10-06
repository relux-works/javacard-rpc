// jcrpc-parity compares independently built CLI binaries over existing IDLs.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type invocation struct {
	Args   []string
	Exit   int
	Stdout string
	Stderr string
	Files  map[string]string
}
type comparison struct {
	Input       string
	InputSHA256 string
	Baseline    invocation
	Candidate   invocation
}
type report struct {
	BaselineSHA256  string
	CandidateSHA256 string
	Comparisons     []comparison
	Error           string
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	flags := flag.NewFlagSet("jcrpc-parity", flag.ContinueOnError)
	old := flags.String("baseline", "", "independently built baseline CLI")
	new := flags.String("candidate", "", "candidate CLI")
	repo := flags.String("repo", "..", "facade repository root")
	consumer := flags.String("consumer", "", "consumer IDL (required)")
	out := flags.String("out", "", "JSON evidence path (required)")
	plant := flags.Bool("plant-byte-change", false, "change one generated candidate byte as a negative control")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *old == "" || *new == "" || *consumer == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "baseline, candidate, consumer and out are required")
		return 2
	}
	r := report{}
	err := verify(*old, *new, *repo, *consumer, *plant, &r)
	if err != nil {
		r.Error = err.Error()
	}
	b, marshalErr := json.MarshalIndent(r, "", "  ")
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, marshalErr)
		return 1
	}
	if writeErr := os.WriteFile(*out, append(b, '\n'), 0644); writeErr != nil {
		fmt.Fprintln(os.Stderr, writeErr)
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("%d comparisons; empty byte diff\n", len(r.Comparisons))
	return 0
}

func hashFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// inventory fails on unreadable entries and non-regular output; it never treats
// a failed read as an empty tree. Directory metadata is outside byte parity.
func inventory(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular output %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, err := hashFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = digest
		return nil
	})
	return files, err
}

func compareFiles(old, new map[string]string) error {
	keys := make([]string, 0, len(old)+len(new))
	for k := range old {
		keys = append(keys, k)
	}
	for k := range new {
		if _, ok := old[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		a, aOK := old[k]
		b, bOK := new[k]
		if !aOK || !bOK || a != b {
			return fmt.Errorf("output drift: %s (baseline=%s candidate=%s)", k, a, b)
		}
	}
	return nil
}

func invoke(binary, output string, args []string) (invocation, error) {
	if err := os.MkdirAll(output, 0755); err != nil {
		return invocation{}, err
	}
	actual := append(append([]string{}, args[:len(args)-1]...), "--out-dir", output, args[len(args)-1])
	c := exec.Command(binary, actual...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			return invocation{}, err
		}
	}
	files, err := inventory(output)
	if err != nil {
		return invocation{}, err
	}
	normalize := func(s string) string { return strings.ReplaceAll(s, output, "<out-dir>") }
	return invocation{Args: actual, Exit: code, Stdout: normalize(stdout.String()), Stderr: normalize(stderr.String()), Files: files}, nil
}

func verify(old, new, repo, consumer string, plant bool, r *report) error {
	var err error
	old, err = filepath.Abs(old)
	if err != nil {
		return err
	}
	new, err = filepath.Abs(new)
	if err != nil {
		return err
	}
	r.BaselineSHA256, err = hashFile(old)
	if err != nil {
		return err
	}
	r.CandidateSHA256, err = hashFile(new)
	if err != nil {
		return err
	}
	inputs := []string{}
	err = filepath.WalkDir(filepath.Join(repo, "examples"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "generated" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if !d.IsDir() && filepath.Ext(path) == ".toml" {
			inputs = append(inputs, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return fmt.Errorf("no example IDLs found")
	}
	inputs = append(inputs, filepath.Join(repo, "codegen", "testdata", "counter.toml"), filepath.Join(repo, "codegen", "testdata", "stream.toml"), consumer)
	sort.Strings(inputs)
	root, err := os.MkdirTemp("", "jcrpc-parity-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	selectors := [][]string{{"--java", "io.parity.server"}, {"--kotlin", "io.parity.client"}, {"--swift", "ParityClient"}, {"--all"}, {"--java", "io.parity.server", "--kotlin", "io.parity.client"}, {"--all", "--java", "io.parity.server", "--kotlin", "io.parity.client", "--swift", "ParityClient"}}
	for _, input := range inputs {
		input, err = filepath.Abs(input)
		if err != nil {
			return err
		}
		digest, err := hashFile(input)
		if err != nil {
			return err
		}
		cases := [][]string{{"--validate-only"}, {}, {"--help"}, {"-h"}}
		for _, selector := range selectors {
			for _, memory := range []string{"", "clear_on_deselect", "clear_on_reset"} {
				for _, sim := range []string{"", "com.klinec:jcardsim:3.0.5.9", "works.relux:jcardsim:3.0.5.9-relux.1"} {
					args := append([]string{"--verbose"}, selector...)
					if memory != "" {
						args = append(args, "--stream-memory", memory)
					}
					if sim != "" {
						args = append(args, "--simulator-dependency", sim)
					}
					cases = append(cases, args)
				}
			}
		}
		for _, args := range cases {
			args = append(append([]string{}, args...), input)
			idx := len(r.Comparisons)
			oldDir := filepath.Join(root, fmt.Sprintf("%03d-old", idx))
			newDir := filepath.Join(root, fmt.Sprintf("%03d-new", idx))
			a, err := invoke(old, oldDir, args)
			if err != nil {
				return err
			}
			b, err := invoke(new, newDir, args)
			if err != nil {
				return err
			}
			if plant && len(b.Files) > 0 {
				keys := make([]string, 0, len(b.Files))
				for k := range b.Files {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				path := filepath.Join(newDir, keys[0])
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if len(data) == 0 {
					return fmt.Errorf("plant needs nonempty output")
				}
				data[0] ^= 1
				if err := os.WriteFile(path, data, 0644); err != nil {
					return err
				}
				b.Files, err = inventory(newDir)
				if err != nil {
					return err
				}
				plant = false
			}
			r.Comparisons = append(r.Comparisons, comparison{Input: input, InputSHA256: digest, Baseline: a, Candidate: b})
			if a.Exit != b.Exit || a.Stdout != b.Stdout || a.Stderr != b.Stderr {
				return fmt.Errorf("CLI behavior drift: %s %v", input, args)
			}
			if err := compareFiles(a.Files, b.Files); err != nil {
				return fmt.Errorf("%s %v: %w", input, args, err)
			}
		}
	}
	if plant {
		return fmt.Errorf("byte-change plant was not applied")
	}
	return nil
}
