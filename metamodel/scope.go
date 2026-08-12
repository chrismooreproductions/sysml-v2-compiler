package metamodel

// scope is a lexical scope used while translating an AST into a Model: it
// maps names visible at some point in the source to the ElementIDs they
// resolve to, chaining to an enclosing scope for names not declared
// locally.
type scope struct {
	parent  *scope
	symbols map[string]ElementID
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, symbols: make(map[string]ElementID)}
}

// define declares name in this scope, pointing at id. It reports false
// without changing anything if name is already declared in this exact
// scope; shadowing a name from a parent scope is fine and always succeeds.
func (s *scope) define(name string, id ElementID) bool {
	if _, exists := s.symbols[name]; exists {
		return false
	}
	s.symbols[name] = id
	return true
}

// resolve looks up name in this scope, then walks up through enclosing
// scopes until it's found or the chain is exhausted.
func (s *scope) resolve(name string) (ElementID, bool) {
	if id, ok := s.symbols[name]; ok {
		return id, true
	}
	if s.parent != nil {
		return s.parent.resolve(name)
	}
	return "", false
}
