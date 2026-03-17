# LTS Support Policy

This document defines the long-term support (LTS) lifecycle for git-project-sync, including support windows, patch cadence, backport rules, and maintenance automation.

## Support Tiers

| Tier | Label | Description |
| --- | --- | --- |
| **Active** | `active` | Receives new features, bug fixes, and security patches on every release. |
| **LTS** | `lts` | Receives security patches and critical bug fixes only. No new features. |
| **EOL** | `eol` | No further updates. Users must upgrade. |

## Release Cadence

- **Minor releases** (`vX.Y.0`): approximately every 8–12 weeks during active development.
- **Patch releases** (`vX.Y.Z`): as needed for security fixes or critical bugs; within 7 days of confirmed vulnerability.
- **LTS patch releases**: same 7-day SLA for security; critical bugs within 14 days.

## Support Windows

| Version | Released | Active Until | LTS Until | EOL |
| --- | --- | --- | --- | --- |
| v1.x | TBD | v2.0 GA | 12 months after v2.0 GA | 18 months after v2.0 GA |

When a new major version (vN.0.0) is released:

- The previous major line (`v(N-1).x`) enters **LTS** immediately.
- LTS lasts **12 months** from the date of the new major GA.
- The line is **EOL** 18 months after the new major GA (6-month runout after LTS expires).

## Backport Policy

A fix is eligible for backport to an LTS branch when it meets **at least one** criterion:

- Resolves a security vulnerability (CVSS ≥ 4.0) disclosed via responsible disclosure or GitHub Security Advisories.
- Fixes a data-loss or data-corruption bug.
- Fixes a crash or panic in an automated sync or daemon path.
- Is explicitly requested by a maintainer and approved by a second reviewer.

**Non-eligible backports:**

- New features or enhancements.
- Refactors with no behavioral change.
- Test-only changes (unless required to validate a backport fix).
- Performance improvements (unless they also fix a correctness issue).

## LTS Branch Naming Convention

LTS branches follow the pattern `release/vX.Y`, for example `release/v1.0`.

Maintenance workflow:

1. Create `release/vX.Y` from the GA tag when the major enters LTS.
2. Apply fixes via PR targeting `release/vX.Y`, cherry-picked from `main` where possible.
3. Tag patch releases as `vX.Y.Z` from the LTS branch.
4. The CI `release.yml` workflow triggers on `v*` tags from any branch.

## Maintenance Automation

The `maintenance` CI job (`.github/workflows/release.yml`) runs weekly on LTS branches and:

1. Checks for new Go vulnerability advisories (`govulncheck`).
2. Checks for dependency updates (`go list -m -u all`).
3. Scans for secrets (`gitleaks`).
4. Opens an issue if any check fails, labelled `lts-maintenance`.

See `.github/workflows/release.yml` — `maintenance` job for the automation definition.

## Security Disclosure

Report security vulnerabilities via GitHub Security Advisories:

1. Navigate to the repository Security tab and choose **Report a vulnerability**.
2. Do not open a public issue for a security report.
3. Maintainers acknowledge within 72 hours and aim to release a patch within 7 days.

## PAT Token and Credential Policy

PAT tokens and credentials are never stored in source control. See `docs/reference/pat-permission-requirements.md` for required scopes and the security implications of each.

## Definition of Done for LTS Adoption

- [ ] LTS branch `release/vX.Y` created from GA tag.
- [ ] `docs/LTS_POLICY.md` version table updated with correct dates.
- [ ] Weekly maintenance CI job is active on the LTS branch.
- [ ] Release candidate checklist updated with LTS section.
- [ ] Announcement published in repository Discussions or Releases notes.
