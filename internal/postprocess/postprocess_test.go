// SPDX-License-Identifier: EPL-2.0

package postprocess

import (
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

func run(t *testing.T, lang grammar.Language, src string) *model.Element {
	t.Helper()
	n, err := syntax.Parse(lang, "t", src)
	if err != nil {
		t.Fatal(err)
	}
	root := model.FromSyntax(n)
	Run(root)
	return root
}

func find(root *model.Element, class string) []*model.Element {
	var out []*model.Element
	root.Walk(func(e *model.Element) {
		if e.Class == class {
			out = append(out, e)
		}
	})
	return out
}

func one(t *testing.T, root *model.Element, class string) *model.Element {
	t.Helper()
	got := find(root, class)
	if len(got) != 1 {
		t.Fatalf("%d %s elements in %s", len(got), class, model.Dump(root))
	}
	return got[0]
}

// A reference the post-processor sets directly has a target and no name.
func target(t *testing.T, e *model.Element, role string) *model.Element {
	t.Helper()
	refs := e.Refs(role)
	if len(refs) != 1 || refs[0].Target == nil {
		t.Fatalf("%s.%s = %+v in %s", e.Class, role, refs, model.Dump(e))
	}
	return refs[0].Target
}

func TestEndFeaturesAreEnds(t *testing.T) {
	root := run(t, grammar.SysML, "connection def C { end part a; end part b; part c; }")
	var ends []string
	for _, p := range find(root, "PartUsage") {
		if p.IsTrue("isEnd") {
			ends = append(ends, p.Text("declaredName"))
		}
	}
	if len(ends) != 2 || ends[0] != "a" || ends[1] != "b" {
		t.Errorf("end features = %v", ends)
	}
}

func TestParameterDirections(t *testing.T) {
	// the operand of an invocation is a ParameterMembership; return is a ReturnParameterMembership
	root := run(t, grammar.SysML, "calc def C { in x; return y = f(1, 2); }")
	var inv *model.Element
	for _, e := range find(root, "InvocationExpression") {
		inv = e
	}
	if inv == nil {
		t.Fatal("no invocation")
	}
	var dirs []string
	for _, m := range inv.Children("ownedRelationship") {
		if m.IsA("ParameterMembership") {
			dirs = append(dirs, m.Child("ownedRelatedElement").Text("direction"))
		}
	}
	if len(dirs) != 2 || dirs[0] != "in" || dirs[1] != "in" {
		t.Errorf("argument directions = %v", dirs)
	}
	ret := one(t, root, "ReturnParameterMembership").Child("ownedRelatedElement")
	if ret.Text("direction") != "out" {
		t.Errorf("return direction = %q", ret.Text("direction"))
	}
}

func TestFeatureChainTargetIsTheOwnedChain(t *testing.T) {
	root := run(t, grammar.SysML, "part def A { part x :> a.b.c; }")
	sub := one(t, root, "Subsetting")
	chain := target(t, sub, "subsettedFeature")
	if chain.Class != "Feature" || chain.Owner != sub || len(chain.Children("ownedRelationship")) != 3 {
		t.Errorf("subsetted feature = %s", model.Dump(chain))
	}
	// the subsetting feature is the part that owns the relationship, not the chain
	if got := target(t, sub, "subsettingFeature"); got != sub.Owner {
		t.Errorf("subsettingFeature = %s", model.Dump(got))
	}
}

func TestRedefinitionStoredUnderTheRedefiningFeature(t *testing.T) {
	root := run(t, grammar.SysML, "part def A { part x :>> y; }")
	red := one(t, root, "Redefinition")
	if got := red.Refs("redefinedFeature"); len(got) != 1 || got[0].Name != "y" {
		t.Errorf("redefinedFeature = %+v", got)
	}
	if red.Has("general") || red.Has("subsettedFeature") {
		t.Errorf("a redefined role was used: %s", model.Dump(red))
	}
	if got := target(t, red, "redefiningFeature"); got != red.Owner {
		t.Errorf("redefiningFeature is %s", model.Dump(got))
	}
}

func TestFeatureTypingKnowsItsFeature(t *testing.T) {
	root := run(t, grammar.SysML, "part x : A;")
	typing := one(t, root, "FeatureTyping")
	if got := target(t, typing, "typedFeature"); got.Class != "PartUsage" {
		t.Errorf("typedFeature = %s", got.Class)
	}
}

func TestSubclassificationKnowsItsClassifier(t *testing.T) {
	root := run(t, grammar.SysML, "part def A :> B;")
	sc := one(t, root, "Subclassification")
	if got := target(t, sc, "subclassifier"); got.Class != "PartDefinition" {
		t.Errorf("subclassifier = %s", got.Class)
	}
}

func TestIsCompositeDefaults(t *testing.T) {
	root := run(t, grammar.SysML, `package P { part def D { part p; attribute a; ref part r; item i; in part d; } part top; }`)
	want := map[string]string{"p": "true", "a": "", "r": "", "i": "true", "d": "false", "top": "false"}
	for _, class := range []string{"PartUsage", "AttributeUsage", "ItemUsage"} {
		for _, u := range find(root, class) {
			name := u.Text("declaredName")
			if got := u.Text("isComposite"); got != want[name] {
				t.Errorf("%s isComposite = %q, want %q", name, got, want[name])
			}
		}
	}
}

func TestVariationsAndEnumerationsAreAbstract(t *testing.T) {
	root := run(t, grammar.SysML, "variation part def V { } enum def E { a; }")
	if !one(t, root, "PartDefinition").IsTrue("isAbstract") {
		t.Error("a variation is not abstract")
	}
	if !one(t, root, "EnumerationDefinition").IsTrue("isAbstract") {
		t.Error("an enumeration definition is not abstract")
	}
}

func TestPortUsageIsNotCompositeOutsideAPort(t *testing.T) {
	root := run(t, grammar.SysML, "part def A { port p; } port def Q { port inner; }")
	for _, p := range find(root, "PortUsage") {
		want := p.Text("declaredName") == "inner"
		if p.IsTrue("isComposite") != want {
			t.Errorf("port %s composite = %v, want %v", p.Text("declaredName"), p.IsTrue("isComposite"), want)
		}
	}
}

func TestImportsInAFilterPackageArePublic(t *testing.T) {
	root := run(t, grammar.SysML, "package P { private import Q::*[@M]; }")
	var visibility []string
	root.Walk(func(e *model.Element) {
		if e.IsA("Import") {
			visibility = append(visibility, e.Text("visibility"))
		}
	})
	// the written import is private; the import the filter package wraps is public
	if len(visibility) != 2 || visibility[0] != "private" || visibility[1] != "public" {
		t.Errorf("import visibilities = %v", visibility)
	}
}

func TestNamespaceImportOfTheOwnerWhenNothingIsNamed(t *testing.T) {
	root := run(t, grammar.SysML, "package P { private import Q::*; }")
	imp := one(t, root, "NamespaceImport")
	if refs := imp.Refs("importedNamespace"); len(refs) != 1 || refs[0].Name != "Q" {
		t.Errorf("importedNamespace = %+v", refs)
	}
}

func TestKerMLConstantFeaturesAreVariable(t *testing.T) {
	root := run(t, grammar.KerML, "classifier C { const feature f; feature g; }")
	for _, f := range find(root, "Feature") {
		want := f.Text("declaredName") == "f"
		if f.IsTrue("isVariable") != want {
			t.Errorf("%s isVariable = %v, want %v", f.Text("declaredName"), f.IsTrue("isVariable"), want)
		}
	}
}
