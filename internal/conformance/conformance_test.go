// SPDX-License-Identifier: EPL-2.0

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oracleRoot finds the downloaded XMI oracle: $ORACLE_DIR, or the directory
// tools/fetch-oracle.sh uses. The test is skipped when there is none, so unit
// tests run offline.
func oracleRoot(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("ORACLE_DIR")
	if dir == "" {
		upstream, err := os.ReadFile("../../UPSTREAM")
		if err != nil {
			t.Fatal(err)
		}
		tag := ""
		for _, line := range strings.Split(string(upstream), "\n") {
			if strings.HasPrefix(line, "RELEASE_TAG=") {
				tag = strings.TrimPrefix(line, "RELEASE_TAG=")
			}
		}
		dir = filepath.Join("../../.oracle", tag)
	}
	if _, err := os.Stat(filepath.Join(dir, "sysml.library.xmi")); err != nil {
		t.Skipf("no oracle at %s: run tools/fetch-oracle.sh (or set ORACLE_DIR)", dir)
	}
	return dir
}

// The ratchet: structural conformance with the pilot's serialization of the
// standard library may get better but must not get worse. Raise these as the
// number improves.
const (
	minExactFiles = 84
	maxDiffs      = 79
)

func TestStructureMatchesTheOracle(t *testing.T) {
	dir := oracleRoot(t)
	files, err := LibraryFiles("../library/sysml.library", filepath.Join(dir, "sysml.library.xmi"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 94 {
		t.Fatalf("%d library files, want 94", len(files))
	}
	results := CompareLibrary(files)
	n, exact, errs, diffs := Totals(results)
	t.Logf("%d files: %d match exactly, %d errors, %d differences\n%s", n, exact, errs, diffs, Report(results))
	if errs > 0 {
		t.Errorf("%d files could not be compared", errs)
	}
	if exact < minExactFiles {
		t.Errorf("%d files match exactly, fewer than the %d already achieved", exact, minExactFiles)
	}
	if diffs > maxDiffs {
		t.Errorf("%d differences, more than the %d already achieved", diffs, maxDiffs)
	}
}

// The ratchet for name resolution: every reference in the standard library is
// resolved and its target compared with the oracle's, by position.
const (
	minMatchingRefs  = 19063
	maxResolvedDiffs = 2
	minResolvedExact = 93
)

func TestResolvedLibraryMatchesTheOracle(t *testing.T) {
	dir := oracleRoot(t)
	res, err := CompareResolved(filepath.Join(dir, "sysml.library.xmi"))
	if err != nil {
		t.Fatal(err)
	}
	n, exact, errs, diffs := Totals(res.Files)
	st := res.Refs
	var sb strings.Builder
	sb.WriteString(Report(res.Files))
	sb.WriteString("\nreferences:\n")
	for i, k := range st.Kinds() {
		if i >= 12 {
			break
		}
		r := st.ByKind[k]
		sb.WriteString(fmt.Sprintf("  %-44s %6d total %6d match %4d wrong %4d unresolved\n", k, r.Total, r.Match, r.Wrong, r.Unresolved))
	}
	t.Logf("%d files: %d match exactly, %d errors, %d differences\n%d references: %d match, %d resolve elsewhere, %d unresolved, %d without an oracle counterpart\n%s",
		n, exact, errs, diffs, st.Total, st.Match, st.Wrong, st.Unresolved, st.NoOracle, sb.String())
	for i, s := range st.WrongSamples {
		if i >= 8 {
			break
		}
		t.Logf("wrong: %s", s)
	}
	for i, s := range st.UnresolvedSamples {
		if i >= 8 {
			break
		}
		t.Logf("unresolved: %s", s)
	}
	if errs > 0 {
		t.Errorf("%d files could not be compared", errs)
	}
	if exact < minResolvedExact || diffs > maxResolvedDiffs {
		t.Errorf("structure after resolution: %d exact files, %d differences", exact, diffs)
	}
	if st.Match < minMatchingRefs {
		t.Errorf("%d references match, fewer than the %d already achieved", st.Match, minMatchingRefs)
	}
}
