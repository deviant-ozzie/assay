### Added
- **Acceptance/ruling citation corroboration** (`statusgen --corroborate`) — a second
  lane alongside the `human:<name>` stamp check. It reads FREE-PROSE and commit-message
  claims that a configured human ACCEPTED or RULED ON something (e.g. "&lt;name&gt; accepted
  this on #1583", "per &lt;name&gt;'s ruling") and requires a comment or review authored by
  that human on the cited issue/PR. An unlinked claim, or one with no such artifact on the
  cited issue/PR, is `MISSING-CORROBORATION` — the same non-zero exit the stamp lane uses.
  Detection is anchored on names an adopter has declared human in `ASSAY_HUMAN_LOGIN_MAP`,
  so it hardcodes no name and stays inert when unconfigured; the corroboration read is a
  live, possibly cross-repo lookup of the cited artifact, which is why it lives on the
  network-capable `--corroborate` verb rather than the offline `--lint` gate. Closes a
  laundering surface one over from the register stamp: a fabricated human acceptance
  written into a durable governance artifact (a runbook, a brief, a commit record) that no
  human artifact stands behind. Logic in `statusgen/citationcorroborate.go`.
