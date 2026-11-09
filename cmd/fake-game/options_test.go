package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParseOptions(t *testing.T) {
	t.Parallel()

	errorCases := []struct {
		name string
		args []string
	}{
		{"FKG-007/flag desconhecida é erro", []string{"--bogus"}},
		{"FKG-007/--depth -1 é erro", []string{"--depth", "-1"}},
		{"FKG-007/--run abc é erro", []string{"--run", "abc"}},
		{"FKG-007/--fanout 0 é erro", []string{"--fanout", "0"}},
		{"FKG-007/--exit-code 256 é erro", []string{"--exit-code", "256"}},
		{"FKG-007/--max-life 0 é erro", []string{"--max-life", "0"}},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseOptions(tc.args); err == nil {
				t.Fatalf("parseOptions(%q) = nil error, want error", tc.args)
			}
		})
	}

	t.Run("FKG-005/padrão de --max-life é 2m", func(t *testing.T) {
		t.Parallel()
		opts, err := parseOptions(nil)
		if err != nil {
			t.Fatalf("parseOptions(nil): %v", err)
		}
		if opts.MaxLife != 2*time.Minute {
			t.Errorf("MaxLife = %v, want 2m", opts.MaxLife)
		}
		// Other defaults from the contract table.
		if opts.Depth != 0 || opts.Fanout != 1 || opts.ExitCode != 0 {
			t.Errorf("defaults depth/fanout/exit-code = %d/%d/%d, want 0/1/0", opts.Depth, opts.Fanout, opts.ExitCode)
		}
		if opts.Run != (lifetime{}) || opts.ParentRun != (lifetime{}) {
			t.Errorf("defaults run/parent-run = %+v/%+v, want zero", opts.Run, opts.ParentRun)
		}
		if opts.Breakaway || opts.UntilFile != "" || opts.Report != "" {
			t.Errorf("defaults breakaway/until-file/report = %v/%q/%q, want false/\"\"/\"\"", opts.Breakaway, opts.UntilFile, opts.Report)
		}
	})

	t.Run("FKG-003/forever é aceito em --run e --parent-run", func(t *testing.T) {
		t.Parallel()
		opts, err := parseOptions([]string{"--depth", "1", "--run", "forever", "--parent-run", "forever"})
		if err != nil {
			t.Fatalf("parseOptions: %v", err)
		}
		if !opts.Run.Forever {
			t.Errorf("Run.Forever = false, want true")
		}
		if !opts.ParentRun.Forever {
			t.Errorf("ParentRun.Forever = false, want true")
		}

		opts, err = parseOptions([]string{"--run", "300ms", "--parent-run", "2s"})
		if err != nil {
			t.Fatalf("parseOptions durations: %v", err)
		}
		if opts.Run != (lifetime{D: 300 * time.Millisecond}) {
			t.Errorf("Run = %+v, want {D:300ms}", opts.Run)
		}
		if opts.ParentRun != (lifetime{D: 2 * time.Second}) {
			t.Errorf("ParentRun = %+v, want {D:2s}", opts.ParentRun)
		}
	})
}

func TestChildArgs(t *testing.T) {
	t.Parallel()

	t.Run("FKG-002/mesmas flags com --depth d-1", func(t *testing.T) {
		t.Parallel()
		inputs := []struct {
			args  []string
			depth int
		}{
			{[]string{"--depth", "2", "--fanout", "2", "--parent-run", "0", "--run", "forever",
				"--until-file", `C:\tmp dir\F`, "--max-life", "30s", "--exit-code", "3", "--report", `C:\tmp dir\r.jsonl`}, 1},
			{[]string{"--fanout", "3", "--depth", "1", "--breakaway", "--run", "5s"}, 0},
			{[]string{"--run", "forever", "--depth=3", "--parent-run", "2s"}, 2},
		}
		for _, in := range inputs {
			parent, err := parseOptions(in.args)
			if err != nil {
				t.Fatalf("parseOptions(%q): %v", in.args, err)
			}
			got := childArgs(in.args, in.depth)
			child, err := parseOptions(got)
			if err != nil {
				t.Fatalf("parseOptions(childArgs(%q, %d) = %q): %v", in.args, in.depth, got, err)
			}
			want := parent
			want.Depth = in.depth
			// Positional args are not flags; the contract does not say whether children inherit them.
			want.Rest, child.Rest = nil, nil
			if !reflect.DeepEqual(child, want) {
				t.Errorf("childArgs(%q, %d) = %q\nparsed %+v\nwant   %+v", in.args, in.depth, got, child, want)
			}
		}
	})
}
