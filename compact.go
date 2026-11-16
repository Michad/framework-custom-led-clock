package main

import (
	"flag"
	"fmt"
)

// compactFlag is -compact: which matrix ("left" or "right") to show the
// compact layout on, or "" for the two-matrix layout. It works as a bool
// flag, a bare -compact meaning left, and also takes a value either as
// -compact=right or as the next argument, -compact right (see parseArgs).
type compactFlag struct {
	side string
	// bare is set by a bare -compact, whose side may follow as an argument.
	bare bool
}

func (c *compactFlag) String() string { return c.side }

func (c *compactFlag) IsBoolFlag() bool { return true }

func (c *compactFlag) Set(s string) error {
	c.bare = false
	switch s {
	case "true":
		c.side, c.bare = "left", true
	case "false":
		c.side = ""
	case "left", "right":
		c.side = s
	default:
		return fmt.Errorf("want left or right, got %q", s)
	}
	return nil
}

// parseArgs parses args with fs, taking a left or right straight after a
// bare -compact as its side and parsing on past it.
func parseArgs(fs *flag.FlagSet, args []string, compact *compactFlag) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	for compact.bare && fs.NArg() > 0 {
		side := fs.Arg(0)
		if err := compact.Set(side); err != nil {
			return fmt.Errorf("-compact: %v", err)
		}
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return err
		}
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return nil
}
