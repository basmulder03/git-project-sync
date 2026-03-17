# Release Checklist

Use this checklist before promoting a tag to a production release.

## Release Candidate Gate (v1.0)

- [ ] Acceptance mapping reviewed in `docs/engineering/acceptance-test-matrix.md` and all required automated/manual items are accounted for.
- [ ] Candidate version is explicitly marked as RC (`vX.Y.Z-rcN`) before final promotion tag.
- [ ] Release owner confirms go/no-go decision and records timestamp + approver.
- [ ] Manual release workflow inputs are prepared (`version`, `target_ref`, `prerelease`, RC gate approvals).

## Pre-Tag Gate (CI on main/PR)

- [ ] `test` job passed on Linux and Windows (`go test ./...`, integration suite, reliability subset).
- [ ] `coverage` job passed with package threshold enforcement (`scripts/ci/coverage.sh`).
- [ ] `perf` job passed; no benchmark regression beyond tolerance defined in `scripts/ci/perf_baselines.txt`.
- [ ] `security` job passed:
  - [ ] `govulncheck ./...` returned no unresolved vulnerabilities.
  - [ ] `gitleaks` secret scan returned no findings.

## Tag/Release Gate (release workflow)

- [ ] Triggered via Actions button (`Release` workflow, `workflow_dispatch`) with desired version.
- [ ] Workflow created and pushed the expected annotated tag for the selected ref.

- [ ] Release workflow passed on the candidate tag.
- [ ] Release workflow security checks passed:
  - [ ] `govulncheck ./...`
  - [ ] `gitleaks` secret scan
- [ ] All expected platform artifacts were built (`syncctl`, `syncd`, `synctui` for Linux/Windows amd64).
- [ ] `dist/checksums.txt` exists and includes every distributed artifact.
- [ ] SBOM artifacts were generated and attached:
  - [ ] `dist/sbom.spdx.json`
  - [ ] `dist/sbom.cdx.json`
- [ ] `dist/manifest.json` references checksummed release artifacts.

## Promotion Sign-Off

- [ ] Manual spot-check: run `--version` for at least one Linux and one Windows binary from release artifacts.
- [ ] Install/update path sanity-check completed (bootstrap/setup flow).
- [ ] No unresolved critical incidents or rollback blockers in operations tracker.
- [ ] Final approval recorded by release owner.

## LTS Promotion (major version releases only)

Complete these steps when releasing a new major version that triggers the previous major line entering LTS.

- [ ] LTS branch `release/vX.Y` created from the previous GA tag.
- [ ] `docs/LTS_POLICY.md` version table updated with correct release date and EOL date.
- [ ] Weekly maintenance CI job confirmed active on the new LTS branch.
- [ ] Announcement published in repository Releases notes describing LTS timeline.
- [ ] All open backport-eligible PRs against `main` are evaluated for cherry-pick to `release/vX.Y`.

## Rollback Decision Checklist

- [ ] Previous stable artifacts are available and checksummed.
- [ ] Rollback trigger criteria are defined (for example: repeated `update_failed`, post-release sev-1, sustained SLO breach).
- [ ] Rollback owner and execution channel are identified.
- [ ] Verification commands after rollback are prepared (`syncctl --version`, `syncctl doctor`, `syncctl stats show`).
- [ ] Customer/operator communication template for rollback is prepared.

