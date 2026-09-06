package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/medici-finance/assay/tools/desk/internal/deskkit"
)

// changelogFixtureOpts parameterises newChangelogFixture: what the BASE commit carries
// (which decides whether the repo reads as one that enforces the fragment convention) and
// what the BRANCH commit adds (which is the diff the waiver decision reads).
type changelogFixtureOpts struct {
	// baseFiles are written and committed on the default branch, before the branch is cut.
	baseFiles map[string]string
	// branchFiles are written and committed on feature/test-branch. They are the whole
	// three-dot diff the decision sees.
	branchFiles map[string]string
}

// newChangelogFixture is newBaseFixture's parameterised twin. It builds the same offline
// worktree — origin URL parsing to an allowed repo, pushes routed to a local bare, the
// `fixture/01` brief on the default branch so `Brief:` trailers resolve — but lets a test
// choose exactly which paths the base carries and which the branch changes, because those
// two facts ARE the changelog waiver decision.
func newChangelogFixture(t *testing.T, opts changelogFixtureOpts) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	mustGit(t, "", "init", "--bare", "-b", "main", bare)
	mustGit(t, "", "init", "-b", "main", work)
	mustGit(t, work, "config", "user.email", "t@e.st")
	mustGit(t, work, "config", "user.name", "Test")
	mustGit(t, work, "config", "commit.gpgsign", "false")
	mustGit(t, work, "remote", "add", "origin", ghURL)
	mustGit(t, work, "remote", "set-url", "--push", "origin", "file://"+bare)

	writeTree(t, work, map[string]string{
		"README.md": "seed\n",
		// The brief every test body's `Brief: fixture/01` trailer resolves against. On the
		// BASE commit, so it never appears in the branch diff.
		"docs/streams/fixture/brief-01-test.md": "---\nschema: brief-v1\nbrief: fixture/01\n" +
			"title: fixture brief\n---\n\nFixture brief for deskpr tests.\n",
	})
	writeTree(t, work, opts.baseFiles)
	mustGit(t, work, "add", "-A")
	mustGit(t, work, "commit", "-m", "base")
	mainSHA := mustGit(t, work, "rev-parse", "HEAD")
	mustGit(t, work, "update-ref", "refs/remotes/origin/main", mainSHA)
	mustGit(t, work, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	mustGit(t, work, "checkout", "-b", "feature/test-branch")
	writeTree(t, work, opts.branchFiles)
	mustGit(t, work, "add", "-A")
	mustGit(t, work, "commit", "-m", "branch work")
	return work
}

// writeTree writes each path (relative, slash-separated) with its content, creating
// parent directories as needed.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		writeFile(t, full, content)
	}
}

// gateWiredBase is the smallest base tree that makes a fixture repo read as one enforcing
// the per-PR fragment convention: a `changelog/` directory with its README in it.
var gateWiredBase = map[string]string{
	"changelog/README.md": "Per-PR changelog fragments live here.\n",
}

// createDocsBody is a body with a resolving trailer, used by every create below.
const createDocsBody = "records the verification\nBrief: fixture/01"

// addedChangelogSkipLabel reports whether any recorded gh argv asked the forge to add the
// waiver label — the single observable this behaviour is defined by.
func addedChangelogSkipLabel(calls [][]string) bool {
	return anyCall(ghCalls(calls), "pr", "edit", "--add-label", changelogSkipLabelName)
}

