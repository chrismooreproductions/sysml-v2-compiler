// SPDX-License-Identifier: EPL-2.0

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// invoke runs the command with the given arguments and returns its exit status
// and what it printed.
func invoke(args ...string) (status int, stdout, stderr string) {
	var out, errb bytes.Buffer
	status = run(args, &out, &errb)
	return status, out.String(), errb.String()
}

func write(t *testing.T, dir, name, text string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckAcceptsAGoodModel(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "ok.sysml", "package P { part def A; part a : A; attribute n : ScalarValues::Integer; }")
	status, out, errs := invoke("check", f)
	if status != 0 || out != "" || errs != "" {
		t.Errorf("status %d, stdout %q, stderr %q", status, out, errs)
	}
}

func TestCheckReportsUnresolvedNames(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "fleet.sysml", "package Fleet {\n  part def Truck;\n  part t : Trck;\n}\n")
	status, _, errs := invoke("check", f)
	want := f + ":3:12: cannot resolve \"Trck\" (searched from Fleet)\n"
	if status != 1 || errs != want {
		t.Errorf("status %d, stderr %q, want %q", status, errs, want)
	}
}

func TestCheckReportsSyntaxErrorsInEveryFileAndKeepsGoing(t *testing.T) {
	dir := t.TempDir()
	bad1 := write(t, dir, "a.sysml", "package P {\n  part x : ;\n}")
	good := write(t, dir, "b.sysml", "package Q { part def A; }")
	bad2 := write(t, dir, "c.sysml", "package {{")
	status, _, errs := invoke("check", bad1, good, bad2)
	lines := strings.Split(strings.TrimSpace(errs), "\n")
	if status != 1 || len(lines) != 2 || !strings.HasPrefix(lines[0], bad1+":2:12: unexpected \";\"") || !strings.HasPrefix(lines[1], bad2+":1:") {
		t.Errorf("status %d, stderr:\n%s", status, errs)
	}
}

func TestCheckWithoutTheLibrary(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "m.sysml", "package P { attribute n : ScalarValues::Integer; }")
	if status, _, errs := invoke("check", "-nolib", f); status != 1 || !strings.Contains(errs, `cannot resolve "ScalarValues::Integer"`) {
		t.Errorf("-nolib: status %d, stderr %q", status, errs)
	}
	if status, _, _ := invoke("check", f); status != 0 {
		t.Errorf("with the library: status %d", status)
	}
}

func TestDirectoriesAreSearchedForModels(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "one.sysml", "package One { part def A; }")
	write(t, dir, "sub/two.kerml", "classifier K;")
	write(t, dir, "sub/deeper/three.sysml", "package Three { private import One::*; part a : A; }")
	write(t, dir, "notes.txt", "this is not a model {{{")
	status, out, errs := invoke("resolve", dir)
	if status != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	if !strings.Contains(out, "three.sysml:1:") || !strings.Contains(out, "A -> One::A (PartDefinition, ") {
		t.Errorf("resolve output:\n%s", out)
	}
}

func TestResolvePrintsTargetsAndUnresolvedNames(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "fleet.sysml", "package Fleet {\n  part def Truck;\n  part t : Trck;\n  attribute w : ScalarValues::Real;\n}\n")
	status, out, _ := invoke("resolve", f)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if status != 1 || len(lines) != 2 {
		t.Fatalf("status %d, output:\n%s", status, out)
	}
	if lines[0] != f+":3:12: Trck -> ?" {
		t.Errorf("first line %q", lines[0])
	}
	want := f + ":4:17: ScalarValues::Real -> ScalarValues::Real (DataType, library:Kernel Libraries/Kernel Data Type Library/ScalarValues.kerml)"
	if lines[1] != want {
		t.Errorf("second line %q, want %q", lines[1], want)
	}
}

