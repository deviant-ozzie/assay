### Fixed
- **A PR whose sensitivity is declared in its brief frontmatter (`gate: human`, or any
  `risk:` flag `yes`) is now risk-classed by both `deskboard` and `deskflip`, even when its
  changed paths hit no compiled trigger.** Previously the owning brief never resolved, so the
  board marked such a PR `FLIP` and `deskflip` required no `Security-Review: pass` — a
  human-gated, sensitive-data change was flippable with no security verdict. `deskflip`'s
  security lane now REFUSES the ready flip on such a PR until a reviewer App posts
  `Security-Review: pass` at the current head (absence is never a pass).

### Changed
- **`deskboard` resolves a PR's owning brief from the body's `Brief:` trailer** (both the
  `<stream>/<NN>` slash form and the `<…>:<stream>:<NN>` colon form), falling back to
  branch-as-claim only when the body names no brief. The board's `riskClassed` and
  `deskflip`'s risk classification now UNION a brief term over the existing visibility,
  security-surface-label, and changed-path terms. The brief term is **additive only**: an
  unresolvable or unreadable brief contributes nothing and the other terms still decide, so it
  can only ADD scrutiny — it never waives the gate.

### Added
- `deskkit.BriefRiskFromBody(repo, body)` resolves the `Brief:` trailer to a brief file under
  the configured stream roots and reads its `gate:`/`risk:` frontmatter, returning the owning
  brief id and whether the brief's own declaration risk-classes the PR. `PullRequest` grows a
  `Body` field to feed it. (A later change can consolidate the local trailer splitter onto the
  shared canonicaliser once that lands.)
