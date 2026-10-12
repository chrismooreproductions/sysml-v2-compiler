#!/bin/sh
# Copies upstream sources into this repository at the tags pinned in
# UPSTREAM, unmodified:
#   - from a SysML-v2-Release checkout: the standard library sources and the
#     example, training and validation models (.sysml and .kerml files only);
#   - from a SysML-v2-Pilot-Implementation checkout: the three Xtext grammars,
#     the SysML.ecore metamodel, and ImplicitGeneralizationMap.java (the table
#     of implicit supertypes, kept as .java.txt so Go tools ignore it).
#
#   tools/vendor-upstream.sh /path/to/SysML-v2-Release /path/to/SysML-v2-Pilot-Implementation
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
. "$root/UPSTREAM"
src=${1:?usage: tools/vendor-upstream.sh <SysML-v2-Release checkout> <Pilot-Implementation checkout>}
pilot=${2:?usage: tools/vendor-upstream.sh <SysML-v2-Release checkout> <Pilot-Implementation checkout>}

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

git -C "$pilot" fetch --depth 1 origin tag "$PILOT_TAG" >/dev/null 2>&1 || true
pcommit=$(git -C "$pilot" rev-parse "$PILOT_TAG^{commit}")
if [ "$pcommit" != "$PILOT_COMMIT" ]; then
	echo "tag $PILOT_TAG is $pcommit in $pilot, but UPSTREAM pins $PILOT_COMMIT" >&2
	exit 1
fi
gdest="$root/internal/grammar/xtext"
rm -rf "$gdest"
mkdir -p "$gdest"
for f in \
	org.omg.kerml.expressions.xtext/src/org/omg/kerml/expressions/xtext/KerMLExpressions.xtext \
	org.omg.kerml.xtext/src/org/omg/kerml/xtext/KerML.xtext \
	org.omg.sysml.xtext/src/org/omg/sysml/xtext/SysML.xtext; do
	git -C "$pilot" show "$PILOT_TAG:$f" >"$gdest/$(basename "$f")"
done
mkdir -p "$root/internal/schema"
git -C "$pilot" show "$PILOT_TAG:org.omg.sysml/model/SysML.ecore" >"$root/internal/schema/SysML.ecore"
mkdir -p "$root/internal/semantics/upstream"
git -C "$pilot" show "$PILOT_TAG:org.omg.sysml.logic/src/main/java/org/omg/sysml/util/ImplicitGeneralizationMap.java" \
	>"$root/internal/semantics/upstream/ImplicitGeneralizationMap.java.txt"
echo "vendored pilot grammars, metamodel and implicit-supertype table $PILOT_TAG ($pcommit)"
