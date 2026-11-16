package main

import (
	"flag"
	"io"
	"testing"
)

func TestParseCompact(t *testing.T) {
	for _, c := range []struct {
		args []string
		side string
		fill string
	}{
		{nil, "", "slideup"},
		{[]string{"-fill", "topo"}, "", "topo"},
		{[]string{"-compact"}, "left", "slideup"},
		{[]string{"-compact", "-fill", "topo"}, "left", "topo"},
		{[]string{"-compact", "left"}, "left", "slideup"},
		{[]string{"-compact", "right"}, "right", "slideup"},
		{[]string{"-compact", "right", "-fill", "disco"}, "right", "disco"},
		{[]string{"-fill", "disco", "-compact", "right"}, "right", "disco"},
		{[]string{"-compact=right", "-fill", "disco"}, "right", "disco"},
		{[]string{"--compact=left"}, "left", "slideup"},
		{[]string{"-compact=false"}, "", "slideup"},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var compact compactFlag
		fs.Var(&compact, "compact", "")
		fill := fs.String("fill", "slideup", "")
		if err := parseArgs(fs, c.args, &compact); err != nil {
			t.Errorf("%q: %v", c.args, err)
			continue
		}
		if compact.side != c.side || *fill != c.fill {
			t.Errorf("%q: compact=%q fill=%q, want %q %q", c.args, compact.side, *fill, c.side, c.fill)
		}
	}
	for _, args := range [][]string{
		{"-compact", "middle"},
		{"-compact=middle"},
		{"-compact", "left", "right"},
		{"stray"},
		{"-compact=right", "left"},
	} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var compact compactFlag
		fs.Var(&compact, "compact", "")
		if err := parseArgs(fs, args, &compact); err == nil {
			t.Errorf("%q: parsed as compact=%q, want an error", args, compact.side)
		}
	}
}