// TestCreateDocsOnlyDiffAppliesChangelogSkip is the FAIL-FIRST case for the new
// behaviour: on a repo that carries the fragment gate, a PR whose whole diff is under
// docs/ gets the waiver label applied at create. Before the change this test fails —
// nothing on the create path ever emitted a `--add-label`.
func TestCreateDocsOnlyDiffAppliesChangelogSkip(t *testing.T) {
	work := newChangelogFixture(t, changelogFixtureOpts{
		baseFiles: gateWiredBase,
		branchFiles: map[string]string{
			"docs/streams/fixture/brief-01-test.md": "---\nschema: brief-v1\nbrief: fixture/01\n" +
				"title: fixture brief\n---\n\nFixture brief for deskpr tests.\n\n## Evidence\n\n- row 1 ran\n",
			"docs/streams/fixture/README.md": "| 01 | fixture brief | verified |\n",
		},
	})
	calls := withEnv(t, work)
	stderr := withStderrCapture(t)

	if rc := run([]string{"create", "--title", "record evidence", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
		t.Fatalf("create rc = %d, want 0", rc)
	}
	if !addedChangelogSkipLabel(*calls) {
		t.Fatalf("a documentation-only diff on a gated repo must get %q applied at create; gh calls: %v",
			changelogSkipLabelName, ghCalls(*calls))
	}
	// The label lands on the PR the create just made, at the target repo — not on some
	// other number, and not without -R.
	if !anyCall(ghCalls(*calls), "pr", "edit", "101", "-R", "example-org/tracker", "--add-label", changelogSkipLabelName) {
		t.Fatalf("the label must be applied to the just-created PR #101 on the target repo; gh calls: %v", ghCalls(*calls))
	}
	if got := stderr.String(); !strings.Contains(got, changelogSkipLabelName) {
		t.Fatalf("applying the waiver must say so on stderr; stderr: %q", got)
	}
}

// TestCreateNonDocsDiffWithoutFragmentGetsNoLabel is the preserved-behaviour half. A diff
// that touches anything outside docs/ and brings no fragment is exactly the PR the gate
// exists to catch: deskpr must NOT waive it, and the create itself must be unchanged —
// same push, same always-draft create — so the gate goes red in CI as it always did.
func TestCreateNonDocsDiffWithoutFragmentGetsNoLabel(t *testing.T) {
	work := newChangelogFixture(t, changelogFixtureOpts{
		baseFiles: gateWiredBase,
		branchFiles: map[string]string{
			"tools/desk/cmd/deskpr/feature.go": "package main\n\nfunc feature() {}\n",
			"docs/streams/fixture/README.md":   "| 01 | fixture brief | implemented |\n",
		},
	})
	calls := withEnv(t, work)

	if rc := run([]string{"create", "--title", "add feature", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
		t.Fatalf("create rc = %d, want 0", rc)
	}
	if addedChangelogSkipLabel(*calls) {
		t.Fatalf("a diff touching a non-docs path must never be waived; gh calls: %v", ghCalls(*calls))
	}
	if !anyCall(gitCalls(*calls), "push", "-u", "origin", "feature/test-branch") {
		t.Fatalf("the existing create path must be unchanged (plain push); git calls: %v", gitCalls(*calls))
	}
	if !anyCall(ghCalls(*calls), "pr", "create", "--draft") {
		t.Fatalf("the existing create path must be unchanged (always --draft); gh calls: %v", ghCalls(*calls))
	}
}

// TestCreateDiffWithFragmentGetsNoLabel: a PR that brought its own fragment has already
// satisfied the gate. Labelling it would assert the opposite of what the author did — and
// the docs-only arm must not fire just because the rest of the diff is documentation.
func TestCreateDiffWithFragmentGetsNoLabel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		branch map[string]string
	}{
		{
			name: "fragment alongside documentation",
			branch: map[string]string{
				"changelog/feature-test-branch.md": "### Added\n\n- the thing\n",
				"docs/streams/fixture/README.md":   "| 01 | fixture brief | implemented |\n",
			},
		},
		{
			name: "fragment alone",
			branch: map[string]string{
				"changelog/feature-test-branch.md": "### Fixed\n\n- the other thing\n",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			work := newChangelogFixture(t, changelogFixtureOpts{
				baseFiles:   gateWiredBase,
				branchFiles: tc.branch,
			})
			calls := withEnv(t, work)

			if rc := run([]string{"create", "--title", "ship it", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
				t.Fatalf("create rc = %d, want 0", rc)
			}
			if addedChangelogSkipLabel(*calls) {
				t.Fatalf("a diff carrying a changelog fragment must never be waived; gh calls: %v", ghCalls(*calls))
			}
		})
	}
}

// TestCreateUngatedRepoGetsNoLabel: on a repo with neither a changelog/ tree nor a
// changelog workflow the label means nothing, so a docs-only PR there is left alone.
// This is the fail-open-to-nothing direction of the detection.
func TestCreateUngatedRepoGetsNoLabel(t *testing.T) {
	work := newChangelogFixture(t, changelogFixtureOpts{
		branchFiles: map[string]string{"docs/streams/fixture/README.md": "| 01 | fixture brief | verified |\n"},
	})
	calls := withEnv(t, work)

	if rc := run([]string{"create", "--title", "docs", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
		t.Fatalf("create rc = %d, want 0", rc)
	}
	if addedChangelogSkipLabel(*calls) {
		t.Fatalf("a repo that does not carry the gate must never be labelled; gh calls: %v", ghCalls(*calls))
	}
}

// TestCreateGateDetectedFromWorkflowAlone: the second detection signal on its own. A repo
// whose fragments directory has not been created yet, but whose CI carries the changelog
// workflow, still enforces the gate.
func TestCreateGateDetectedFromWorkflowAlone(t *testing.T) {
	work := newChangelogFixture(t, changelogFixtureOpts{
		baseFiles: map[string]string{
			".github/workflows/changelog-check.yml": "name: changelog-check\non: pull_request\n",
		},
		branchFiles: map[string]string{"docs/streams/fixture/README.md": "| 01 | fixture brief | verified |\n"},
	})
	calls := withEnv(t, work)

	if rc := run([]string{"create", "--title", "docs", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
		t.Fatalf("create rc = %d, want 0", rc)
	}
	if !addedChangelogSkipLabel(*calls) {
		t.Fatalf("the workflow alone must be enough to detect the gate; gh calls: %v", ghCalls(*calls))
	}
}

// TestCreateLabelRefusalIsAdvisory: the forge refusing the label — the shape a repo takes
// when it deliberately withholds `pull-requests: write` from the PR-opening App — must
// leave the create SUCCESSFUL and say so loudly. A create that already returned a URL is
// never turned into a failure by this path.
func TestCreateLabelRefusalIsAdvisory(t *testing.T) {
	work := newChangelogFixture(t, changelogFixtureOpts{
		baseFiles:   gateWiredBase,
		branchFiles: map[string]string{"docs/streams/fixture/README.md": "| 01 | fixture brief | verified |\n"},
	})
	calls := withEnv(t, work)
	stderr := withStderrCapture(t)
	t.Setenv("FAKEGH_EDIT_FAIL", "1")

	if rc := run([]string{"create", "--title", "docs", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
		t.Fatalf("a refused label must not fail the create; rc = %d, want 0", rc)
	}
	if !addedChangelogSkipLabel(*calls) {
		t.Fatalf("the label was never attempted; gh calls: %v", ghCalls(*calls))
	}
	if got := stderr.String(); !strings.Contains(got, "WARNING") || !strings.Contains(got, "stays in force") {
		t.Fatalf("a refused label must warn that the gate stays in force; stderr: %q", got)
	}
}

// TestDocsOnlyPaths pins the decision itself, including the case no create test can
// stage: an EMPTY path list. A waiver granted on the strength of a read that told us
// nothing is the one outcome this must never produce.
func TestDocsOnlyPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		names []string
		want  bool
	}{
		{"empty list is never a waiver", nil, false},
		{"blank lines only is never a waiver", []string{"", "  "}, false},
		{"single docs path", []string{"docs/streams/a/README.md"}, true},
		{"several docs paths", []string{"docs/a.md", "docs/streams/b/README.md"}, true},
		{"trailing blank line tolerated", []string{"docs/a.md", ""}, true},
		{"one non-docs path disqualifies", []string{"docs/a.md", "main.go"}, false},
		{"fragment disqualifies", []string{"changelog/x.md"}, false},
		{"fragment with docs disqualifies", []string{"docs/a.md", "changelog/x.md"}, false},
		{"a path merely starting with the word docs is not under docs/", []string{"docsite/a.md"}, false},
		{"repo-root file disqualifies", []string{"README.md"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := docsOnlyPaths(tc.names); got != tc.want {
				t.Fatalf("docsOnlyPaths(%q) = %v, want %v", tc.names, got, tc.want)
			}
		})
	}
}

// TestChangelogGateWired pins the detection against a tree, both signals and neither.
func TestChangelogGateWired(t *testing.T) {
	t.Run("changelog directory", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"changelog/README.md": "x\n"})
		if !changelogGateWired(dir) {
			t.Fatal("a changelog/ directory must read as the gate being wired")
		}
	})
	t.Run("workflow only", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{".github/workflows/changelog-check.yml": "name: changelog-check\n"})
		if !changelogGateWired(dir) {
			t.Fatal("a changelog workflow must read as the gate being wired")
		}
	})
	t.Run("neither", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{".github/workflows/ci.yml": "name: ci\n", "docs/a.md": "x\n"})
		if changelogGateWired(dir) {
			t.Fatal("a repo with neither signal must not read as gated")
		}
	})
	t.Run("a changelog FILE is not the fragments directory", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"changelog": "not a directory\n"})
		if changelogGateWired(dir) {
			t.Fatal("a regular file named changelog must not read as the fragments directory")
		}
	})
}

