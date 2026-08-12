#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fuzz_time=${FUZZ_TIME:-1s}
list_file="$repo_root/.tmp/fuzz-targets.txt"

mkdir -p "$repo_root/.cache" "$repo_root/.tmp"
: > "$list_file"
find "$repo_root" -type f -name '*_test.go' \
	-not -path '*/.cache/*' -not -path '*/.tools/*' -not -path '*/vendor/*' |
	while IFS= read -r file; do
		sed -nE 's/^func (Fuzz[[:alnum:]_]+)\(f \*testing\.F\).*/\1/p' "$file" |
			while IFS= read -r target; do
				printf '%s\t%s\n' "$(dirname -- "$file")" "$target"
			done
	done | LC_ALL=C sort -u > "$list_file"

if [ ! -s "$list_file" ]; then
	echo "fuzz-smoke: no fuzz targets found" >&2
	exit 1
fi

count=0
while IFS="	" read -r directory target; do
	if [ "$directory" = "$repo_root" ]; then
		package=.
	else
		package="./${directory#"$repo_root"/}"
	fi
	echo "fuzz-smoke: $package $target"
	GOCACHE="$repo_root/.cache" GOTMPDIR="$repo_root/.tmp" TMPDIR="$repo_root/.tmp" \
		CGO_ENABLED=0 go test "$package" -run='^$' -fuzz="^${target}$" -fuzztime="$fuzz_time"
	count=$((count + 1))
done < "$list_file"

echo "fuzz-smoke: exercised $count target(s)"
