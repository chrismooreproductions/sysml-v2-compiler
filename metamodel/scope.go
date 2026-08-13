package metamodel

// scope is a per-namespace guard against declaring the same name twice
// within it, used only during declaration. It deliberately has no notion
// of an enclosing/parent scope: looking up a name from the perspective of
// some position in the model, against that position's ancestors, is
// Model.Resolve's job, computed after the fact from the model's own Owner
// links rather than from scopes captured while walking the AST.
type scope struct {
	symbols map[string]ElementID
}

func newScope() *scope {
	return &scope{symbols: make(map[string]ElementID)}
}

// define declares name in this scope, pointing at id. It reports false
// without changing anything if name is already declared in this scope.
func (s *scope) define(name string, id ElementID) bool {
	if _, exists := s.symbols[name]; exists {
		return false
	}
	s.symbols[name] = id
	return true
}
