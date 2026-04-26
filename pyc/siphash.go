package pyc

import "encoding/binary"

// siphash13 computes SipHash-1-3 of msg under the 128-bit key (k0, k1).
//
// CPython's _Py_KeyedHash uses SipHash-1-3 with k0 = caller-supplied key,
// k1 = 0 (Python/pyhash.c::_Py_KeyedHash → siphash13). The .pyc validation
// hash uses _imp.pyc_magic_number_token as k0; that token is just the
// magic number's u32 widened to u64 (the high bytes are randomized at
// interpreter startup but the .pyc-on-disk path uses the deterministic
// token, see Modules/_imp.c::_imp_source_hash_impl).
//
// SipHash spec: https://www.aumasson.jp/siphash/siphash.pdf
func siphash13(k0, k1 uint64, msg []byte) uint64 {
	v0 := k0 ^ 0x736f6d6570736575
	v1 := k1 ^ 0x646f72616e646f6d
	v2 := k0 ^ 0x6c7967656e657261
	v3 := k1 ^ 0x7465646279746573

	full := len(msg) - len(msg)%8
	for i := 0; i < full; i += 8 {
		m := binary.LittleEndian.Uint64(msg[i : i+8])
		v3 ^= m
		v0, v1, v2, v3 = sipRound(v0, v1, v2, v3) // 1 compression round
		v0 ^= m
	}

	// Final block: pack remaining bytes plus length-mod-256 in MSB.
	var b [8]byte
	copy(b[:], msg[full:])
	b[7] = byte(len(msg) & 0xff)
	m := binary.LittleEndian.Uint64(b[:])

	v3 ^= m
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0 ^= m

	v2 ^= 0xff
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)
	v0, v1, v2, v3 = sipRound(v0, v1, v2, v3)

	return v0 ^ v1 ^ v2 ^ v3
}

func sipRound(v0, v1, v2, v3 uint64) (uint64, uint64, uint64, uint64) {
	v0 += v1
	v1 = rotl(v1, 13)
	v1 ^= v0
	v0 = rotl(v0, 32)

	v2 += v3
	v3 = rotl(v3, 16)
	v3 ^= v2

	v0 += v3
	v3 = rotl(v3, 21)
	v3 ^= v0

	v2 += v1
	v1 = rotl(v1, 17)
	v1 ^= v2
	v2 = rotl(v2, 32)

	return v0, v1, v2, v3
}

func rotl(x uint64, n uint) uint64 {
	return (x << n) | (x >> (64 - n))
}

// magicToken is the u64 key SipHash-1-3 uses for .pyc source hashing.
// Equals _imp.pyc_magic_number_token at runtime (verified empirically
// against Python 3.14.4: 0x0a0d0e2b, the magic bytes 0x2b 0x0e 0x0d 0x0a
// interpreted as little-endian u32, widened to u64).
const magicToken uint64 = 0x0a0d0e2b

// SourceHash is the 8-byte SipHash-1-3 of source bytes under the .pyc key.
// Output is little-endian (CPython writes the low byte first to the file).
func SourceHash(source []byte) [8]byte {
	h := siphash13(magicToken, 0, source)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], h)
	return b
}
