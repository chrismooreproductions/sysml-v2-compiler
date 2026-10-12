#!/bin/sh
# Copies the standard library sources and the release's example models from a
# checkout of SysML-v2-Release into this repository, at the tag pinned in
# UPSTREAM. Only .sysml and .kerml files are copied; nothing is modified.
#
#   tools/vendor-upstream.sh /path/to/SysML-v2-Release
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
. "$root/UPSTREAM"
src=${1:?usage: tools/vendor-upstream.sh /path/to/SysML-v2-Release}

git -C "$src" fetch --depth 1 origin tag "$RELEASE_TAG" >/dev/null 2>&1 || true
commit=$(git -C "$src" rev-parse "$RELEASE_TAG^{commit}")
if [ "$commit" != "$RELEASE_COMMIT" ]; then
	echo "tag $RELEASE_TAG is $commit in $src, but UPSTREAM pins $RELEASE_COMMIT" >&2
	exit 1
fi

copy() { # copy <upstream path> <destination under the repo>
	dest="$root/$2"
	rm -rf "$dest"
	mkdir -p "$dest"
	git -C "$src" archive "$RELEASE_TAG" "$1" | tar -x -C "$dest" --strip-components="$3"
	find "$dest" -type f ! -name '*.sysml' ! -name '*.kerml' -delete
	find "$dest" -type d -empty -delete
}

copy sysml.library internal/library/sysml.library 1
copy sysml/src/examples testdata/upstream/sysml/examples 3
copy sysml/src/training testdata/upstream/sysml/training 3
copy sysml/src/validation testdata/upstream/sysml/validation 3
copy kerml/src/examples testdata/upstream/kerml/examples 3

echo "vendored release $RELEASE_TAG ($commit)"
