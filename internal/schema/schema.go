// SPDX-License-Identifier: EPL-2.0

// Package schema describes the KerML and SysML v2 metamodel: its metaclasses,
// their supertypes and features, and the enumerations. The tables are generated
// from the specification's own ecore model (SysML.ecore, vendored from the
// pilot implementation) and the functions answer the questions later stages
// ask of it: is one metaclass a kind of another, what is a property's type
// and default, which properties share storage because one redefines the
// other.
package schema

//go:generate go run ../../tools/genschema .

import (
	"sync"
)

// Class is a metaclass.
type Class struct {
	Name      string
	Abstract  bool
	Interface bool
	Supers    []string // direct supertypes, in declaration order
	Features  []Feature
}

// Feature is a property of a metaclass.
type Feature struct {
	Name string
	// Type is the name of the class, enumeration or datatype (EString, EBoolean, ...).
	Type      string
	Attribute bool // an attribute with a datatype, as opposed to a reference
	Lower     int
	Many      bool // the upper bound is not 1
	// Containment is set only where the ecore says so; most containment is
	// expressed by subsetting ownedRelationship and ownedRelatedElement.
	Containment bool
	Derived     bool
	Transient   bool
	Volatile    bool
	Union       bool
	Default     string   // the default literal, if the ecore gives one
	Opposite    string   // "Class.feature"
	Redefines   []string // "Class.feature" each redefined feature
	Subsets     []string // "Class.feature" each subsetted feature
}

// Enum is an enumeration.
type Enum struct {
	Name     string
	Literals []Literal
}

// Literal is an enumeration literal.
type Literal struct {
	Name    string
	Value   int
	Literal string
}

type index struct {
	classes map[string]*Class
	enums   map[string]*Enum
	// closure[c] is c and all its supertypes.
	closure map[string]map[string]bool
}

var (
	once sync.Once
	idx  index
)

func load() {
	once.Do(func() {
		idx.classes = map[string]*Class{}
		idx.enums = map[string]*Enum{}
		idx.closure = map[string]map[string]bool{}
		for i := range classes {
			idx.classes[classes[i].Name] = &classes[i]
		}
		for i := range enums {
			idx.enums[enums[i].Name] = &enums[i]
		}
		var walk func(name string, into map[string]bool)
		walk = func(name string, into map[string]bool) {
			if into[name] {
				return
			}
			into[name] = true
			if c := idx.classes[name]; c != nil {
				for _, s := range c.Supers {
					walk(s, into)
				}
			}
		}
		for name := range idx.classes {
			m := map[string]bool{}
			walk(name, m)
			idx.closure[name] = m
		}
	})
}

// Lookup returns a metaclass by name.
func Lookup(name string) (*Class, bool) {
	load()
	c, ok := idx.classes[name]
	return c, ok
}

// Classes returns every metaclass, in the order the ecore declares them.
func Classes() []Class {
	load()
	return classes
}

// LookupEnum returns an enumeration by name.
func LookupEnum(name string) (*Enum, bool) {
	load()
	e, ok := idx.enums[name]
	return e, ok
}

// IsEnum reports whether a type name is an enumeration.
func IsEnum(name string) bool {
	_, ok := LookupEnum(name)
	return ok
}

// HasLiteral reports whether an enumeration has a literal with the given name.
func (e *Enum) HasLiteral(name string) bool {
	for _, l := range e.Literals {
		if l.Name == name {
			return true
		}
	}
	return false
}

// IsKindOf reports whether class is target or one of its subtypes.
func IsKindOf(class, target string) bool {
	load()
	return idx.closure[class][target]
}

// Supertypes returns a class and all its supertypes, nearest first.
func Supertypes(class string) []string {
	if v, ok := supertypesMemo.Load(class); ok {
		return v.([]string)
	}
	out := computeSupertypes(class)
	supertypesMemo.Store(class, out)
	return out
}

var supertypesMemo sync.Map // class -> []string, which callers must not change

func computeSupertypes(class string) []string {
	load()
	seen := map[string]bool{}
	var out []string
	queue := []string{class}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
		if k := idx.classes[c]; k != nil {
			queue = append(queue, k.Supers...)
		}
	}
	return out
}