func TestDumpPrintsTheTree(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "m.sysml", "package P { part x : A; }")
	status, out, _ := invoke("dump", f)
	want := `Namespace
  ownedRelationship OwningMembership
    ownedRelatedElement Package declaredName="P"
      ownedRelationship OwningMembership
        ownedRelatedElement PartUsage declaredName="x" isComposite=false
          ownedRelationship FeatureTyping
            type -> @A
`
	if status != 0 || out != want {
		t.Errorf("status %d, output:\n%s", status, out)
	}
	_, spans, _ := invoke("dump", "-spans", f)
	if !strings.Contains(spans, `Package declaredName="P"  [1:1-1:26]`) || !strings.Contains(spans, "FeatureTyping  [1:22-1:23]") {
		t.Errorf("-spans output:\n%s", spans)
	}
}

func TestDumpOfSeveralFilesNamesEach(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.sysml", "package A;")
	b := write(t, dir, "b.kerml", "classifier B;")
	_, out, _ := invoke("dump", a, b)
	if !strings.Contains(out, "# "+a+"\n") || !strings.Contains(out, "\n# "+b+"\n") || !strings.Contains(out, "Classifier") {
		t.Errorf("output:\n%s", out)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"frobnicate"},
		{"check"},
		{"check", "/no/such/file.sysml"},
		{"dump"},
		{"resolve", "-bogus", "x"},
	}
	for _, args := range cases {
		if status, _, errs := invoke(args...); status != 2 || errs == "" {
			t.Errorf("%v: status %d, stderr %q", args, status, errs)
		}
	}
	if status, out, _ := invoke("help"); status != 0 || !strings.Contains(out, "usage: sysmlc") {
		t.Errorf("help: status %d, stdout %q", status, out)
	}
	if status, out, _ := invoke("version"); status != 0 || !strings.HasPrefix(out, "sysmlc ") {
		t.Errorf("version: status %d, stdout %q", status, out)
	}
}

func TestImpliedPrintsWhatTheSpecificationAdds(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "fleet.sysml", "package Fleet { part def Truck; part truck : Truck; }")
	status, out, errs := invoke("implied", f)
	want := f + ":1:17: Fleet::Truck Subclassification -> Parts::Part\n" +
		f + ":1:33: Fleet::truck Subsetting -> Parts::parts\n"
	if status != 0 || errs != "" || out != want {
		t.Errorf("status %d, stderr %q, stdout:\n%s\nwant:\n%s", status, errs, out, want)
	}
}

func TestCheckReportsConstraintViolations(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "v.sysml", "package P { part def V { variant part x; } }")
	status, _, errs := invoke("check", f)
	want := f + ":1:26: A variant must be an owned member of a variation.\n"
	if status != 1 || errs != want {
		t.Errorf("status %d, stderr %q, want %q", status, errs, want)
	}
}

func TestCheckReportsEverySyntaxErrorInAFile(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "m.sysml", "package P {\n  part x : ;\n  part def A { attribute a : ; part ok; }\n  part y @@ ;\n  part z;\n}\n")
	status, _, errs := invoke("check", f)
	lines := strings.Split(strings.TrimSpace(errs), "\n")
	if status != 1 || len(lines) != 3 ||
		!strings.HasPrefix(lines[0], f+":2:12:") || !strings.HasPrefix(lines[1], f+":3:30:") || !strings.HasPrefix(lines[2], f+":4:10:") {
		t.Errorf("status %d, stderr:\n%s", status, errs)
	}
}

func TestDumpImpliedShowsWhatTheSpecificationAdds(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "m.sysml", "package P { part def A; part a : A; }")
	status, out, errs := invoke("dump", "-implied", f)
	if status != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	for _, want := range []string{"Subclassification* isImplied=true\n", "superclassifier -> @Parts::Part\n", "Subsetting* isImplied=true\n", "subsettedFeature -> @Parts::parts\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	_, plain, _ := invoke("dump", f)
	if strings.Contains(plain, "isImplied") {
		t.Errorf("plain dump shows implied relationships:\n%s", plain)
	}
}

