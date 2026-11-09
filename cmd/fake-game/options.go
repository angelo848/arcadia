package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// lifetime is a process lifetime: a duration >= 0 or "forever".
type lifetime struct {
	D       time.Duration
	Forever bool
}

func (l *lifetime) String() string {
	if l == nil {
		return "0"
	}
	if l.Forever {
		return "forever"
	}
	return l.D.String()
}

func (l *lifetime) Set(s string) error {
	if s == "forever" {
		*l = lifetime{Forever: true}
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid lifetime %q: %w", s, err)
	}
	if d < 0 {
		return fmt.Errorf("invalid lifetime %q: must be >= 0", s)
	}
	*l = lifetime{D: d}
	return nil
}

// options are the parsed command-line flags (see contracts/fake-game.md).
type options struct {
	Depth, Fanout, ExitCode int
	Run, ParentRun          lifetime
	UntilFile, Report       string
	MaxLife                 time.Duration // default 2m
	Breakaway               bool
	Rest                    []string // positional args after flags
}

const defaultMaxLife = 2 * time.Minute

func newFlagSet(o *options) *flag.FlagSet {
	fs := flag.NewFlagSet("fake-game", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&o.Depth, "depth", 0, "levels below this process")
	fs.IntVar(&o.Fanout, "fanout", 1, "children per non-leaf process")
	fs.Var(&o.Run, "run", "leaf lifetime (duration or forever)")
	fs.Var(&o.ParentRun, "parent-run", "non-leaf lifetime (duration or forever)")
	fs.StringVar(&o.UntilFile, "until-file", "", "exit as soon as this path exists")
	fs.DurationVar(&o.MaxLife, "max-life", defaultMaxLife, "absolute lifetime limit")
	fs.IntVar(&o.ExitCode, "exit-code", 0, "exit code on normal exit (0..255)")
	fs.BoolVar(&o.Breakaway, "breakaway", false, "create children with CREATE_BREAKAWAY_FROM_JOB")
	fs.StringVar(&o.Report, "report", "", "JSON Lines report path")
	return fs
}

// parseOptions parses and validates args (without the program name).
func parseOptions(args []string) (options, error) {
	var o options
	fs := newFlagSet(&o)
	if err := fs.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}
	o.Rest = fs.Args()
	switch {
	case o.Depth < 0:
		return options{}, errors.New("--depth must be >= 0")
	case o.Fanout < 1:
		return options{}, errors.New("--fanout must be >= 1")
	case o.ExitCode < 0 || o.ExitCode > 255:
		return options{}, errors.New("--exit-code must be in 0..255")
	case o.MaxLife <= 0:
		return options{}, errors.New("--max-life must be > 0")
	}
	return o, nil
}

// boolFlags are the flags that never consume a separate value.
var boolFlags = map[string]bool{"breakaway": true}

// childArgs returns args with every --depth occurrence removed and "--depth depth"
// prepended; all other flags (and anything after them) are kept verbatim.
func childArgs(args []string, depth int) []string {
	out := []string{"--depth", strconv.Itoa(depth)}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || !strings.HasPrefix(a, "-") || a == "-" {
			out = append(out, args[i:]...)
			break
		}
		name := strings.TrimLeft(a, "-")
		hasValue := false
		if j := strings.IndexByte(name, '='); j >= 0 {
			name, hasValue = name[:j], true
		}
		takesNext := !hasValue && !boolFlags[name]
		if name == "depth" {
			if takesNext {
				i++
			}
			continue
		}
		out = append(out, a)
		if takesNext && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out
}

// reportLine is one JSON Lines record of --report.
type reportLine struct {
	Event string   `json:"event"` // start | exit
	PID   int      `json:"pid"`
	PPID  int      `json:"ppid,omitempty"`
	Depth int      `json:"depth"`
	Cwd   string   `json:"cwd,omitempty"`
	Args  []string `json:"args,omitempty"`
	Code  *int     `json:"code,omitempty"`
	Error string   `json:"error,omitempty"`
}
