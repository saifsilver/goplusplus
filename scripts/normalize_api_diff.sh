#!/usr/bin/env sh
set -eu

test "$#" -eq 1 || {
	echo "usage: normalize_api_diff.sh API_DIFF_FILE" >&2
	exit 2
}

# A semantic-version value change is release metadata, not a source-compatibility
# break. Other changes to Version (removal, type changes, or malformed values)
# remain in the stream and continue to fail the compatibility gate.
sed -E '/^- Version: value changed from "v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)" to "v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"$/d' "$1"
