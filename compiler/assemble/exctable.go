package assemble

// ExcRegion is one entry in the PEP 657 exception table. v0.6.5
// only handles the empty-table case (no fixture exercises
// exception handling); the type is here so v0.6.10's try/except
// work plugs into a stable interface.
type ExcRegion struct {
	StartUnit   int
	EndUnit     int
	HandlerUnit int
	Depth       int
	LastI       bool
}

// EncodeExcTable emits the PEP 657 exception table. v0.6.5 only
// supports the empty-table case; passing any non-empty regions
// returns a placeholder until v0.6.10 fills in the encoder.
func EncodeExcTable(regions []ExcRegion) []byte {
	if len(regions) == 0 {
		return nil
	}
	// v0.6.10 replaces this branch with the real encoder. Until
	// then, return a recognizable sentinel so a caller misusing
	// the API fails the round-trip rather than silently emitting
	// nothing.
	panic("assemble.EncodeExcTable: non-empty regions not supported until v0.6.10")
}
