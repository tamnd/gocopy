// Package pyc writes a CPython 3.14 .pyc file: a 16-byte header followed by
// a marshal stream produced by the marshal package.
package pyc

import (
	"fmt"
	"os"

	"github.com/tamnd/gocopy/v1/bytecode"
	"github.com/tamnd/gocopy/v1/marshal"
)

// WriteOptions configures pyc.WriteFile.
type WriteOptions struct {
	Mode            InvalidationMode
	SourcePath      string // file to stat for mtime/size in timestamp mode
	Source          []byte // raw source bytes; hashed in hash mode
	SourceDateEpoch *int64 // overrides mtime if set; matches Reproducible Builds convention
}

// WriteFile writes c to dst as a CPython 3.14 .pyc file.
func WriteFile(dst string, c *bytecode.CodeObject, opts WriteOptions) error {
	body, err := marshal.Marshal(c)
	if err != nil {
		return fmt.Errorf("pyc: marshal: %w", err)
	}

	hopts := HeaderOptions{Mode: opts.Mode, Source: opts.Source}
	if opts.Mode == ModeTimestamp {
		mtime, size, err := statSource(opts.SourcePath, opts.SourceDateEpoch)
		if err != nil {
			return err
		}
		hopts.SourceMTime = mtime
		hopts.SourceSize = size
	}
	hdr, err := BuildHeader(hopts)
	if err != nil {
		return err
	}

	out := make([]byte, 0, len(hdr)+len(body))
	out = append(out, hdr...)
	out = append(out, body...)
	return os.WriteFile(dst, out, 0o644)
}

// statSource returns (mtime, size) for use in timestamp mode. SOURCE_DATE_EPOCH
// (or the explicit override) wins over filesystem mtime; size always comes
// from the file on disk.
func statSource(path string, override *int64) (int64, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, fmt.Errorf("pyc: stat source: %w", err)
	}
	mtime := info.ModTime().Unix()
	if override != nil {
		mtime = *override
	}
	return mtime, info.Size(), nil
}
