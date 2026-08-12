#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_file="$repo_root/.tmp/benchmark-gate.txt"

mkdir -p "$repo_root/.cache" "$repo_root/.tmp"
GOCACHE="$repo_root/.cache" GOTMPDIR="$repo_root/.tmp" TMPDIR="$repo_root/.tmp" CGO_ENABLED=0 \
	go test -run='^$' -bench='^Benchmark(RouterStatic|RouterParam|MinimalAllocEndpoint)$' \
	-benchmem -count=3 . | tee "$output_file"

awk '
	/^BenchmarkRouterStatic-/ { static_seen++; if ($(NF-1) > 4 || $(NF-3) > 128) failed=1 }
	/^BenchmarkRouterParam-/ { param_seen++; if ($(NF-1) > 4 || $(NF-3) > 128) failed=1 }
	/^BenchmarkMinimalAllocEndpoint-/ { minimal_seen++; if ($(NF-1) > 3 || $(NF-3) > 128) failed=1 }
	END {
		if (static_seen != 3 || param_seen != 3 || minimal_seen != 3) {
			print "benchmark-gate: expected three samples for each benchmark" > "/dev/stderr"
			exit 1
		}
		if (failed) {
			print "benchmark-gate: allocation budget exceeded" > "/dev/stderr"
			exit 1
		}
	}
' "$output_file"

echo "benchmark-gate: allocation budgets satisfied"
