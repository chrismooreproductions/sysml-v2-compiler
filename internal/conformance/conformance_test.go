// SPDX-License-Identifier: EPL-2.0

package conformance

import (
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
