#!/bin/sh
# Downloads the release's XMI oracles (sysml.library.xmi, about 25 MB, and
# sysml.library.xmi.implied, about 120 MB) at the tag pinned in UPSTREAM into
# .oracle/ and prints the directory. They are never committed.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
. "$root/UPSTREAM"
dest=${1:-$root/.oracle}
dir="$dest/$RELEASE_TAG"

if [ ! -d "$dir/sysml.library.xmi" ]; then
	rm -rf "$dir"
	mkdir -p "$dest"
	git clone --quiet --depth 1 --branch "$RELEASE_TAG" "$RELEASE_REPO" "$dir"
fi
commit=$(git -C "$dir" rev-parse HEAD)
if [ "$commit" != "$RELEASE_COMMIT" ]; then
	echo "$dir is at $commit, but UPSTREAM pins $RELEASE_COMMIT" >&2
	exit 1
fi
echo "$dir"
