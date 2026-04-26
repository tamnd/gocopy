package pyc

import (
	"bytes"
	"testing"
)

func TestBuildHeaderTimestamp(t *testing.T) {
	t.Parallel()
	hdr, err := BuildHeader(HeaderOptions{
		Mode:        ModeTimestamp,
		SourceMTime: 1577836800, // 2020-01-01T00:00:00Z
		SourceSize:  0,
	})
	if err != nil {
		t.Fatalf("BuildHeader: %v", err)
	}
	want := []byte{
		0x2b, 0x0e, 0x0d, 0x0a, // magic
		0, 0, 0, 0, // flags=0 timestamp mode
		0x00, 0xe1, 0x0b, 0x5e, // mtime u32 LE = 1577836800
		0, 0, 0, 0, // size=0
	}
	if !bytes.Equal(hdr, want) {
		t.Errorf("header timestamp\nwant: %x\n got: %x", want, hdr)
	}
}

func TestBuildHeaderUncheckedHash(t *testing.T) {
	t.Parallel()
	hdr, err := BuildHeader(HeaderOptions{
		Mode:   ModeUncheckedHash,
		Source: []byte{},
	})
	if err != nil {
		t.Fatalf("BuildHeader: %v", err)
	}
	want := []byte{
		0x2b, 0x0e, 0x0d, 0x0a, // magic
		3, 0, 0, 0, // flags=3 unchecked hash
		0x61, 0xf5, 0xb7, 0xcb, 0x65, 0x3a, 0x6a, 0xa4, // SipHash of empty
	}
	if !bytes.Equal(hdr, want) {
		t.Errorf("header unchecked-hash\nwant: %x\n got: %x", want, hdr)
	}
}

func TestBuildHeaderCheckedHash(t *testing.T) {
	t.Parallel()
	hdr, err := BuildHeader(HeaderOptions{
		Mode:   ModeCheckedHash,
		Source: []byte("abc"),
	})
	if err != nil {
		t.Fatalf("BuildHeader: %v", err)
	}
	want := []byte{
		0x2b, 0x0e, 0x0d, 0x0a, // magic
		1, 0, 0, 0, // flags=1 checked hash
		0x6e, 0x46, 0x1b, 0xf6, 0xa5, 0xe9, 0x7b, 0x4a, // SipHash of "abc"
	}
	if !bytes.Equal(hdr, want) {
		t.Errorf("header checked-hash\nwant: %x\n got: %x", want, hdr)
	}
}