func TestEvalPrintsValuesAndSaysWhyItCannotComputeTheRest(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "m.sysml", "package P {\n  attribute a = 4500;\n  attribute b = a * 2 + 1;\n  attribute c = RealFunctions::sqrt(4.0);\n  attribute d;\n  attribute e = d + 1;\n}\n")
	status, out, errs := invoke("eval", f)
	want := f + ":2:3: P::a = 4500\n" + f + ":3:3: P::b = 9001\n" +
		f + ":4:3: P::c = not evaluable: cannot evaluate a call of RealFunctions::sqrt: it has no body (at 4:17)\n" +
		f + ":6:3: P::e = not determined: P::d has no value (at 6:17)\n"
	if status != 0 || errs != "" || out != want {
		t.Errorf("status %d, stderr %q, stdout:\n%s\nwant:\n%s", status, errs, out, want)
	}
}

func TestLSPServesOverStandardInput(t *testing.T) {
	frame := func(body string) string { return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body) }
	input := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":null,"capabilities":{}}}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)
	old := stdin
	stdin = func() io.Reader { return strings.NewReader(input) }
	defer func() { stdin = old }()
	status, out, errs := invoke("lsp")
	if status != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q", status, errs)
	}
	for _, want := range []string{`"hoverProvider":true`, `"definitionProvider":true`, `"id":2`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %s:\n%s", want, out)
		}
	}
}

func TestLSPServesOverTCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback:", err)
	}
	done := make(chan int, 1)
	go func() { done <- serveTCP(l, func(string, ...any) {}) }()
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":null,"capabilities":{}}}`
	fmt.Fprintf(conn, "Content-Length: %d\r\n\r\n%s", len(body), body)
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	// the reply is framed: headers, a blank line, then Content-Length bytes,
	// which may arrive in separate reads
	r := bufio.NewReader(conn)
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("reading the reply's headers: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			length, _ = strconv.Atoi(v)
		}
	}
	reply := make([]byte, max(length, 0))
	if _, err := io.ReadFull(r, reply); err != nil || !strings.Contains(string(reply), `"hoverProvider":true`) {
		t.Errorf("reply %q, %v", reply, err)
	}
	l.Close()
	if st := <-done; st != 0 {
		t.Errorf("serveTCP returned %d", st)
	}
}

func TestFactsPrintsTheResolvedFactsOfEachElement(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "fleet.sysml", "package Fleet { part def Truck { in attribute load : ScalarValues::Integer [1..2]; } }")
	status, out, errs := invoke("facts", f)
	want := f + ":1:1: Package Fleet\n" +
		"  members: owning Fleet::Truck\n" +
		f + ":1:17: PartDefinition Fleet::Truck\n" +
		"  specializes: Parts::Part (implied)\n" +
		"  members: feature Fleet::Truck::load\n" +
		f + ":1:34: AttributeUsage Fleet::Truck::load\n" +
		"  is: in reference\n" +
		"  multiplicity: 1..2\n" +
		"  types: ScalarValues::Integer\n" +
		"  subsets: Base::dataValues (implied)\n"
	if status != 0 || errs != "" || out != want {
		t.Errorf("status %d, stderr %q, stdout:\n%s\nwant:\n%s", status, errs, out, want)
	}
}

func TestEvalReportsRequirementsConstraintsAndCalculations(t *testing.T) {
	dir := t.TempDir()
	f := write(t, dir, "m.sysml", `package P {
  requirement def MassReq {
    attribute massActual : ScalarValues::Integer;
    attribute massLimit : ScalarValues::Integer = 2000;
    require constraint { massActual <= massLimit }
  }
  requirement ok : MassReq { attribute :>> massActual = 1800; }
  requirement over : MassReq { attribute :>> massActual = 2500; }
  requirement unknown : MassReq;
  calc def Half { in attribute x : ScalarValues::Integer; x / 2 }
  calc h : Half { in attribute :>> x = 8; }
}
`)
	status, out, errs := invoke("eval", f)
	want := f + ":4:5: P::MassReq::massLimit = 2000\n" +
		f + ":7:3: P::ok require constraint at 5:13 = true\n" +
		f + ":8:3: P::over require constraint at 5:13 = false\n" +
		f + ":9:3: P::unknown require constraint at 5:13 = not determined: P::MassReq::massActual has no value (at 5:26)\n" +
		f + ":11:3: P::h = 4\n"
	if status != 0 || errs != "" || out != want {
		t.Errorf("status %d, stderr %q, stdout:\n%s\nwant:\n%s", status, errs, out, want)
	}
}
