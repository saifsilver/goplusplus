#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture="$repo_root/.tmp/api-normalization-fixture.txt"
actual="$repo_root/.tmp/api-normalization-actual.txt"
expected="$repo_root/.tmp/api-normalization-expected.txt"

mkdir -p "$repo_root/.tmp"
printf '%s\n' \
	'- Version: value changed from "v1.11.5" to "v1.11.7"' \
	'- ./cache.Store.Get: changed from func(string) to func(string) error' \
	'- Version: value changed from "development" to "v1.11.7"' > "$fixture"
printf '%s\n' \
	'- ./cache.Store.Get: changed from func(string) to func(string) error' \
	'- Version: value changed from "development" to "v1.11.7"' > "$expected"

sh "$repo_root/scripts/normalize_api_diff.sh" "$fixture" > "$actual"
diff -u "$expected" "$actual"
echo "api-compat-test: version normalization is narrow and stable"
