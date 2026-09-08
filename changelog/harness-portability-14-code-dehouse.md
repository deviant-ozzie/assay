### Added
- Harness-portability code de-house: the stream's tool and packaging deliverables now live in
  the public tree — three self-contained Go modules (`tools/harnessgen`, `tools/harnesslint`,
  `tools/plugindrift`), the bundle's provenance and packaging (`plugins/assay/SOURCES.yaml`,
  `PARITY.md`, `RELEASE-NOTES.md`, `.codex-plugin/plugin.json`, the generated `codex/` and
  `cursor/` packaging, `resident-rules.md` and its generated payload), the two capability
  matrices under `docs/research/`, and the Codex smoke protocol. The `harnessgen`/`harnesslint`
  generators are discovered by CI's existing Go-module walk, so "Assay runs natively on Codex and
  Cursor" is now checkable in this repository.

### Changed
- `freshness.yaml` registers the two harness capability matrices and the three per-harness
  binding files under a 45-day re-review leash.
