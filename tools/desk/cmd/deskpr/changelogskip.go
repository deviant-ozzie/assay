package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// changelogSkipLabelName is the waiver label the per-PR changelog-fragment gate honours.
// A repo that enforces the convention greens its changelog leg on EITHER a
// `changelog/<slug>.md` fragment in the diff OR this label on the PR.
const changelogSkipLabelName = "changelog:skip"

// THE PROBLEM THIS SOLVES. Where a repo enforces the fragment convention, the gate is a
// required check, and a documentation-only PR — a board/Evidence/docs write that ships no
// behaviour — has nothing a changelog entry could truthfully say about it. Left alone such
// a PR sits red forever on a check whose only honest fix is the waiver label, and the
// waiver is not the author's to apply, so the PR waits on a human for a decision nobody
// disagrees about. Deciding it here, from the diff, removes the wait without moving the
// judgement: "every changed path is documentation" is a mechanical fact, not a call about
// whether a change is notable.
//
// THE SCOPE IS DELIBERATELY NARROW, AND IT NEVER WIDENS THE GATE.
//
//  1. It fires ONLY when every changed path lives under `docs/`. One path outside
//     disqualifies the whole PR — a mixed diff is a code PR that happens to touch docs,
//     and it owes a fragment like any other.
//  2. It fires ONLY when the diff adds no `changelog/` fragment. A PR that brought its own
//     fragment has already satisfied the gate; labelling it would say the opposite of what
//     the author did.
//  3. It fires ONLY on a repo that actually carries the gate. Elsewhere the label means
//     nothing and applying it is noise.
//  4. Every read it cannot make FAILS OPEN to doing nothing: an unreadable diff, an
//     unreadable tree, a forge that refuses the label. Failing open here leaves the gate
//     exactly as it was — the PR goes red on the ordinary missing-fragment path, which is
//     visible and has a documented fix. Failing closed would convert an unreadable file
//     list into an unexplained failure of a create that already succeeded.
//
// WHERE THE AUTHORITY ACTUALLY SITS. This asks the forge for a label; the forge decides.
// A maintainer who wants the waiver to stay a human act keeps it a human act by not
// granting the PR-opening App `pull-requests: write` on the repo — the request then fails
// and this path degrades to a stderr warning with the gate still in force. Nothing here
// can grant a permission the installation does not already hold, and a repo whose
// changelog workflow decides a docs-only waiver in-workflow (from PR metadata the
// automation cannot forge) is unaffected either way: the label is redundant there, not
// load-bearing.

// changelogGateWired reports whether the repo checked out at dir enforces the per-PR
// changelog-fragment convention. Two independent signals, either of which is sufficient:
// a `changelog/` directory in the tree, or a workflow file whose name says changelog.
// Detected from the tree rather than remembered, so the answer tracks the repo in front of
// the tool. Anything it cannot read reads as "not wired" — the fail-open direction.
func changelogGateWired(dir string) bool {
	if fi, err := os.Stat(filepath.Join(dir, "changelog")); err == nil && fi.IsDir() {
		return true
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".github", "workflows"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.Contains(strings.ToLower(e.Name()), "changelog") {
			return true
		}
	}
	return false
}

// docsOnlyPaths reports whether names is a NON-EMPTY path list every element of which
// lives under `docs/`, with no `changelog/` fragment among them.
//
// The empty list is deliberately NOT docs-only. A diff with no paths in it is a read that
// told us nothing, and a waiver granted on the strength of nothing is the one outcome this
// decision must never produce.
func docsOnlyPaths(names []string) bool {
	sawOne := false
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		sawOne = true
		// Checked before the docs/ test rather than relying on the two prefixes being
		// disjoint: the fragment rule is the one this decision is about, so it is stated,
		// not inferred from path layout that a future repo could rearrange.
		if strings.HasPrefix(n, "changelog/") {
			return false
		}
		if !strings.HasPrefix(n, "docs/") {
			return false
		}
	}
	return sawOne
}

// changedPathsVsBase lists the paths this branch changes relative to the base the PR
// opens against. Three-dot, matching what the gate itself reads: the changes THIS branch
// introduces, not everything that has happened on the base since it forked.
//
// --no-renames is load-bearing, not tidiness. With rename detection on, `--name-only`
// reports a rename as its DESTINATION path alone, so moving a source file to
// `docs/whatever.go` would present as a diff every path of which is under docs/ — a
// documentation-only verdict on a change that moved code. Disabling detection lists the
// removal and the addition as the two paths they are, and the source path outside docs/
// disqualifies the PR as it should.
func changedPathsVsBase(f *gitFacts) ([]string, error) {
	out, err := git(f.dir, "diff", "--no-renames", "--name-only", f.baseRef+"...HEAD")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// maybeApplyChangelogSkip applies the changelog waiver label to a JUST-CREATED PR when,
// and only when, the four conditions above hold. It returns a suffix for the create's
// audit detail line when the label landed, and the empty string on every other path.
//
// ADVISORY BY CONSTRUCTION. It runs after `gh pr create` has already returned a URL, so
// the PR exists whatever happens here; nothing on this path may turn a create that
// succeeded into a reported failure. Every non-applying outcome is a stderr note or
// silence, never a returned error — the same shape warnIfConflicting uses for the same
// reason.
func maybeApplyChangelogSkip(f *gitFacts, prNum int) string {
	if !changelogGateWired(f.dir) {
		return ""
	}
	names, err := changedPathsVsBase(f)
	if err != nil {
		fmt.Fprintf(deskprStderr, "deskpr: NOTICE could not read the changed-path list vs %s for %s#%d — "+
			"leaving the changelog gate in force (%v)\n", f.baseRef, f.repo, prNum, err)
		return ""
	}
	if !docsOnlyPaths(names) {
		return ""
	}
	if _, lerr := gh(f.dir, "pr", "edit", strconv.Itoa(prNum), "-R", f.repo,
		"--add-label", changelogSkipLabelName); lerr != nil {
		fmt.Fprintf(deskprStderr, "deskpr: WARNING could not apply %s to %s#%d — the changelog gate stays in "+
			"force, so this PR needs a fragment or the label from someone who can set it (%v)\n",
			changelogSkipLabelName, f.repo, prNum, lerr)
		return ""
	}
	fmt.Fprintf(deskprStderr, "deskpr: applied %s to %s#%d — every changed path is under docs/ and the diff "+
		"adds no changelog/ fragment\n", changelogSkipLabelName, f.repo, prNum)
	return " — " + changelogSkipLabelName + " applied (documentation-only diff)"
}
