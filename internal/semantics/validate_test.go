// SPDX-License-Identifier: EPL-2.0

package semantics

import (
	"strings"
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/library"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/model"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/postprocess"
)

// The standard library is valid: none of the checks reports anything in it.
func TestLibraryIsValid(t *testing.T) {
	files, err := library.Files()
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorkspace()
	var roots []*model.Element
	for _, f := range files {
		root := model.FromSyntax(f.Tree)
		postprocess.Run(root)
		Structure(root)
		w.Add(root)
		roots = append(roots, root)
	}
	w.ResolveAll()
	var got []string
	for _, is := range w.Validate(roots...) {
		got = append(got, is.Constraint+" "+is.Element.Class+" "+QualifiedName(is.Element)+" "+w.FileOf(is.Element))
	}
	if len(got) > 0 {
		t.Errorf("the library violates:\n%s", strings.Join(got, "\n"))
	}
}

func constraintsIn(t *testing.T, src string) []string {
	t.Helper()
	w, roots := resolved(t, true, src)
	var out []string
	for _, is := range w.Validate(roots...) {
		out = append(out, is.Constraint)
	}
	return out
}

func TestConstraintsAreReported(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"end feature with a direction":   {`package P { connection def C { end in a; end b; } }`, "validateFeatureEndNoDirection"},
		"constant outside an occurrence": {`package P { attribute def D { constant attribute a : ScalarValues::Integer; } }`, "validateFeatureConstantIsVariable"},
		"variant outside a variation":    {`package P { part def V { variant part x; } }`, "validateVariantMembershipOwningNamespace"},
	}
	for name, c := range cases {
		got := constraintsIn(t, c.src)
		found := false
		for _, g := range got {
			found = found || g == c.want
		}
		if !found {
			t.Errorf("%s: got %v, want %s", name, got, c.want)
		}
	}
}

// The grammar does not let some memberships appear in the wrong kind of type,
// so those constraints can only be broken by a model built another way. Build
// one: an owner, holding a membership, holding a member.
func TestMembershipOwningTypes(t *testing.T) {
	cases := []struct{ constraint, owner, membership, member string }{
		{"validateActorMembershipOwningType", "PartDefinition", "ActorMembership", "PartUsage"},
		{"validateSubjectMembershipOwningType", "PartDefinition", "SubjectMembership", "ReferenceUsage"},
		{"validateStakeholderMembershipOwningType", "PartDefinition", "StakeholderMembership", "PartUsage"},
		{"validateObjectiveMembershipOwningType", "RequirementDefinition", "ObjectiveMembership", "RequirementUsage"},
		{"validateTransitionFeatureMembershipOwningType", "ActionDefinition", "TransitionFeatureMembership", "ActionUsage"},
		{"validateStateSubactionMembershipOwningType", "PartDefinition", "StateSubactionMembership", "ActionUsage"},
		{"validateReturnParameterMembershipOwningType", "PartDefinition", "ReturnParameterMembership", "Feature"},
		{"validateControlNodeOwningType", "PartDefinition", "FeatureMembership", "ForkNode"},
	}
	good := map[string]string{
		"validateActorMembershipOwningType":             "RequirementDefinition",
		"validateSubjectMembershipOwningType":           "CaseDefinition",
		"validateStakeholderMembershipOwningType":       "RequirementUsage",
		"validateObjectiveMembershipOwningType":         "CaseDefinition",
		"validateTransitionFeatureMembershipOwningType": "TransitionUsage",
		"validateStateSubactionMembershipOwningType":    "StateDefinition",
		"validateReturnParameterMembershipOwningType":   "Function",
		"validateControlNodeOwningType":                 "ActionDefinition",
	}
	violates := func(owner, membership, member string) []string {
		member_ := &model.Element{Class: member}
		mem := &model.Element{Class: membership}
		mem.Add("ownedRelatedElement", "+=", member_)
		own := &model.Element{Class: owner}
		own.Add("ownedRelationship", "+=", mem)
		top := &model.Element{Class: "OwningMembership"}
		top.Add("ownedRelatedElement", "+=", own)
		root := &model.Element{Class: "Namespace"}
		root.Add("ownedRelationship", "+=", top)
		w := NewWorkspace()
		w.Add(root)
		var out []string
		for _, is := range w.Validate(root) {
			out = append(out, is.Constraint)
		}
		return out
	}
	has := func(l []string, s string) bool {
		for _, x := range l {
			if x == s {
				return true
			}
		}
		return false
	}
	for _, c := range cases {
		if got := violates(c.owner, c.membership, c.member); !has(got, c.constraint) {
			t.Errorf("%s in a %s: got %v, want %s", c.membership, c.owner, got, c.constraint)
		}
		if got := violates(good[c.constraint], c.membership, c.member); has(got, c.constraint) {
			t.Errorf("%s in a %s: wrongly reported %s", c.membership, good[c.constraint], c.constraint)
		}
	}
}

func TestValidModelsHaveNoIssues(t *testing.T) {
	src := `package P {
		part def Car { part wheels[4]; attribute n : ScalarValues::Integer; }
		part def V { variation part x { variant part a; } }
		requirement def R { subject s : Car; }
		state def S { entry action a; state s1; state s2; transition t first s1 then s2; }
		action def A { action x; fork f; }
	}`
	if got := constraintsIn(t, src); len(got) != 0 {
		t.Errorf("issues in a valid model: %v", got)
	}
}

func TestEveryCheckHasAMessage(t *testing.T) {
	for _, c := range structuralChecks {
		if messageFor(c.constraint) == "" {
			t.Errorf("%s has no message", c.constraint)
		}
	}
}
