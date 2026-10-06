package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Production bootstrap's HTTP/digest gate accepts the exact payload and rejects
// one altered payload, an HTTP refusal and an incomplete body without replacing
// prepared output or leaving partial artifacts. No public service is mocked in CI.
func TestToolchainDigestRejection(t *testing.T) {
	approved := []byte("approved artifact")
	sum := sha256.Sum256(approved)
	digest := hex.EncodeToString(sum[:])
	for _, kind := range []string{"valid", "drift", "http-404", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "http-404" {
					w.WriteHeader(404)
				}
				if kind == "incomplete" {
					w.Header().Set("Content-Length", "100")
				}
				body := approved
				if kind == "drift" {
					body = []byte("changed artifact")
				}
				w.Write(body)
			}))
			defer server.Close()
			root := t.TempDir()
			dest := filepath.Join(root, "artifact.jar")
			if e := os.WriteFile(dest, approved, 0644); e != nil {
				t.Fatal(e)
			}
			e := fetchArtifact(root, artifact{server.URL, "artifact.jar", digest})
			if kind == "valid" {
				if e != nil {
					t.Fatalf("approved artifact refused: %v", e)
				}
			} else {
				expected := map[string]string{"drift": "artifact digest disagreement", "http-404": "HTTP 404", "incomplete": "incomplete read"}[kind]
				if e == nil || !strings.Contains(e.Error(), expected) {
					t.Fatalf("%s artifact admitted or wrong refusal: %v", kind, e)
				}
			}
			b, e := os.ReadFile(dest)
			if e != nil || string(b) != string(approved) {
				t.Fatalf("failed download replaced prepared artifact: %v %s", e, b)
			}
			files, e := os.ReadDir(root)
			if e != nil || len(files) != 1 {
				t.Fatalf("partial download persisted: %v %v", e, files)
			}
		})
	}
}
