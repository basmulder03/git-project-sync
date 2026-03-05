# Release Process

This project supports a manual one-click release flow from GitHub Actions.

## Workflow Entry Point

Use the `Release` workflow (`workflow_dispatch`) and provide:

- `version` (required): `vX.Y.Z` or `vX.Y.Z-rcN`
- `target_ref` (required): branch/commit to release (default `main`)
- `prerelease` (optional): mark GitHub release as pre-release
- `rc_gate_approved` (required)
- `rollback_plan_reviewed` (required)

## What the Workflow Does

1. Validates version format.
2. Creates and pushes an annotated tag for the selected ref.
3. Runs test/security gates.
4. Builds release artifacts for Linux/Windows.
5. Generates checksums and SBOMs.
6. Builds release manifest.
7. Publishes GitHub Release.

## Required Pre-Checks

- Complete `docs/RELEASE_CHECKLIST.md`.
- Confirm acceptance mapping in `docs/ACCEPTANCE_TESTS.md`.
- Ensure no unresolved release blockers.

## Post-Release Checks

- Verify artifact `--version` output for Linux and Windows binaries.
- Run install/update sanity checks.
- Monitor first daemon cycles for elevated error reason codes.
