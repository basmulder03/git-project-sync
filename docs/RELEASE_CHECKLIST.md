# Release Checklist

Use this checklist before promoting a tag to a production release.

## Pre-Tag Gate (CI on main/PR)

- [ ] `test` job passed on Linux and Windows (`go test ./...`, integration suite, reliability subset).
- [ ] `coverage` job passed with package threshold enforcement (`scripts/ci/coverage.sh`).
- [ ] `security` job passed:
  - [ ] `govulncheck ./...` returned no unresolved vulnerabilities.
  - [ ] `gitleaks` secret scan returned no findings.

## Tag/Release Gate (release workflow)

- [ ] Release workflow passed on the candidate tag.
- [ ] Release workflow security checks passed:
  - [ ] `govulncheck ./...`
  - [ ] `gitleaks` secret scan
- [ ] All expected platform artifacts were built (`syncctl`, `syncd`, `synctui`, `syncsetup` for Linux/Windows amd64).
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
