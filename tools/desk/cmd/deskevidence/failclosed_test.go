package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// failclosed_test.go — source-level guards that survive the forge migration.
//
// The pre-migration guards in this file pinned the SHAPE of resolveInstallID (no fabricated
// installation id) and the httpClient timeout, and exercised the verifier-App PEM search path.
// All three concerns left this package with the JWT/installation exchange: the mint now lives in
// the identity layer (`desktoken verifier`), reached through mintVerifierToken, and the custody
// binding is the resolver's (internal/deskkit/forgeresolve.go and its tests). The one guard that
// still belongs here is the no-parallel rule, because this package's tests still mutate shared
// package-level state through setupFake.

// TestNoParallelTestsInPackage: this package's tests mutate package-level state (the forgeForFn
// / mintTokenFn / publicRepoGateFn seams, ghToken, stdout, stderr, lockWait) through setupFake,
// none of which is safe to share. Today no test calls t.Parallel, so those mutations are safe; a
// future one added without noticing would race them into an intermittently green suite — the
// worst failure mode for a package whose whole job is proving guards fire.
func TestNoParallelTestsInPackage(t *testing.T) {
	entries, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no _test.go files found — this guard is looking in the wrong directory")
	}
	// Assembled at run time so this guard's own source does not contain the literal it searches
	// for — otherwise it reports itself and can never pass.
	needle := "t." + "Parallel("
	for _, name := range entries {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(trimmed, needle) {
				t.Errorf("%s:%d parallelises a test; this package's tests share mutable package state "+
					"(the forgeForFn / mintTokenFn / publicRepoGateFn seams, ghToken, stdout, stderr, lockWait "+
					"via setupFake). Make that state per-test before parallelising anything here.", name, i+1)
			}
		}
	}
}
