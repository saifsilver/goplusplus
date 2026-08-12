#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
baseline_version=${API_BASELINE_VERSION:-v1.11.5}
module=github.com/saifsilver/goplusplus
apidiff="$repo_root/.tools/bin/apidiff"
expected="$repo_root/api/compatibility-exceptions-${baseline_version}.txt"
old_export="$repo_root/.tmp/api-old.export"
new_export="$repo_root/.tmp/api-new.export"
actual="$repo_root/.tmp/api-incompatible.txt"
sorted_expected="$repo_root/.tmp/api-expected.sorted"
sorted_actual="$repo_root/.tmp/api-actual.sorted"
normalized_expected="$repo_root/.tmp/api-expected.normalized"
normalized_actual="$repo_root/.tmp/api-actual.normalized"

test -x "$apidiff" || {
	echo "api-compat: apidiff is missing; run 'make install-tools'" >&2
	exit 1
}
test -f "$expected" || {
	echo "api-compat: missing reviewed exception file $expected" >&2
	exit 1
}

mkdir -p "$repo_root/.cache" "$repo_root/.tmp"
release_dir="$repo_root/.cache/mod/$module@$baseline_version"
if [ ! -d "$release_dir" ]; then
	metadata=$(GOMODCACHE="$repo_root/.cache/mod" go mod download -json "$module@$baseline_version")
	release_dir=$(printf '%s\n' "$metadata" | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')
fi
test -n "$release_dir" && test -d "$release_dir" || {
	echo "api-compat: could not resolve $module@$baseline_version" >&2
	exit 1
}

(
	cd "$release_dir"
	GOCACHE="$repo_root/.cache" GOTMPDIR="$repo_root/.tmp" TMPDIR="$repo_root/.tmp" \
		"$apidiff" -m -w "$old_export" "$module"
)
GOCACHE="$repo_root/.cache" GOTMPDIR="$repo_root/.tmp" TMPDIR="$repo_root/.tmp" \
	"$apidiff" -m -w "$new_export" "$module"
"$apidiff" -m -incompatible "$old_export" "$new_export" > "$actual"

sh "$repo_root/scripts/normalize_api_diff.sh" "$expected" > "$normalized_expected"
sh "$repo_root/scripts/normalize_api_diff.sh" "$actual" > "$normalized_actual"
LC_ALL=C sort "$normalized_expected" > "$sorted_expected"
LC_ALL=C sort "$normalized_actual" > "$sorted_actual"
if ! diff -u "$sorted_expected" "$sorted_actual"; then
	echo "api-compat: incompatible API changes differ from the reviewed exception set" >&2
	exit 1
fi

echo "api-compat: no unreviewed incompatibilities relative to $baseline_version"
