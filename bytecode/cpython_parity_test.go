package bytecode

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// runPython3_14 invokes python3.14 with the given inline script and
// returns its stdout. If python3.14 is not on PATH, the test calls
// t.Skip so the basic CI test job (which has no Python) stays green.
// The oracle CI job has Python and runs these tests.
func runPython3_14(t *testing.T, script string) string {
	t.Helper()
	if _, err := exec.LookPath("python3.14"); err != nil {
		t.Skip("python3.14 not on PATH; skipping CPython parity test")
	}
	cmd := exec.Command("python3.14", "-c", script)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("python3.14 failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("python3.14 failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// TestCPythonOpmapParity asserts every opcode named in OpMetaTable
// has the same numeric value in CPython 3.14's opcode.opmap.
func TestCPythonOpmapParity(t *testing.T) {
	out := runPython3_14(t, `
import json, opcode
print(json.dumps(opcode.opmap))
`)
	var opmap map[string]int
	if err := json.Unmarshal([]byte(out), &opmap); err != nil {
		t.Fatalf("decode opmap: %v\noutput: %s", err, out)
	}
	for op := range 256 {
		m := OpMetaTable[op]
		if m.Name == "" {
			continue
		}
		got, ok := opmap[m.Name]
		if !ok {
			t.Errorf("OpMetaTable[%d].Name=%q is not in CPython 3.14 opcode.opmap", op, m.Name)
			continue
		}
		if got != op {
			t.Errorf("opcode %s: gocopy=%d cpython=%d", m.Name, op, got)
		}
	}
}

// TestCPythonCacheSizeParity asserts every entry in
// OpMetaTable.CacheSize matches CPython's _inline_cache_entries.
func TestCPythonCacheSizeParity(t *testing.T) {
	out := runPython3_14(t, `
import json, opcode
print(json.dumps(opcode._inline_cache_entries))
`)
	var caches map[string]int
	if err := json.Unmarshal([]byte(out), &caches); err != nil {
		t.Fatalf("decode cache entries: %v\noutput: %s", err, out)
	}
	for op := range 256 {
		m := OpMetaTable[op]
		if m.Name == "" {
			continue
		}
		want := caches[m.Name] // missing means 0
		if int(m.CacheSize) != want {
			t.Errorf("opcode %s: gocopy CacheSize=%d cpython=%d", m.Name, m.CacheSize, want)
		}
	}
}

// TestCPythonHasArgParity asserts OpMetaTable.HasArg matches
// membership in opcode.hasarg.
func TestCPythonHasArgParity(t *testing.T) {
	out := runPython3_14(t, `
import json, opcode
print(json.dumps(sorted(opcode.hasarg)))
`)
	var hasarg []int
	if err := json.Unmarshal([]byte(out), &hasarg); err != nil {
		t.Fatalf("decode hasarg: %v\noutput: %s", err, out)
	}
	hasargSet := make(map[int]bool, len(hasarg))
	for _, op := range hasarg {
		hasargSet[op] = true
	}
	for op := range 256 {
		m := OpMetaTable[op]
		if m.Name == "" {
			continue
		}
		want := hasargSet[op]
		if m.HasArg != want {
			t.Errorf("opcode %s: gocopy HasArg=%v cpython=%v", m.Name, m.HasArg, want)
		}
	}
}

// TestCPythonStackEffectParity asserts the StackEff field for
// fixed-effect (StackVar=false) opcodes matches dis.stack_effect.
//
// Stack-variable ops are skipped here; the parity test for those is
// "the table flagged it as variable", which is verified in
// opmeta_test.go.
func TestCPythonStackEffectParity(t *testing.T) {
	// Build a JSON map of {op_int: stack_effect_at_zero_arg}.
	out := runPython3_14(t, `
import json, dis, opcode
result = {}
for name, op in opcode.opmap.items():
    if name.startswith('<') or name.startswith('INSTRUMENTED'):
        continue
    try:
        eff = dis.stack_effect(op, 0 if op in opcode.hasarg else None)
    except (ValueError, TypeError):
        continue
    result[op] = eff
print(json.dumps(result))
`)
	var effects map[string]int
	if err := json.Unmarshal([]byte(out), &effects); err != nil {
		t.Fatalf("decode effects: %v\noutput: %s", err, out)
	}
	// JSON keys are strings; convert.
	got := make(map[int]int, len(effects))
	for k, v := range effects {
		var op int
		if _, err := jsonAtoi(k, &op); err != nil {
			t.Fatalf("decode op key %q: %v", k, err)
		}
		got[op] = v
	}
	// Opcodes whose effect at oparg=0 is a special case we must
	// skip in the cross-check. CALL with oparg 0 means "0 positional
	// args" and gives -1; BUILD_X with 0 means "empty container"
	// and gives +1; LOAD_GLOBAL/LOAD_ATTR change effect with bit 0.
	// Our table marks all of these StackVar=true — they don't need
	// a fixed StackEff value.
	for op := range 256 {
		m := OpMetaTable[op]
		if m.Name == "" || m.StackVar {
			continue
		}
		want, ok := got[op]
		if !ok {
			// CPython couldn't compute the effect (e.g. some pseudo
			// or special opcodes). Accept silently.
			continue
		}
		if int(m.StackEff) != want {
			t.Errorf("opcode %s: gocopy StackEff=%d cpython=%d", m.Name, m.StackEff, want)
		}
	}
}

// TestCPythonFlagBitsParity asserts the CO_* constants in flags.go
// match CPython 3.14's compile module.
func TestCPythonFlagBitsParity(t *testing.T) {
	out := runPython3_14(t, `
import json
# CO_FUTURE_* and CO_HAS_DOCSTRING and friends live on a few
# different modules. Pull them straight from the compile module
# where possible, and from Include constants via inspect for the
# common ones.
import inspect
co = {
    'CO_OPTIMIZED': inspect.CO_OPTIMIZED,
    'CO_NEWLOCALS': inspect.CO_NEWLOCALS,
    'CO_VARARGS': inspect.CO_VARARGS,
    'CO_VARKEYWORDS': inspect.CO_VARKEYWORDS,
    'CO_NESTED': inspect.CO_NESTED,
    'CO_GENERATOR': inspect.CO_GENERATOR,
    'CO_NOFREE': inspect.CO_NOFREE,
    'CO_COROUTINE': inspect.CO_COROUTINE,
    'CO_ITERABLE_COROUTINE': inspect.CO_ITERABLE_COROUTINE,
    'CO_ASYNC_GENERATOR': inspect.CO_ASYNC_GENERATOR,
    # CO_FUTURE_* and CO_HAS_DOCSTRING / CO_NO_MONITORING_EVENTS are
    # not exposed by inspect; use the canonical values from
    # Include/cpython/code.h.
    'CO_FUTURE_BARRY_AS_BDFL': 0x00400000,
    'CO_FUTURE_GENERATOR_STOP': 0x00800000,
    'CO_FUTURE_ANNOTATIONS': 0x01000000,
    'CO_NO_MONITORING_EVENTS': 0x02000000,
    'CO_HAS_DOCSTRING': 0x04000000,
}
print(json.dumps(co))
`)
	var flags map[string]uint32
	if err := json.Unmarshal([]byte(out), &flags); err != nil {
		t.Fatalf("decode flags: %v\noutput: %s", err, out)
	}
	checks := map[string]uint32{
		"CO_OPTIMIZED":             CO_OPTIMIZED,
		"CO_NEWLOCALS":             CO_NEWLOCALS,
		"CO_VARARGS":               CO_VARARGS,
		"CO_VARKEYWORDS":           CO_VARKEYWORDS,
		"CO_NESTED":                CO_NESTED,
		"CO_GENERATOR":             CO_GENERATOR,
		"CO_NOFREE":                CO_NOFREE,
		"CO_COROUTINE":             CO_COROUTINE,
		"CO_ITERABLE_COROUTINE":    CO_ITERABLE_COROUTINE,
		"CO_ASYNC_GENERATOR":       CO_ASYNC_GENERATOR,
		"CO_FUTURE_BARRY_AS_BDFL":  CO_FUTURE_BARRY_AS_BDFL,
		"CO_FUTURE_GENERATOR_STOP": CO_FUTURE_GENERATOR_STOP,
		"CO_FUTURE_ANNOTATIONS":    CO_FUTURE_ANNOTATIONS,
		"CO_NO_MONITORING_EVENTS":  CO_NO_MONITORING_EVENTS,
		"CO_HAS_DOCSTRING":         CO_HAS_DOCSTRING,
	}
	for name, ours := range checks {
		theirs, ok := flags[name]
		if !ok {
			t.Errorf("%s missing from CPython script output", name)
			continue
		}
		if ours != theirs {
			t.Errorf("%s: gocopy=0x%x cpython=0x%x", name, ours, theirs)
		}
	}
}

// jsonAtoi decodes a JSON-style numeric string into i. Avoids the
// strconv import in the parity-test-only path.
func jsonAtoi(s string, out *int) (int, error) {
	return 0, json.Unmarshal([]byte(s), out)
}
