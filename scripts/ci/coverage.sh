#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/coverage}"
PROFILE_DIR="${OUT_DIR}/profiles"
SUMMARY_FILE="${OUT_DIR}/summary.txt"
REPORT_FILE="${OUT_DIR}/report.md"

mkdir -p "${PROFILE_DIR}"

# Critical package thresholds (percent).
declare -A THRESHOLDS=(
  ["github.com/basmulder03/git-project-sync/cmd/syncctl"]=55
  ["github.com/basmulder03/git-project-sync/internal/core/daemon"]=75
  ["github.com/basmulder03/git-project-sync/internal/core/git"]=65
  ["github.com/basmulder03/git-project-sync/internal/core/install"]=60
  ["github.com/basmulder03/git-project-sync/internal/core/state"]=65
  ["github.com/basmulder03/git-project-sync/internal/core/sync"]=70
  ["github.com/basmulder03/git-project-sync/internal/core/update"]=65
  ["github.com/basmulder03/git-project-sync/internal/core/workspace"]=75
)

mapfile -t PACKAGES < <(printf '%s\n' "${!THRESHOLDS[@]}" | sort)

printf "Coverage threshold report\n" >"${SUMMARY_FILE}"
printf "=========================\n\n" >>"${SUMMARY_FILE}"

{
  printf "# Coverage Report\n\n"
  printf "| Package | Coverage | Threshold | Result |\n"
  printf "| --- | ---: | ---: | --- |\n"
} >"${REPORT_FILE}"

failures=0
for package in "${PACKAGES[@]}"; do
  profile_file="${PROFILE_DIR}/$(echo "${package}" | tr '/' '_').out"
  go test "${package}" -coverprofile "${profile_file}" >/dev/null

  coverage_line="$(go tool cover -func "${profile_file}" | awk '/^total:/ {print $3}')"
  coverage_percent="${coverage_line%%%}"
  threshold="${THRESHOLDS[${package}]}"

  result="PASS"
  if ! awk "BEGIN { exit !(${coverage_percent} >= ${threshold}) }"; then
    result="FAIL"
    failures=$((failures + 1))
  fi

  printf "%s coverage=%s%% threshold=%s%% result=%s\n" "${package}" "${coverage_percent}" "${threshold}" "${result}" >>"${SUMMARY_FILE}"
  printf "| %s | %.1f%% | %s%% | %s |\n" "${package}" "${coverage_percent}" "${threshold}" "${result}" >>"${REPORT_FILE}"
done

if [[ ${failures} -gt 0 ]]; then
  printf "\nCoverage thresholds failed for %d package(s).\n" "${failures}" >>"${SUMMARY_FILE}"
  cat "${SUMMARY_FILE}"
  exit 1
fi

printf "\nAll coverage thresholds passed.\n" >>"${SUMMARY_FILE}"
cat "${SUMMARY_FILE}"
