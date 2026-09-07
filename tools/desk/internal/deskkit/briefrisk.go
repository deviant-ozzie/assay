package deskkit

// briefrisk.go — resolving a PR's OWNING BRIEF from its body's `Brief:` trailer, and
// reading that brief's OWN gate/risk frontmatter as a risk-classification signal.
//
// WHY THIS EXISTS. Risk classification for the ready-flip gate was reading the diff and
// the repo visibility, but never the brief the PR delivers. A brief declares its own
// sensitivity in its frontmatter — `gate: human` and the four `risk:` flags — and that
// declaration is authoritative for work whose sensitivity lives in intent rather than in
// a touched path. A PR whose brief is human-gated (or answers any risk flag `yes`) that
// changes only code the compiled path-triggers do not name was therefore classed NOT
// risk-classed, and the `Security-Review: pass` requirement the gate exists to enforce
// never fired. This resolves the owning brief from the trailer and reads that frontmatter
// so the declared sensitivity is consulted.
//
// TRAILER PARSE is the shared ParseTrailers (trailer.go); FILE RESOLUTION is against the
// CONFIGURED stream roots (roots.go), the same roots the multi-repo board covers. The
// value-splitter here is a small LOCAL helper (splitBriefRef) rather than a shared export,
// deliberately — the shared canonicalisers are being introduced elsewhere, and this change
// stays self-contained to avoid colliding with them; a later consolidation can fold this
// onto the shared splitter once that lands.
//
// ADDITIVE ONLY, like every other risk term (riskclassifier.go): this signal is read
// exclusively to ADD risk classification. A brief that cannot be resolved or read
// contributes NOTHING (RiskClassed stays false) and the diff/visibility/label terms still
// decide — the brief term never WAIVES the gate, so an unreadable brief cannot fail the
// gate open. Its ABSENCE is never consulted; only a positively-read `gate: human` / risk
// `yes` widens.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BriefRisk is the result of resolving a PR body's `Brief:` trailer.
type BriefRisk struct {
	// OwningBrief is the canonical "<stream>/<NN>" id the body's `Brief:` trailer names,
	// or "" when the body carries no `Brief:` trailer (or one that does not parse). It is
	// set from the trailer whether or not the brief FILE could be found, so a caller can
	// attribute the PR to its brief even on a root this process is not configured for.
	OwningBrief string
	// Resolved is true when OwningBrief named a brief file that was found and read under
	// the repo's configured root. When false, RiskClassed is not a claim about the brief —
	// the declaration could not be consulted.
	Resolved bool
	// RiskClassed is true when the resolved brief's frontmatter gates on a human
	// (`gate: human`) OR answers ANY `risk:` flag `yes`. False whenever the brief was not
	// resolved (see the additive-only contract above).
	RiskClassed bool
	// Reason is a short human-readable explanation, for a refusal message. "" when not
	// risk-classed.
	Reason string
}

// briefGateLine matches the top-level `gate:` frontmatter line.
var briefGateLine = regexp.MustCompile(`(?m)^gate:\s*(.*)$`)

// briefRiskKeys are the four canonical risk-answer keys, in the brief-v1 order.
var briefRiskKeys = []string{"regulatory", "customer", "irreversible", "sensitive-data"}

// BriefRiskFromBody parses body for a `Brief:` trailer, resolves it to a brief file under
// the repo's configured stream root, reads the brief frontmatter, and reports the owning
// brief id plus whether the brief's OWN declaration risk-classes the PR.
//
// It never errors: every failure to resolve or read the brief yields a BriefRisk that is
// not risk-classed (additive-only — see the file header). Both the slash form
// `<stream>/<NN>` and the colon forms `<...>:<stream>:<NN>` are accepted.
func BriefRiskFromBody(repo, body string) BriefRisk {
	trs, err := ParseTrailers([]byte(body))
	if err != nil {
		return BriefRisk{}
	}
	var val string
	for _, t := range trs {
		if t.Kind == TrailerBrief {
			val = t.Value
			break
		}
	}
	if val == "" {
		return BriefRisk{}
	}
	stream, nn, ok := splitBriefRef(val)
	if !ok {
		return BriefRisk{}
	}
	owning := stream + "/" + nn

	root := RootForRepo(repo)
	if root == "" {
		// Named, but this process is not configured with a root for the repo — attribute
		// the PR to its brief, but make no risk claim we cannot substantiate.
		return BriefRisk{OwningBrief: owning}
	}
	matches, _ := filepath.Glob(filepath.Join(root, "docs", "streams", stream, "brief-"+nn+"-*.md"))
	if len(matches) == 0 {
		return BriefRisk{OwningBrief: owning}
	}
	raw, rerr := os.ReadFile(matches[0])
	if rerr != nil {
		return BriefRisk{OwningBrief: owning}
	}
	classed, reason := briefFrontmatterRisk(string(raw))
	return BriefRisk{OwningBrief: owning, Resolved: true, RiskClassed: classed, Reason: reason}
}

// briefFrontmatterRisk reads a brief file's frontmatter and reports whether it declares
// the PR risk-bearing: `gate: human`, OR any of the four `risk:` flags answered `yes`
// (both the inline flow map and per-line forms). Matching is confined to the frontmatter
// fence so a `sensitive-data: yes` in prose cannot risk-class a PR by accident.
func briefFrontmatterRisk(content string) (classed bool, reason string) {
	block := briefFrontmatterFence(content)
	if block == "" {
		return false, ""
	}
	if m := briefGateLine.FindStringSubmatch(block); m != nil {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(m[1]), `"'`), "human") {
			return true, "owning brief is human-gated (gate: human)"
		}
	}
	for _, k := range briefRiskKeys {
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(k) + `\s*:\s*yes`)
		if re.MatchString(block) {
			return true, "owning brief declares risk: " + k + " = yes"
		}
	}
	return false, ""
}

// briefFrontmatterFence returns the text between the leading `---` fences, or "" when the
// content does not open with one. Mirrors the small extractors verifyloop/deskdispatch
// carry — deskkit keeps its own so it does not import a command package.
func briefFrontmatterFence(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n")
		}
	}
	return ""
}

// splitBriefRef reduces the accepted `Brief:` trailer value forms to (stream, NN): the
// slash form `<stream>/<NN>`, and the colon forms `<stream>:<NN>`, `<repo>:<stream>:<NN>`,
// and `<cell>:<repo>:<stream>:<NN>`. For the colon forms the LAST two parts are stream and
// NN; any repo/cell prefixes are not needed for the file resolution here. NN must be
// numeric. It is a package-LOCAL helper on purpose — see the file header.
func splitBriefRef(v string) (stream, nn string, ok bool) {
	v = strings.TrimSpace(v)
	var parts []string
	if strings.Contains(v, ":") {
		parts = strings.Split(v, ":")
		if len(parts) < 2 {
			return "", "", false
		}
		stream, nn = parts[len(parts)-2], parts[len(parts)-1]
	} else {
		parts = strings.Split(v, "/")
		if len(parts) != 2 {
			return "", "", false
		}
		stream, nn = parts[0], parts[1]
	}
	stream, nn = strings.TrimSpace(stream), strings.TrimSpace(nn)
	if stream == "" || nn == "" {
		return "", "", false
	}
	for _, c := range nn {
		if c < '0' || c > '9' {
			return "", "", false
		}
	}
	return stream, nn, true
}
