package bytecode

// Atom is an interned identifier. It indexes into an AtomTable.
// Atom IDs are dense and start at 0, so they can directly drive
// arrays such as co_names indexing.
type Atom uint32

// AtomTable interns strings. Insertion order is preserved: the first
// string passed to Intern gets atom 0, the next unique string gets 1,
// and so on. Re-interning a known string returns its original ID.
//
// Determinism matters: gocopy needs byte-identical output, and
// co_names ordering follows source order. The slice + map combo
// keeps interning O(1) amortized while preserving that ordering.
type AtomTable struct {
	byID    []string
	byValue map[string]Atom
}

// NewAtomTable returns an empty AtomTable.
func NewAtomTable() *AtomTable {
	return &AtomTable{byValue: make(map[string]Atom)}
}

// Intern returns the Atom for s, allocating a new one if it has not
// been seen before.
func (t *AtomTable) Intern(s string) Atom {
	if a, ok := t.byValue[s]; ok {
		return a
	}
	a := Atom(len(t.byID))
	t.byID = append(t.byID, s)
	t.byValue[s] = a
	return a
}

// String returns the string for the given Atom. It panics if the
// Atom is out of range; that is always a programmer error.
func (t *AtomTable) String(a Atom) string {
	return t.byID[a]
}

// Len returns the number of distinct atoms.
func (t *AtomTable) Len() int {
	return len(t.byID)
}

// Slice returns a defensive copy of all interned strings in atom
// order, suitable for emitting as a tuple constant.
func (t *AtomTable) Slice() []string {
	out := make([]string, len(t.byID))
	copy(out, t.byID)
	return out
}