// TestCreateRenameIntoDocsGetsNoLabel is the rename case. Git's rename detection reports a
// rename as its DESTINATION path alone, so a source file MOVED under docs/ would otherwise
// present as a diff every path of which is documentation — a documentation-only verdict on
// a change that moved code. The source path must still disqualify it.
func TestCreateRenameIntoDocsGetsNoLabel(t *testing.T) {
	work := newChangelogFixture(t, changelogFixtureOpts{
		baseFiles: map[string]string{
			"changelog/README.md":              "Per-PR changelog fragments live here.\n",
			"tools/desk/cmd/deskpr/feature.go": "package main\n\nfunc feature() {}\n",
		},
		branchFiles: map[string]string{
			"docs/feature.go": "package main\n\nfunc feature() {}\n",
		},
	})
	// The move itself: the fixture wrote the destination, so remove the source to make the
	// branch commit a genuine rename that git's detector will pair up.
	mustGit(t, work, "rm", "-q", "tools/desk/cmd/deskpr/feature.go")
	mustGit(t, work, "commit", "-q", "-m", "move it")

	calls := withEnv(t, work)
	if rc := run([]string{"create", "--title", "move a file", "--body-min", createDocsBody}); rc != deskkit.ExitOK {
		t.Fatalf("create rc = %d, want 0", rc)
	}
	if addedChangelogSkipLabel(*calls) {
		t.Fatalf("a rename whose SOURCE lives outside docs/ must not be waived; gh calls: %v", ghCalls(*calls))
	}
}
