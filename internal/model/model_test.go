// SPDX-License-Identifier: EPL-2.0

package model

import (
	"testing"

	"github.com/chrismooreproductions/sysml-v2-compiler/internal/grammar"
	"github.com/chrismooreproductions/sysml-v2-compiler/internal/syntax"
)

func parse(t *testing.T, src string) *Element {
	t.Helper()
	n, err := syntax.Parse(grammar.SysML, "t", src)
	if err != nil {
		t.Fatal(err)
	}
	return FromSyntax(n)
}

func TestFromSyntaxKeepsStructureSpansAndOwners(t *testing.T) {
	src := "package P { part x : A; }"
	root := parse(t, src)
	if got := Dump(root); got != `Namespace(ownedRelationship+=OwningMembership(ownedRelatedElement+=Package(declaredName="P" ownedRelationship+=OwningMembership(ownedRelatedElement+=PartUsage(declaredName="x" ownedRelationship+=FeatureTyping(type=@A))))))` {
		t.Errorf("Dump = %s", got)
	}
	var part *Element
	root.Walk(func(e *Element) {
		if e.Class == "PartUsage" {
			part = e
		}
	})
	if part == nil {
		t.Fatal("no PartUsage")
	}
	if got := src[part.Span.Start.Offset:part.Span.End.Offset]; got != "part x : A;" {
		t.Errorf("span = %q", got)
	}
	// owners: PartUsage <- OwningMembership <- Package <- OwningMembership <- Namespace
	chain := ""
	for e := part; e != nil; e = e.Owner {
		chain += e.Class + " "
	}
	if chain != "PartUsage OwningMembership Package OwningMembership Namespace " {
		t.Errorf("owner chain = %s", chain)
	}
	if root.Owner != nil {
		t.Error("the root has an owner")
	}
}

func TestAccessors(t *testing.T) {
	root := parse(t, "abstract part def A { in attribute x; }")
	var def, attr *Element
	root.Walk(func(e *Element) {
		switch e.Class {
		case "PartDefinition":
			def = e
		case "AttributeUsage":
			attr = e
		}
	})
	if !def.IsA("Definition") || !def.IsA("Namespace") || def.IsA("Usage") {
		t.Error("IsA is wrong for PartDefinition")
	}
	if !def.IsTrue("isAbstract") || def.Text("declaredName") != "A" || def.Text("nothing") != "" {
		t.Errorf("Text/IsTrue: abstract=%v name=%q", def.IsTrue("isAbstract"), def.Text("declaredName"))
	}
	if attr.Text("direction") != "in" || !attr.Has("direction") || attr.Has("isEnd") {
		t.Error("direction is wrong")
	}
	if len(def.Children("ownedRelationship")) != 1 || def.Child("ownedRelationship") == nil || def.Child("declaredName") != nil {
		t.Error("Children/Child are wrong")
	}
}

func TestAddSetAndOwnership(t *testing.T) {
	e := &Element{Class: "Package"}
	child := &Element{Class: "OwningMembership", Generated: true}
	e.Add("ownedRelationship", "+=", child)
	if child.Owner != e {
		t.Error("Add did not set the owner")
	}
	e.Set("declaredName", Text("P"))
	e.Set("declaredName", Text("Q"))
	if e.Text("declaredName") != "Q" || len(e.Props) != 2 {
		t.Errorf("Set: %s", Dump(e))
	}
	e.Set("isAbstract", Bool(true))
	if !e.IsTrue("isAbstract") {
		t.Error("Bool not stored")
	}
	if got := Dump(e); got != `Package(ownedRelationship+=OwningMembership*() declaredName="Q" isAbstract=true)` {
		t.Errorf("Dump = %s", got)
	}
}

func TestRefs(t *testing.T) {
	root := parse(t, "part x : 'Q R'::A;")
	var typing *Element
	root.Walk(func(e *Element) {
		if e.Class == "FeatureTyping" {
			typing = e
		}
	})
	refs := typing.Refs("type")
	if len(refs) != 1 || refs[0].Name != "'Q R'::A" || refs[0].Target != nil {
		t.Errorf("refs = %+v", refs)
	}
	owner := &Element{Class: "Package"}
	refs[0].Target = owner
	refs[0].Name = ""
	if got := Dump(typing); got != "FeatureTyping(type=->Package)" {
		t.Errorf("Dump = %s", got)
	}
}
