package main

import (
	"strings"
	"testing"
)

// The fixture roster (rosterfixture_test.go) maps ASSAY_HUMAN_LOGIN_MAP=alex:ada, so
// "alex" is the one CONFIGURED human in these tests and resolves to login "ada". Any
// other name is unmapped and must be ignored by the citation detector.

func TestCitationAcceptanceRe(t *testing.T) {
	// The regex captures "<token> <verb>" name-agnostically; the SEMANTIC filter
	// (only a HumanLogin-resolvable name is a citation) lives in detectCitations,
	// tested separately. Here we pin only that the verb forms are recognised and the
	// token immediately before the verb is captured as the candidate name.
	tests := []struct {
		line     string
		wantName string
	}{
		{"Alex accepted this contradiction on #1583", "alex"},
		{"alex approved the prod flip", "alex"},
		{"alex ruled that option (a) is the way", "alex"},
		{"alex decided to defer the flip", "alex"},
		{"alex acknowledged the change", "alex"},
		{"alex signed-off on the runbook", "alex"},
		{"alex signed off on the runbook", "alex"},
	}
	for _, tc := range tests {
		m := citationAcceptanceRe.FindStringSubmatch(tc.line)
		if m == nil {
			t.Errorf("acceptanceRe(%q): no match", tc.line)
			continue
		}
		if strings.ToLower(m[1]) != tc.wantName {
			t.Errorf("acceptanceRe(%q): name=%q want %q", tc.line, m[1], tc.wantName)
		}
	}
	// A line with an approval word but no name-before-verb adjacency is a non-match
	// only insofar as the captured token fails HumanLogin — the regex itself is
	// permissive by design. Assert the one shape it must NOT capture: a verb with no
	// preceding token at start-of-string.
	if citationAcceptanceRe.MatchString("approved by the team") {
		t.Errorf("acceptanceRe matched a leading verb with no preceding name token")
	}
}

func TestCitationPossessiveRe(t *testing.T) {
	for _, line := range []string{
		"per alex's ruling ('Option (a) is the way')",
		"alex's decision was recorded",
		"following alex’s approval", // curly apostrophe
	} {
		m := citationPossessiveRe.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("possessiveRe(%q): no match", line)
			continue
		}
		if strings.ToLower(m[1]) != "alex" {
			t.Errorf("possessiveRe(%q): name=%q want alex", line, m[1])
		}
	}
	// A bare "alex ruling" (no possessive marker) is NOT matched by the possessive
	// pattern (the verb pattern handles the "ruled" sense separately).
	if citationPossessiveRe.MatchString("alex ruling body") {
		t.Errorf("possessiveRe matched a non-possessive phrase")
	}
}

func TestExtractCitedRef(t *testing.T) {
	tests := []struct {
		line     string
		wantRepo string
		wantNum  int
		wantOK   bool
	}{
		{"alex accepted this on #1583", "", 1583, true},
		{"alex ruled on owner-x/repo-y#42 last week", "owner-x/repo-y", 42, true},
		{"see https://github.com/owner-x/repo-y/issues/99 for the ruling", "owner-x/repo-y", 99, true},
		{"see https://github.com/owner-x/repo-y/pull/77", "owner-x/repo-y", 77, true},
		{"alex accepted this, no reference at all", "", 0, false},
		// an owner/repo#N must not also be scavenged as a bare #N
		{"a/b#5", "a/b", 5, true},
	}
	for _, tc := range tests {
		repo, num, ok := extractCitedRef(tc.line)
		if ok != tc.wantOK || repo != tc.wantRepo || num != tc.wantNum {
			t.Errorf("extractCitedRef(%q) = (%q,%d,%v), want (%q,%d,%v)",
				tc.line, repo, num, ok, tc.wantRepo, tc.wantNum, tc.wantOK)
		}
	}
}

func TestDetectCitations_AnchorsOnConfiguredHuman(t *testing.T) {
	text := strings.Join([]string{
		"alex accepted this contradiction on #1583",  // configured human -> kept
		"casey approved the change on #1600",         // unmapped -> dropped
		"the compiler approved the optimization",      // not a human -> dropped
	}, "\n")
	cits := detectCitations("docs/runbook.md", text)
	if len(cits) != 1 {
		t.Fatalf("got %d citations, want 1 (only the configured human): %+v", len(cits), cits)
	}
	c := cits[0]
	if c.Name != "alex" || c.Number != 1583 || !c.HasRef {
		t.Errorf("citation = %+v, want name=alex number=1583 hasRef=true", c)
	}
	if c.Source != "docs/runbook.md" {
		t.Errorf("source = %q, want docs/runbook.md", c.Source)
	}
}

