package main

// Tier 1 of the semantic layer: the parser post-processors in the pilot
// implementation (org.omg.kerml.xtext/.../postprocessing and
// org.omg.sysml.xtext/.../postprocessing). They run on each element right
// after parsing, parents before children, and fill in what the grammar
// leaves implicit.

func clone(n *Node) *Node {
	c := &Node{Class: n.Class, Props: make([]Prop, len(n.Props))}
	for i, p := range n.Props {
		if ch, ok := p.V.(*Node); ok {
			p.V = clone(ch)
		}
		c.Props[i] = p
	}
	return c
}

func (n *Node) has(name string) bool {
	for _, p := range n.Props {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (n *Node) str(name string) string {
	for _, p := range n.Props {
		switch v := p.V.(type) {
		case Str:
			if p.Name == name {
				return string(v)
			}
		case Bool:
			if p.Name == name && bool(v) {
				return "true"
			}
		}
	}
	return ""
}

func (n *Node) set(name string, v Val) {
	for i, p := range n.Props {
		if p.Name == name {
			n.Props[i].V = v
			return
		}
	}
	n.Props = append(n.Props, Prop{name, "=", v})
}

func (n *Node) owned() []*Node {
	var out []*Node
	for _, p := range n.Props {
		if ch, ok := p.V.(*Node); ok && p.Name == "ownedRelatedElement" {
			out = append(out, ch)
		}
	}
	return out
}

func isA(n *Node, class string) bool { return n != nil && theMeta.isSub(n.Class, class) }

// ctx locates a node in the ownership tree. rel is the relationship that
// holds an element; owner is the element that holds a relationship;
// ownerRel is the relationship that holds that owner.
type ctx struct{ rel, owner, ownerRel *Node }

func postProcess(root *Node) *Node {
	r := clone(root)
	ppTree(r, ctx{})
	return r
}

func ppTree(n *Node, c ctx) {
	ppNode(n, c)
	for _, p := range n.Props {
		ch, ok := p.V.(*Node)
		if !ok {
			continue
		}
		switch p.Name {
		case "ownedRelationship":
			ppTree(ch, ctx{owner: n, ownerRel: c.rel})
		case "ownedRelatedElement":
			ppTree(ch, ctx{rel: n, owner: c.owner, ownerRel: c.ownerRel})
		default:
			ppTree(ch, ctx{owner: n, ownerRel: c.rel})
		}
	}
}

// ref records a cross-reference to an element the node owns. References to
// the owner itself are inverse ends of containment and are not recorded.
func ref(n *Node, role string, target *Node) {
	role = theMeta.effectiveRole(n.Class, role)
	if target != nil && !n.has(role) {
		n.Props = append(n.Props, Prop{role, "=", Ref(role)})
	}
}

func first(l []*Node) *Node {
	if len(l) == 0 {
		return nil
	}
	return l[0]
}

func last(l []*Node) *Node {
	if len(l) == 0 {
		return nil
	}
	return l[len(l)-1]
}

// sourceOrFirst is the pattern "owning element if it is an X, otherwise the
// first owned element (a feature chain)": only the latter is a reference.
func sourceOrFirst(n *Node, c ctx, role, ownerClass string) {
	// getOwningRelatedElement(): a relationship held inside another
	// relationship (an OwningMembership) is owned by that relationship,
	// not by the element the outer one belongs to.
	owner := c.owner
	if c.rel != nil {
		owner = c.rel
	}
	if !n.has(theMeta.effectiveRole(n.Class, role)) && !isA(owner, ownerClass) {
		ref(n, role, first(n.owned()))
	}
}

func ppNode(n *Node, c ctx) {
	// A cross-reference the grammar writes under a redefined feature's name
	// (specific, general, ...) is held under the feature that redefines it.
	for i, pr := range n.Props {
		if _, isRef := pr.V.(Ref); isRef {
			n.Props[i].Name = theMeta.effectiveRole(n.Class, pr.Name)
		}
	}
	owned := n.owned()
	is := func(class string) bool { return isA(n, class) }

	// Relationship targets that the grammar left to a feature chain.
	if is("Specialization") {
		ref(n, "general", last(owned))
		sourceOrFirst(n, c, "specific", "Type")
	}
	if is("Subsetting") {
		ref(n, "subsettedFeature", last(owned))
		sourceOrFirst(n, c, "subsettingFeature", "Feature")
	}
	if is("Redefinition") {
		sourceOrFirst(n, c, "redefiningFeature", "Feature")
		ref(n, "redefinedFeature", last(owned))
	}
	if is("ReferenceSubsetting") {
		ref(n, "referencedFeature", last(owned))
	}
	if is("FeatureTyping") {
		ref(n, "type", first(owned))
	}
	if is("Conjugation") {
		sourceOrFirst(n, c, "conjugatedType", "Type")
		ref(n, "originalType", last(owned))
	}
	if is("Disjoining") {
		ref(n, "disjoiningType", last(owned))
		sourceOrFirst(n, c, "typeDisjoined", "Type")
	}
	if is("Differencing") {
		ref(n, "differencingType", last(owned))
	}
	if is("Intersecting") {
		ref(n, "intersectingType", last(owned))
	}
	if is("Unioning") {
		ref(n, "unioningType", last(owned))
	}
	if is("FeatureInverting") {
		sourceOrFirst(n, c, "featureInverted", "Feature")
		ref(n, "invertingFeature", last(owned))
	}
	// importedNamespace / annotatedElement are target ends, so a reference
	// to the owning element is a real reference and is recorded.
	if is("NamespaceImport") && !n.has("importedNamespace") {
		if isA(first(owned), "Namespace") {
			ref(n, "importedNamespace", first(owned))
		} else {
			ref(n, "importedNamespace", c.owner)
		}
	}
	if is("Annotation") && !n.has("annotatedElement") && !isA(c.owner, "AnnotatingElement") {
		ref(n, "annotatedElement", c.owner)
	}
	if is("Dependency") {
		for range owned {
			n.Props = append(n.Props, Prop{"supplier", "+=", Ref("supplier")})
		}
	}
	// An import inside a filter package is public.
	if is("Import") && c.owner != nil && isA(c.ownerRel, "NamespaceImport") {
		n.set("visibility", Str("public"))
	}

	// Properties of the element a membership holds.
	if member := first(owned); member != nil {
		if is("EndFeatureMembership") && isA(member, "Feature") {
			member.set("isEnd", Bool(true))
		}
		if is("ParameterMembership") && isA(member, "Feature") {
			if is("ReturnParameterMembership") {
				member.set("direction", Str("out"))
			} else {
				member.set("direction", Str("in"))
			}
		}
	}

	// Feature and usage defaults.
	if is("Feature") && !is("Usage") && n.str("isConstant") == "true" {
		n.set("isVariable", Bool(true))
	}
	// An enumeration definition is always a variation (a Java override).
	if is("Definition") && (n.str("isVariation") == "true" || is("EnumerationDefinition")) {
		n.set("isAbstract", Bool(true))
	}
	if is("Usage") {
		ppUsage(n, c)
	}
}

// nonCompositeUsages are the usage kinds that are not composite by default.
var nonCompositeUsages = []string{"AttributeUsage", "BindingConnectorAsUsage", "EventOccurrenceUsage",
	"ExhibitStateUsage", "IncludeUseCaseUsage", "PerformActionUsage", "ReferenceUsage", "SuccessionAsUsage"}

// expectedFeaturingType mirrors UsageUtil.getExpectedFeaturingTypeOf: the
// type owning the feature membership that holds the usage.
func expectedFeaturingType(c ctx) *Node {
	if isA(c.rel, "FeatureMembership") {
		return c.owner
	}
	return nil
}

func ppUsage(n *Node, c ctx) {
	// isComposite defaults from the usage kind unless the source said
	// either `ref` (isReference) or isComposite explicitly.
	if !n.has("isComposite") && !n.has("isReference") {
		composite := true
		for _, k := range nonCompositeUsages {
			if isA(n, k) {
				composite = false
			}
		}
		if composite {
			n.set("isComposite", Bool(true))
		}
	}
	if n.str("isVariation") == "true" {
		n.set("isAbstract", Bool(true))
	}
	ft := expectedFeaturingType(c)
	if n.has("direction") || n.str("isEnd") == "true" || ft == nil || isA(ft, "AttributeDefinition") || isA(ft, "AttributeUsage") {
		n.set("isComposite", Bool(false))
	}
	if isA(n, "OccurrenceUsage") && n.has("portionKind") {
		n.set("isPortion", Bool(true))
	}
	if isA(n, "PortUsage") && !(isA(ft, "PortDefinition") || isA(ft, "PortUsage")) {
		n.set("isComposite", Bool(false))
	}
}
