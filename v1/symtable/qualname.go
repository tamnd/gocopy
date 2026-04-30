package symtable

// computeQualname fills s.QualName based on its parent chain,
// matching CPython's compiler_set_qualname in Python/codegen.c.
//
// Rules:
//
//   - The module scope has an empty qualname. Its name is also empty
//     (CPython uses "<module>" as the name, but qualname is "").
//   - A top-level function (parent is module) has qualname == name.
//   - A function nested inside another function has qualname
//     parent.qualname + ".<locals>." + name.
//   - A function nested inside a class has qualname
//     parent.qualname + "." + name. Classes are deferred to v0.6.11
//     so the class branch panics if reached.
//
// computeQualname assumes s.Name is already set and walks up the
// parent chain at most once per scope; callers should invoke it in
// pre-order (parent before child) so each child sees a populated
// parent.QualName.
func computeQualname(s *Scope) {
	if s.Parent == nil {
		s.QualName = ""
		return
	}
	switch s.Parent.Kind {
	case ScopeModule:
		s.QualName = s.Name
	case ScopeFunction:
		s.QualName = s.Parent.QualName + ".<locals>." + s.Name
	case ScopeClass:
		// Methods defined inside a class skip the .<locals>. infix.
		s.QualName = s.Parent.QualName + "." + s.Name
	case ScopeComprehension:
		// Comprehensions inherit their parent's qualname rules.
		s.QualName = s.Parent.QualName + "." + s.Name
	default:
		s.QualName = s.Name
	}
}

// finalizeQualnames walks the tree in pre-order, computing qualname
// for each scope.
func finalizeQualnames(root *Scope) {
	root.Walk(computeQualname)
}
