// Command gocopy compiles Python 3.14 source to a CPython-compatible .pyc.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tamnd/gocopy/v1/compiler"
	"github.com/tamnd/gocopy/v1/pyc"
)

const version = "0.1.9"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gocopy:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return nil
	}
	switch args[0] {
	case "version", "-v", "--version":
		fmt.Fprintln(stdout, version)
		return nil
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	case "compile":
		return cmdCompile(args[1:], stdout)
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "gocopy: compile Python 3.14 source to a CPython-compatible .pyc")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "USAGE")
	fmt.Fprintln(w, "  gocopy compile FILE.py [-o OUT.pyc] [--mode MODE] [--source-date-epoch N]")
	fmt.Fprintln(w, "  gocopy version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "FLAGS")
	fmt.Fprintln(w, "  -o PATH                  output file (default: __pycache__/FILE.cpython-314.pyc)")
	fmt.Fprintln(w, "  --mode MODE              one of: timestamp, hash, unchecked-hash (default: timestamp)")
	fmt.Fprintln(w, "  --source-date-epoch N    override source mtime in timestamp mode (env SOURCE_DATE_EPOCH also honored)")
}

type compileFlags struct {
	source          string
	output          string
	mode            string
	sourceDateEpoch *int64
}

func parseCompileFlags(args []string) (*compileFlags, error) {
	f := &compileFlags{mode: "timestamp"}
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "-o":
			if i+1 >= len(args) {
				return nil, errors.New("-o requires a path")
			}
			f.output = args[i+1]
			i += 2
		case a == "--mode":
			if i+1 >= len(args) {
				return nil, errors.New("--mode requires a value")
			}
			f.mode = args[i+1]
			i += 2
		case a == "--source-date-epoch":
			if i+1 >= len(args) {
				return nil, errors.New("--source-date-epoch requires a value")
			}
			n, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("--source-date-epoch: %w", err)
			}
			f.sourceDateEpoch = &n
			i += 2
		case a[0] == '-':
			return nil, fmt.Errorf("unknown flag %q", a)
		default:
			if f.source != "" {
				return nil, errors.New("only one source file accepted")
			}
			f.source = a
			i++
		}
	}
	if f.source == "" {
		return nil, errors.New("missing source file")
	}
	return f, nil
}

func cmdCompile(args []string, stdout io.Writer) error {
	f, err := parseCompileFlags(args)
	if err != nil {
		return err
	}

	mode, err := parseMode(f.mode)
	if err != nil {
		return err
	}

	src, err := os.ReadFile(f.source)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	code, err := compiler.Compile(src, compiler.Options{Filename: f.source})
	if err != nil {
		return err
	}

	out := f.output
	if out == "" {
		out = defaultPycPath(f.source)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return fmt.Errorf("mkdir __pycache__: %w", err)
		}
	}

	sde := f.sourceDateEpoch
	if sde == nil {
		if env := os.Getenv("SOURCE_DATE_EPOCH"); env != "" {
			n, perr := strconv.ParseInt(env, 10, 64)
			if perr != nil {
				return fmt.Errorf("SOURCE_DATE_EPOCH: %w", perr)
			}
			sde = &n
		}
	}

	if err := pyc.WriteFile(out, code, pyc.WriteOptions{
		Mode:            mode,
		SourcePath:      f.source,
		Source:          src,
		SourceDateEpoch: sde,
	}); err != nil {
		return err
	}
	fmt.Fprintln(stdout, out)
	return nil
}

func parseMode(s string) (pyc.InvalidationMode, error) {
	switch s {
	case "timestamp":
		return pyc.ModeTimestamp, nil
	case "hash":
		return pyc.ModeCheckedHash, nil
	case "unchecked-hash":
		return pyc.ModeUncheckedHash, nil
	default:
		return 0, fmt.Errorf("unknown --mode %q (want timestamp / hash / unchecked-hash)", s)
	}
}

// defaultPycPath mirrors CPython's __pycache__ layout:
// foo/bar/baz.py → foo/bar/__pycache__/baz.cpython-314.pyc.
func defaultPycPath(src string) string {
	dir, base := filepath.Split(src)
	stem := base
	if ext := filepath.Ext(base); ext == ".py" {
		stem = base[:len(base)-len(ext)]
	}
	return filepath.Join(dir, "__pycache__", stem+".cpython-314.pyc")
}