// FeatureOf returns the named feature of a class or the nearest supertype
// that declares it, and the class that does.
func FeatureOf(class, name string) (*Feature, string, bool) {
	k := featureKey{class, name}
	if v, ok := featureMemo.Load(k); ok {
		r := v.(featureResult)
		return r.f, r.from, r.ok
	}
	f, from, ok := computeFeatureOf(class, name)
	featureMemo.Store(k, featureResult{f, from, ok})
	return f, from, ok
}

type featureKey struct{ class, name string }
type featureResult struct {
	f    *Feature
	from string
	ok   bool
}

var featureMemo sync.Map

func computeFeatureOf(class, name string) (*Feature, string, bool) {
	load()
	for _, c := range Supertypes(class) {
		k := idx.classes[c]
		if k == nil {
			continue
		}
		for i := range k.Features {
			if k.Features[i].Name == name {
				return &k.Features[i], c, true
			}
		}
	}
	return nil, "", false
}

// EffectiveRole follows redefinitions. A redefined feature shares storage
// with the feature that redefines it, so a value for Specialization.general on
// a CrossSubsetting is held as crossedFeature.
func EffectiveRole(class, role string) string {
	load()
	for changed := true; changed; {
		changed = false
		ancestors := idx.closure[class]
		for _, c := range Supertypes(class) {
			k := idx.classes[c]
			if k == nil {
				continue
			}
			for i := range k.Features {
				for _, r := range k.Features[i].Redefines {
					if dot := indexDot(r); dot >= 0 && r[dot+1:] == role && ancestors[r[:dot]] {
						role, changed = k.Features[i].Name, true
					}
				}
			}
			if changed {
				break
			}
		}
	}
	return role
}

func indexDot(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

// IsDerived reports whether a feature of the class is stored under another
// name or computed: marked derived or transient in the ecore, or redefined by a
// feature of the class.
func IsDerived(class, name string) bool {
	f, _, ok := FeatureOf(class, name)
	if !ok {
		return false
	}
	return f.Derived || f.Transient || EffectiveRole(class, name) != name
}

// IsSourceEnd reports whether a relationship feature is the source end,
// directly or through redefinitions (specific, subsettingFeature,
// featureInverted, ...). A source-end reference to the owner is the inverse of
// containment; a target-end reference to the owner is a real reference.
func IsSourceEnd(class, role string) bool { return redefinesChain(class, role, "source") }

// IsTargetEnd reports whether a relationship feature is the target end,
// directly or through redefinitions (general, subsettedFeature, type, ...).
func IsTargetEnd(class, role string) bool {
	return redefinesChain(class, role, "target") || redefinesChain(class, role, "general")
}

type chainKey struct{ class, role, goal string }

var (
	chainMu   sync.RWMutex
	chainMemo = map[chainKey]bool{}
)

// redefinesChain is asked for every property of every relationship, many times
// over, so its memo is a plain map under a read lock: a sync.Map would box each
// key.
func redefinesChain(class, role, goal string) bool {
	k := chainKey{class, role, goal}
	chainMu.RLock()
	v, ok := chainMemo[k]
	chainMu.RUnlock()
	if ok {
		return v
	}
	v = computeRedefinesChain(class, role, goal)
	chainMu.Lock()
	chainMemo[k] = v
	chainMu.Unlock()
	return v
}

func computeRedefinesChain(class, role, goal string) bool {
	load()
	seen := map[string]bool{}
	var walk func(r string) bool
	walk = func(r string) bool {
		if r == goal {
			return true
		}
		if seen[r] {
			return false
		}
		seen[r] = true
		for _, c := range Supertypes(class) {
			k := idx.classes[c]
			if k == nil {
				continue
			}
			for i := range k.Features {
				if k.Features[i].Name != r {
					continue
				}
				for _, red := range k.Features[i].Redefines {
					if dot := indexDot(red); dot >= 0 && walk(red[dot+1:]) {
						return true
					}
				}
			}
		}
		return false
	}
	return walk(role)
}
