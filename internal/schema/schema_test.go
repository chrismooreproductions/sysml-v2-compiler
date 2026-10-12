// SPDX-License-Identifier: EPL-2.0

package schema

import (
	"bytes"
	"os"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/schema/gen"
)

// The checked-in tables must be exactly what the generator makes from the
// vendored ecore. When this fails, run `go generate ./internal/schema`.
func TestGeneratedTablesAreCurrent(t *testing.T) {
	ecore, err := os.ReadFile("SysML.ecore")
	if err != nil {
		t.Fatal(err)
	}
	want, err := gen.Generate(ecore)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("schema_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("schema_gen.go is out of date with SysML.ecore: run `go generate ./internal/schema`")
	}
}

// The counts pin the vendored release; a change prompts a look at what moved.
func TestCounts(t *testing.T) {
	if got := len(Classes()); got != 175 {
		t.Errorf("%d classes, want 175", got)
	}
	if got := len(enums); got != 7 {
		t.Errorf("%d enums, want 7", got)
	}
	features := 0
	for _, c := range Classes() {
		features += len(c.Features)
	}
	if features != 415 {
		t.Errorf("%d features, want 415", features)
	}
}

func TestEveryReferenceIsDefined(t *testing.T) {
	datatypes := map[string]bool{"EString": true, "EBoolean": true, "EInt": true, "EDouble": true, "ELong": true, "EBooleanObject": true, "EIntegerObject": true, "EDoubleObject": true}
	for _, c := range Classes() {
		for _, s := range c.Supers {
			if _, ok := Lookup(s); !ok {
				t.Errorf("%s: unknown supertype %s", c.Name, s)
			}
		}
		for _, f := range c.Features {
			if _, ok := Lookup(f.Type); !ok && !IsEnum(f.Type) && !datatypes[f.Type] {
				t.Errorf("%s.%s: unknown type %s", c.Name, f.Name, f.Type)
			}
			for _, r := range append(append([]string{}, f.Redefines...), f.Subsets...) {
				cls, name, ok := splitRef(r)
				if !ok {
					t.Errorf("%s.%s: malformed reference %q", c.Name, f.Name, r)
					continue
				}
				if _, _, found := FeatureOf(cls, name); !found {
					t.Errorf("%s.%s: %s does not exist", c.Name, f.Name, r)
				}
			}
		}
	}
}

func splitRef(r string) (class, name string, ok bool) {
	i := indexDot(r)
	if i < 0 {
		return "", "", false
	}
	return r[:i], r[i+1:], true
}

func TestKindOf(t *testing.T) {
	yes := [][2]string{{"PartUsage", "Usage"}, {"PartUsage", "Feature"}, {"PartUsage", "Namespace"}, {"PartUsage", "Element"},
		{"OperatorExpression", "InvocationExpression"}, {"FeatureChainExpression", "Expression"}, {"ReturnParameterMembership", "ParameterMembership"},
		{"Package", "Namespace"}, {"Import", "Relationship"}, {"Usage", "Usage"}}
	no := [][2]string{{"Usage", "PartUsage"}, {"Package", "Type"}, {"Membership", "Import"}, {"Nonexistent", "Element"}}
	for _, c := range yes {
		if !IsKindOf(c[0], c[1]) {
			t.Errorf("%s is not a kind of %s", c[0], c[1])
		}
	}
	for _, c := range no {
		if IsKindOf(c[0], c[1]) {
			t.Errorf("%s is a kind of %s", c[0], c[1])
		}
	}
}

func TestFeatureLookupAndDefaults(t *testing.T) {
	f, owner, ok := FeatureOf("MembershipImport", "visibility")
	if !ok || owner != "Import" || f.Default != "private" || f.Type != "VisibilityKind" || !IsEnum(f.Type) {
		t.Errorf("Import.visibility = %+v from %q", f, owner)
	}
	f, owner, _ = FeatureOf("OwningMembership", "visibility")
	if owner != "Membership" || f.Default != "public" {
		t.Errorf("Membership.visibility default = %q from %s", f.Default, owner)
	}
	if f, _, _ := FeatureOf("Usage", "isReference"); f == nil || !f.Derived {
		t.Error("Usage.isReference is not derived")
	}
	if f, _, _ := FeatureOf("Feature", "isUnique"); f == nil || f.Default != "true" {
		t.Error("Feature.isUnique default is not true")
	}
	if _, _, ok := FeatureOf("Package", "noSuchFeature"); ok {
		t.Error("found a feature that does not exist")
	}
}

func TestEnums(t *testing.T) {
	e, ok := LookupEnum("VisibilityKind")
	if !ok || !e.HasLiteral("public") || !e.HasLiteral("private") || !e.HasLiteral("protected") || e.HasLiteral("package") {
		t.Errorf("VisibilityKind = %+v", e)
	}
	e, _ = LookupEnum("FeatureDirectionKind")
	if !e.HasLiteral("in") || !e.HasLiteral("out") || !e.HasLiteral("inout") {
		t.Errorf("FeatureDirectionKind = %+v", e)
	}
}

// A redefined feature is stored under the feature that redefines it.
func TestEffectiveRole(t *testing.T) {
	cases := []struct{ class, role, want string }{
		{"Specialization", "general", "general"},
		{"Subsetting", "general", "subsettedFeature"},
		{"Redefinition", "general", "redefinedFeature"},
		{"CrossSubsetting", "general", "crossedFeature"},
		{"CrossSubsetting", "subsettedFeature", "crossedFeature"},
		{"ReferenceSubsetting", "subsettedFeature", "referencedFeature"},
		{"Redefinition", "specific", "redefiningFeature"},
		{"FeatureTyping", "ownedRelationship", "ownedRelationship"},
	}
	for _, c := range cases {
		if got := EffectiveRole(c.class, c.role); got != c.want {
			t.Errorf("EffectiveRole(%s, %s) = %s, want %s", c.class, c.role, got, c.want)
		}
	}
}

func TestSourceAndTargetEnds(t *testing.T) {
	src := [][2]string{{"Specialization", "specific"}, {"Subsetting", "subsettingFeature"}, {"Redefinition", "redefiningFeature"}, {"FeatureInverting", "featureInverted"}, {"Disjoining", "typeDisjoined"}}
	tgt := [][2]string{{"Specialization", "general"}, {"Subsetting", "subsettedFeature"}, {"FeatureTyping", "type"}, {"FeatureInverting", "invertingFeature"}, {"Redefinition", "redefinedFeature"}, {"Annotation", "annotatedElement"}}
	for _, c := range src {
		if !IsSourceEnd(c[0], c[1]) || IsTargetEnd(c[0], c[1]) {
			t.Errorf("%s.%s should be a source end only", c[0], c[1])
		}
	}
	for _, c := range tgt {
		if !IsTargetEnd(c[0], c[1]) || IsSourceEnd(c[0], c[1]) {
			t.Errorf("%s.%s should be a target end only", c[0], c[1])
		}
	}
}

func TestIsDerived(t *testing.T) {
	if !IsDerived("OperatorExpression", "operand") {
		t.Error("operand should be derived")
	}
	if !IsDerived("Subsetting", "general") {
		t.Error("general is redefined by subsettedFeature in a Subsetting")
	}
	if IsDerived("FeatureTyping", "type") || IsDerived("Package", "declaredName") {
		t.Error("type and declaredName are stored")
	}
}