func TestCitationsInDiff_AddedLinesAndFixtureExclusion(t *testing.T) {
	diff := `diff --git a/docs/package-rename-runbook.md b/docs/package-rename-runbook.md
--- a/docs/package-rename-runbook.md
+++ b/docs/package-rename-runbook.md
@@ -40,0 +41,2 @@
+### 4a. Prod flip
+alex accepted this contradiction with the runbook on #1583
diff --git a/docs/streams/education/assay-tutorial-skeleton/README.md b/docs/streams/education/assay-tutorial-skeleton/README.md
--- a/docs/streams/education/assay-tutorial-skeleton/README.md
+++ b/docs/streams/education/assay-tutorial-skeleton/README.md
@@ -1,0 +2,1 @@
+alex accepted the tutorial step on #7
`
	cits := citationsInDiff("", diff)
	if len(cits) != 1 {
		t.Fatalf("got %d citations, want 1 (fixture-corpus path excluded): %+v", len(cits), cits)
	}
	if cits[0].Source != "docs/package-rename-runbook.md" {
		t.Errorf("source = %q, want the runbook", cits[0].Source)
	}
}

// makeArtifact builds a citedArtifact from comment/review author logins.
func makeArtifact(commentLogins, reviewLogins []string) *citedArtifact {
	a := &citedArtifact{}
	for _, l := range commentLogins {
		a.Comments = append(a.Comments, ghComment{Author: ghAuthor{Login: l}, URL: "https://x/" + l})
	}
	for _, l := range reviewLogins {
		a.Reviews = append(a.Reviews, ghReview{Author: ghAuthor{Login: l}, State: "COMMENTED"})
	}
	return a
}

func TestCorroborateCitations_Present_Corroborated(t *testing.T) {
	// The cited-artifact-PRESENT fixture: ada (alex's login) authored a comment on
	// the cited PR -> CORROBORATED.
	cit := citation{Name: "alex", Repo: "", Number: 1583, HasRef: true, Source: "docs/runbook.md"}
	fetched := map[string]*citedArtifact{
		"o/r#1583": makeArtifact([]string{"ada"}, nil),
	}
	res := corroborateCitations([]citation{cit}, fetched, "o/r")
	if len(res) != 1 || res[0].Verdict != verdictCorroborated {
		t.Fatalf("got %+v, want one CORROBORATED", res)
	}
}

func TestCorroborateCitations_PresentViaReview(t *testing.T) {
	cit := citation{Name: "alex", Repo: "o/r", Number: 1583, HasRef: true, Source: "docs/runbook.md"}
	fetched := map[string]*citedArtifact{
		"o/r#1583": makeArtifact(nil, []string{"ada"}),
	}
	res := corroborateCitations([]citation{cit}, fetched, "o/r")
	if res[0].Verdict != verdictCorroborated {
		t.Fatalf("review by the login should corroborate; got %+v", res[0])
	}
}

func TestCorroborateCitations_Fabricated_Flagged(t *testing.T) {
	// The FABRICATED fixture (the issue's live instance): the cited PR exists and
	// has activity, but NONE of it is by ada — the claimed acceptance has no
	// artifact behind it. MISSING-CORROBORATION.
	cit := citation{Name: "alex", Repo: "", Number: 1583, HasRef: true, Source: "docs/runbook.md"}
	fetched := map[string]*citedArtifact{
		"o/r#1583": makeArtifact([]string{"someone-else", "shared-agent"}, []string{"another-bot"}),
	}
	res := corroborateCitations([]citation{cit}, fetched, "o/r")
	if res[0].Verdict != verdictMissing {
		t.Fatalf("fabricated citation must be flagged; got %+v", res[0])
	}
	if !strings.Contains(res[0].Evidence, "no comment or review") {
		t.Errorf("evidence should name the missing artifact; got %q", res[0].Evidence)
	}
}

func TestCorroborateCitations_Unlinked_Flagged(t *testing.T) {
	// "per alex's ruling" in a commit message with no cited artifact at all.
	cit := citation{Name: "alex", HasRef: false, Source: "commit deadbee"}
	res := corroborateCitations([]citation{cit}, map[string]*citedArtifact{}, "o/r")
	if res[0].Verdict != verdictMissing {
		t.Fatalf("unlinked citation must be flagged; got %+v", res[0])
	}
	if !strings.Contains(res[0].Evidence, "NO cited issue/PR") {
		t.Errorf("evidence should say the claim is unlinked; got %q", res[0].Evidence)
	}
}

func TestCorroborateCitations_CrossRepoKey(t *testing.T) {
	// A citation naming an EXTERNAL repo (owner-x/repo-y#42) must look up that repo's
	// key, not the PR's own repo.
	cit := citation{Name: "alex", Repo: "owner-x/repo-y", Number: 42, HasRef: true, Source: "docs/x.md"}
	fetched := map[string]*citedArtifact{
		"owner-x/repo-y#42": makeArtifact([]string{"ada"}, nil),
		"o/r#42":            makeArtifact([]string{"ada"}, nil), // wrong repo, must be ignored
	}
	res := corroborateCitations([]citation{cit}, fetched, "o/r")
	if res[0].Verdict != verdictCorroborated {
		t.Fatalf("cross-repo citation should resolve against its own repo; got %+v", res[0])
	}
	if res[0].Citation.citedKey("o/r") != "owner-x/repo-y#42" {
		t.Errorf("citedKey = %q, want owner-x/repo-y#42", res[0].Citation.citedKey("o/r"))
	}
}
