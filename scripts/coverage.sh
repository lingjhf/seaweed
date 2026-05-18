#!/usr/bin/env sh
set -eu

min_coverage="${COVER_MIN:-62.0}"
tmpdir="${TMPDIR:-/tmp}/seaweed-coverage-$$"
unit_profile="$tmpdir/unit.cover"
integration_profile="$tmpdir/integration.cover"

cleanup() {
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

mkdir -p "$tmpdir"

WEED_BINARY="${WEED_BINARY:-./weed}" go test -count=1 -coverprofile="$unit_profile" ./...
WEED_BINARY="${WEED_BINARY:-./weed}" go test -count=1 -tags=integration -coverprofile="$integration_profile" ./...

awk -v min="$min_coverage" '
	FNR == 1 {
		next
	}
	$1 ~ /\/examples\// || $1 ~ /\/internal\/testweed\// {
		next
	}
	{
		key = $1
		statements[key] = $2
		if ($3 > 0) {
			hit[key] = 1
		}
	}
	END {
		for (key in statements) {
			total += statements[key]
			if (hit[key]) {
				covered += statements[key]
			}
		}
		coverage = int((covered * 1000 / total) + 0.5) / 10
		printf "combined production coverage: %.1f%% (%d/%d statements)\n", coverage, covered, total
		if (coverage < min) {
			printf "coverage %.1f%% is below required %.1f%%\n", coverage, min > "/dev/stderr"
			exit 1
		}
	}
' "$unit_profile" "$integration_profile"
