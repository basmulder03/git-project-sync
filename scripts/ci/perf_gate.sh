#!/usr/bin/env bash
# scripts/ci/perf_gate.sh
#
# Runs the benchmark suite and checks that no single benchmark regresses
# beyond the configured tolerance vs. a stored baseline.
#
# Usage:
#   bash scripts/ci/perf_gate.sh                   # compare vs baselines below
#   BENCH_UPDATE_BASELINE=1 bash scripts/ci/perf_gate.sh  # regenerate baselines
#
# The script exits 0 only if every checked benchmark is within tolerance.
# It does NOT fail if a benchmark is faster than the baseline.
#
# Tolerance: each benchmark may be at most TOLERANCE_PERCENT% slower than its
# baseline before the gate fails (default 50 — generous enough for CI noise
# while catching true regressions).
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${OUT_DIR:-${ROOT_DIR}/coverage}"
BASELINE_FILE="${ROOT_DIR}/scripts/ci/perf_baselines.txt"
RESULT_FILE="${OUT_DIR}/perf_results.txt"
TOLERANCE_PERCENT="${TOLERANCE_PERCENT:-50}"

mkdir -p "${OUT_DIR}"

PACKAGES=(
  "github.com/basmulder03/git-project-sync/internal/core/daemon"
  "github.com/basmulder03/git-project-sync/internal/core/state"
)

# ---------------------------------------------------------------------------
# Run benchmarks and collect ns/op values
# ---------------------------------------------------------------------------
run_benchmarks() {
  local tmpfile
  tmpfile="$(mktemp)"

  for pkg in "${PACKAGES[@]}"; do
    go test "${pkg}" \
      -run '^$' \
      -bench=. \
      -benchtime=3s \
      -benchmem \
      -count=1 \
      2>/dev/null \
    | awk '/^Benchmark/ { print $1, $3 }' \
    >> "${tmpfile}"
  done

  echo "${tmpfile}"
}

# ---------------------------------------------------------------------------
# Update baseline mode
# ---------------------------------------------------------------------------
if [[ "${BENCH_UPDATE_BASELINE:-0}" == "1" ]]; then
  echo "Regenerating perf baselines..."
  tmpfile="$(run_benchmarks)"
  cp "${tmpfile}" "${BASELINE_FILE}"
  rm -f "${tmpfile}"
  echo "Baselines written to ${BASELINE_FILE}"
  exit 0
fi

# ---------------------------------------------------------------------------
# Comparison mode
# ---------------------------------------------------------------------------
if [[ ! -f "${BASELINE_FILE}" ]]; then
  echo "No baseline file found at ${BASELINE_FILE}."
  echo "Run with BENCH_UPDATE_BASELINE=1 to generate baselines."
  echo "Skipping perf gate (no baseline)."
  exit 0
fi

tmpfile="$(run_benchmarks)"
cp "${tmpfile}" "${RESULT_FILE}"
rm -f "${tmpfile}"

echo "Performance regression check (tolerance: +${TOLERANCE_PERCENT}%)"
echo "=================================================================="

failures=0

while IFS=' ' read -r name baseline_ns; do
  [[ -z "${name}" || -z "${baseline_ns}" ]] && continue

  current_ns="$(awk -v n="${name}" '$1 == n { print $2; exit }' "${RESULT_FILE}")"
  if [[ -z "${current_ns}" ]]; then
    echo "SKIP  ${name} — not found in current results"
    continue
  fi

  # Calculate percent change: ((current - baseline) / baseline) * 100
  pct_change="$(awk -v c="${current_ns}" -v b="${baseline_ns}" \
    'BEGIN { printf "%.1f", ((c - b) / b) * 100 }')"

  # Compare using awk for floating-point arithmetic
  if awk -v p="${pct_change}" -v t="${TOLERANCE_PERCENT}" 'BEGIN { exit !(p > t) }'; then
    echo "FAIL  ${name}: ${current_ns} ns/op vs baseline ${baseline_ns} ns/op (${pct_change}% > +${TOLERANCE_PERCENT}%)"
    failures=$((failures + 1))
  else
    echo "PASS  ${name}: ${current_ns} ns/op vs baseline ${baseline_ns} ns/op (${pct_change}%)"
  fi
done < "${BASELINE_FILE}"

echo ""
if [[ ${failures} -gt 0 ]]; then
  echo "Performance gate FAILED: ${failures} benchmark(s) regressed beyond +${TOLERANCE_PERCENT}%."
  exit 1
fi

echo "All performance benchmarks within tolerance."
