package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type artifact struct{ url, name, digest string }

var publicToolchain = []artifact{
	{"https://github.com/relux-works/jcardsim/releases/download/v3.0.5.9-relux.1/jcardsim-3.0.5.9-relux.1.jar", "jcardsim.jar", "fd6e1289d0a337c5ac1dc9624bc46cf8717bd162228d8d801ada9615ba3d122a"},
	{"https://github.com/martinpaljak/ant-javacard/releases/download/v26.02.22/ant-javacard.jar", "ant-javacard.jar", "779909502744af7eb8c24b7b423c4efa6fbf47a1274a11ddf8c49697b1028e64"},
	{"https://codeload.github.com/martinpaljak/oracle_javacard_sdks/tar.gz/700ec80afdda210a0e62fb6a151a9cddc1acd244", "sdk.tar.gz", "367f5dc6a922a689a1447eff9125da919a36f5a8594eeedcd454eb0ada62f37f"},
	{"https://archive.apache.org/dist/ant/binaries/apache-ant-1.10.15-bin.tar.gz", "ant.tar.gz", "71334d7e5d98cfe53d6c429a648a5021137a967378667306c5f613dff5180506"},
}

func fetchArtifact(root string, a artifact) error {
	client := http.Client{Timeout: 240 * time.Second}
	response, e := client.Get(a.url)
	if e != nil {
		return fmt.Errorf("fetch %s: request failed", a.name)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: HTTP %d", a.name, response.StatusCode)
	}
	temp, e := os.CreateTemp(root, ".download-")
	if e != nil {
		return e
	}
	defer os.Remove(temp.Name())
	hash := sha256.New()
	_, e = io.Copy(io.MultiWriter(temp, hash), response.Body)
	closeErr := temp.Close()
	if e != nil {
		return fmt.Errorf("fetch %s: incomplete read", a.name)
	}
	if closeErr != nil {
		return closeErr
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != a.digest {
		return fmt.Errorf("artifact digest disagreement: %s", a.name)
	}
	if e = os.Rename(temp.Name(), filepath.Join(root, a.name)); e != nil {
		return e
	}
	fmt.Printf("%s SHA-256 %s verified\n", a.name, got)
	return nil
}
func toolchain(root string) error {
	if e := os.MkdirAll(root, 0755); e != nil {
		return e
	}
	for _, a := range publicToolchain {
		if e := fetchArtifact(root, a); e != nil {
			return e
		}
	}
	for _, name := range []string{"sdk", "ant"} {
		dir := filepath.Join(root, name)
		if e := os.MkdirAll(dir, 0755); e != nil {
			return e
		}
		if e := command(root, "tar", "-xzf", filepath.Join(root, name+".tar.gz"), "-C", dir, "--strip-components=1"); e != nil {
			return e
		}
	}
	return nil
}
