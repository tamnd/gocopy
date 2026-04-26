package pyc

import (
	"encoding/binary"
	"fmt"
)

// Magic is the 4-byte .pyc magic for CPython 3.14.
var Magic = [4]byte{0x2b, 0x0e, 0x0d, 0x0a}

// InvalidationMode picks the .pyc validation strategy. Mirrors
// py_compile.PycInvalidationMode.
type InvalidationMode int

const (
	ModeTimestamp      InvalidationMode = 0 // flags=0; mtime + size in bytes 8..16
	ModeCheckedHash    InvalidationMode = 1 // flags=1; SipHash of source in bytes 8..16
	ModeUncheckedHash  InvalidationMode = 2 // flags=3; same SipHash but flagged unchecked
)

// HeaderOptions configures the 16-byte .pyc header.
type HeaderOptions struct {
	Mode       InvalidationMode
	SourceMTime int64  // truncated to u32 in timestamp mode
	SourceSize  int64  // truncated to u32 in timestamp mode
	Source     []byte // hashed in hash mode
}

// BuildHeader returns the 16-byte .pyc header per opts. Layout:
//
//	[0..4)   = magic
//	[4..8)   = flags (u32 little-endian)
//	[8..16)  = validation field
//
// In timestamp mode flags=0, validation = (mtime u32, size u32).
// In hash mode flags=1 (checked) or 3 (unchecked), validation = SipHash-1-3.
func BuildHeader(opts HeaderOptions) ([]byte, error) {
	out := make([]byte, 16)
	copy(out, Magic[:])

	switch opts.Mode {
	case ModeTimestamp:
		binary.LittleEndian.PutUint32(out[4:8], 0)
		binary.LittleEndian.PutUint32(out[8:12], uint32(opts.SourceMTime))
		binary.LittleEndian.PutUint32(out[12:16], uint32(opts.SourceSize))
	case ModeCheckedHash:
		binary.LittleEndian.PutUint32(out[4:8], 1)
		h := SourceHash(opts.Source)
		copy(out[8:16], h[:])
	case ModeUncheckedHash:
		binary.LittleEndian.PutUint32(out[4:8], 3)
		h := SourceHash(opts.Source)
		copy(out[8:16], h[:])
	default:
		return nil, fmt.Errorf("pyc: unknown invalidation mode %d", opts.Mode)
	}
	return out, nil
}
