#!/usr/bin/env bash
#
# Per-package statement-coverage gate (constitution v1.2.0: >=95% per package).
#
# Enforces a per-package floor from a Go coverage profile so a change that drops a
# package below its floor fails CI rather than silently eroding coverage.
#
# Usage:
#   scripts/check-coverage.sh [coverage-profile]
# The profile defaults to coverage.out and must be produced with
#   go test -coverprofile=<profile> ./...
#
# Packages not listed in FLOOR use DEFAULT_MIN. Overrides exist only for thin glue
# whose remaining uncovered statements are genuinely untestable (os.Exit, the exec
# that replaces the process, defensive syscall-type assertions). They are a
# no-regression ratchet — raise them as coverage improves, never lower them.

set -euo pipefail

PROFILE="${1:-coverage.out}"
DEFAULT_MIN=95.0

# Package import path -> minimum percent.
declare -A FLOOR=()

if [[ ! -f "$PROFILE" ]]; then
	echo "coverage profile not found: $PROFILE" >&2
	echo "produce it with: go test -coverprofile=$PROFILE ./..." >&2
	exit 2
fi

pkg_pcts="$(awk '
	/^mode:/ { next }
	{
		file = $1
		sub(/:[0-9].*$/, "", file)
		n = split(file, parts, "/")
		pkg = parts[1]
		for (i = 2; i < n; i++) pkg = pkg "/" parts[i]
		stmts = $(NF - 1)
		hits  = $NF
		total[pkg] += stmts
		if (hits > 0) covered[pkg] += stmts
	}
	END {
		for (p in total) {
			pct = (total[p] > 0) ? (covered[p] * 100.0 / total[p]) : 100.0
			printf "%s %.1f\n", p, pct
		}
	}
' "$PROFILE" | sort)"

if [[ -z "$pkg_pcts" ]]; then
	echo "no coverage data found in $PROFILE" >&2
	exit 2
fi

fail=0
printf '%-50s %8s %8s   %s\n' "PACKAGE" "COVERAGE" "FLOOR" "STATUS"
while read -r pkg pct; do
	min="${FLOOR[$pkg]:-$DEFAULT_MIN}"
	if awk "BEGIN{exit !($pct + 0 < $min + 0)}"; then
		status="FAIL"
		fail=1
	else
		status="ok"
	fi
	printf '%-50s %7s%% %7s%%   %s\n' "${pkg#github.com/antst/sighelper/}" "$pct" "$min" "$status"
done <<<"$pkg_pcts"

echo
if [[ "$fail" -ne 0 ]]; then
	echo "coverage gate FAILED: a package is below its floor (default ${DEFAULT_MIN}%)." >&2
	exit 1
fi
echo "coverage gate passed: every package meets its floor (default ${DEFAULT_MIN}%)."
