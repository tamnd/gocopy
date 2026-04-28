package bytecode

import "strings"

// ImportAlias is one (name, asname) pair in an import statement.
type ImportAlias struct {
	Name   string
	Asname string // empty = same as Name; local binding name
}

// ImportEntry is one import or from-import statement.
type ImportEntry struct {
	Line   int
	EndCol byte
	IsFrom bool
	// IsFrom=false: simple import
	Module string // full module name (e.g., "os", "os.path")
	Asname string // local alias; empty = top-level component of Module
	// IsFrom=true: from-import
	FromMod string
	Level   int           // 0=absolute
	Aliases []ImportAlias // imported names, in order
}

// LocalName returns the local binding name for a simple import entry.
func (e *ImportEntry) LocalName() string {
	if e.Asname != "" {
		return e.Asname
	}
	if i := strings.Index(e.Module, "."); i >= 0 {
		return e.Module[:i]
	}
	return e.Module
}

// numSimpleAliases returns the number of alias entries in a simple import.
// For `import X, Y, Z`, this is 3. For `import X`, it is 1.
// IsFrom entries always return 0 here; use len(Aliases) instead.
func (e *ImportEntry) numSimpleAliases() int {
	if e.IsFrom {
		return 0
	}
	return 1 // each ImportEntry represents exactly one alias
}

// cuCount returns the number of code units (instructions) this entry
// contributes, NOT including the final LOAD_CONST None + RETURN_VALUE.
func (e *ImportEntry) cuCount() int {
	if !e.IsFrom {
		// LOAD_SMALL_INT + LOAD_CONST + IMPORT_NAME + STORE_NAME
		return 4
	}
	// LOAD_SMALL_INT + LOAD_CONST(tuple) + IMPORT_NAME +
	// (IMPORT_FROM + STORE_NAME)*N + POP_TOP
	return 4 + 2*len(e.Aliases)
}

// ImportBytecode builds the bytecode for a sequence of import entries.
// noneIdx is the co_consts index for None. The final LOAD_CONST None +
// RETURN_VALUE are appended after all entries.
// Each entry's constIdx is the co_consts index for its fromlist constant.
func ImportBytecode(entries []ImportEntry, constIdxs []byte, nameRefs []ImportNameRef) []byte {
	out := []byte{byte(RESUME), 0}
	noneIdx := constIdxs[len(constIdxs)-1] // last const is always None
	for i, e := range entries {
		if !e.IsFrom {
			ref := nameRefs[i]
			out = append(out,
				byte(LOAD_SMALL_INT), byte(e.Level),
				byte(LOAD_CONST), constIdxs[i],
				byte(IMPORT_NAME), ref.ModuleIdx,
				byte(STORE_NAME), ref.StoreIdx,
			)
		} else {
			ref := nameRefs[i]
			out = append(out,
				byte(LOAD_SMALL_INT), byte(e.Level),
				byte(LOAD_CONST), constIdxs[i],
				byte(IMPORT_NAME), ref.ModuleIdx,
			)
			for j, a := range ref.AliasIdxs {
				_ = a
				out = append(out,
					byte(IMPORT_FROM), ref.AliasIdxs[j].NameIdx,
					byte(STORE_NAME), ref.AliasIdxs[j].StoreIdx,
				)
			}
			out = append(out, byte(POP_TOP), 0)
		}
	}
	out = append(out, byte(LOAD_CONST), noneIdx, byte(RETURN_VALUE), 0)
	return out
}

// ImportNameRef holds pre-computed co_names indices for one import entry.
type ImportNameRef struct {
	ModuleIdx byte
	StoreIdx  byte // for simple import
	AliasIdxs []AliasNameRef
}

// AliasNameRef holds co_names indices for one alias in a from-import.
type AliasNameRef struct {
	NameIdx  byte // IMPORT_FROM oparg (original attribute name)
	StoreIdx byte // STORE_NAME oparg (local binding)
}

// ImportLineTable builds the linetable for a sequence of import entries.
// All imports are at absolute level; prevLine is the line before the first
// import (use 0 since the RESUME prologue is at the synthetic line 0).
func ImportLineTable(entries []ImportEntry, isLast []bool) []byte {
	// RESUME prologue: 1 CU, LONG, line delta -1 from firstlineno.
	out := []byte{0xf0, 0x03, 0x01, 0x01, 0x01}

	prevLine := 0 // synthetic line (firstlineno - 1)

	for i, e := range entries {
		cu := e.cuCount()
		if isLast[i] {
			cu += 2 // LOAD_CONST None + RETURN_VALUE
		}

		lineDelta := e.Line - prevLine
		sc := byte(0) // all imports start at col 0
		ec := e.EndCol

		remaining := cu
		first := true
		for remaining > 0 {
			chunk := remaining
			if chunk > 8 {
				chunk = 8
			}
			remaining -= chunk
			if first {
				out = appendImportFirstChunk(out, lineDelta, chunk, sc, ec)
				first = false
			} else {
				out = appendImportContChunk(out, chunk, sc, ec)
			}
		}

		prevLine = e.Line
	}
	return out
}

// appendImportFirstChunk emits the first chunk for a new source line.
func appendImportFirstChunk(out []byte, lineDelta, length int, sc, ec byte) []byte {
	switch lineDelta {
	case 0:
		out = append(out, entryHeader(codeOneLine0, length), sc, ec)
	case 1:
		out = append(out, entryHeader(codeOneLine1, length), sc, ec)
	case 2:
		out = append(out, entryHeader(codeOneLine2, length), sc, ec)
	default:
		out = append(out, entryHeader(codeLong, length))
		out = appendSignedVarint(out, lineDelta)
		out = append(out, 0x00) // end_line_delta = 0
		out = appendVarint(out, uint(sc)+1)
		out = appendVarint(out, uint(ec)+1)
	}
	return out
}

// appendImportContChunk emits a continuation chunk on the same source line.
// Uses SHORT entry (kind=0) when startCol ≤ 15 and delta ≤ 15, else ONE_LINE0.
func appendImportContChunk(out []byte, length int, sc, ec byte) []byte {
	delta := int(ec) - int(sc)
	if int(sc) <= 15 && delta >= 0 && delta <= 15 {
		payload := (sc << 4) | byte(delta)
		out = append(out, entryHeader(0, length), payload)
	} else {
		out = append(out, entryHeader(codeOneLine0, length), sc, ec)
	}
	return out
}
