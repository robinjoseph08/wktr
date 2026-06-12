package multiplexer

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/robinjoseph08/wktr/internal/config"
)

func TestDetect(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-501/default,1234,0")
	if !NewTmux().Detect() {
		t.Error("expected Detect to be true when TMUX is set")
	}

	t.Setenv("TMUX", "")
	if NewTmux().Detect() {
		t.Error("expected Detect to be false when TMUX is empty")
	}
}

func TestCalculateSizes_EvenDistribution(t *testing.T) {
	panes := []config.Pane{{}, {}, {}}
	sizes := NewTmux().calculateSizes(panes, 99)

	// 3 panes, no explicit size = 33% each. 33 * 99 / 100 = 32
	for i, size := range sizes {
		expected := 32
		if size != expected {
			t.Errorf("pane %d: expected size %d, got %d", i, expected, size)
		}
	}
}

func TestCalculateSizes_WithExplicitSizes(t *testing.T) {
	panes := []config.Pane{
		{Size: 50},
		{},
		{},
	}
	sizes := NewTmux().calculateSizes(panes, 100)

	if sizes[0] != 50 {
		t.Errorf("pane 0: expected 50, got %d", sizes[0])
	}
	if sizes[1] != 25 {
		t.Errorf("pane 1: expected 25, got %d", sizes[1])
	}
	if sizes[2] != 25 {
		t.Errorf("pane 2: expected 25, got %d", sizes[2])
	}
}

func TestCalculateSizes_SinglePane(t *testing.T) {
	panes := []config.Pane{{}}
	sizes := NewTmux().calculateSizes(panes, 60)

	// A single pane with no explicit size gets the full window height.
	if len(sizes) != 1 || sizes[0] != 60 {
		t.Errorf("expected [60], got %v", sizes)
	}
}

func TestCalculateSizes_HorizontalLayoutsFeedTheWindowWidth(t *testing.T) {
	// calculateSizes is axis-agnostic: a horizontal Layout runs the window
	// width (here 200 columns) through the same percentage math a vertical
	// one runs the height through.
	panes := []config.Pane{
		{Size: 50},
		{},
		{},
	}
	sizes := NewTmux().calculateSizes(panes, 200)

	want := []int{100, 50, 50}
	for i, size := range sizes {
		if size != want[i] {
			t.Errorf("pane %d: expected %d columns, got %d", i, want[i], size)
		}
	}
}

func TestTmuxSplitGeometry(t *testing.T) {
	// The fallbacks are exact-asserted because they only ever run when tmux
	// cannot be queried, so no manual use would catch a transposed or
	// mistyped full-screen approximation (60 lines, 200 columns).
	tests := []struct {
		name         string
		direction    string
		wantFlag     string
		wantDim      string
		wantFallback int
	}{
		{name: "unset direction defaults to vertical", direction: "", wantFlag: "-v", wantDim: "#{window_height}", wantFallback: 60},
		{name: "vertical stacks panes with -v splits sized from the height", direction: "vertical", wantFlag: "-v", wantDim: "#{window_height}", wantFallback: 60},
		{name: "horizontal places panes side by side with -h splits sized from the width", direction: "horizontal", wantFlag: "-h", wantDim: "#{window_width}", wantFallback: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geo := tmuxSplitGeometry(tt.direction)
			if geo.flag != tt.wantFlag {
				t.Errorf("flag: got %q, want %q", geo.flag, tt.wantFlag)
			}
			if geo.dimension != tt.wantDim {
				t.Errorf("dimension: got %q, want %q", geo.dimension, tt.wantDim)
			}
			if geo.fallback != tt.wantFallback {
				t.Errorf("fallback: got %d, want %d", geo.fallback, tt.wantFallback)
			}
		})
	}
}

// installFakeTmux puts a fake tmux executable at the front of PATH that
// records every invocation's arguments and answers display-message dimension
// queries with a 120x50 window (values chosen to differ from the fallbacks,
// proving the dimension is queried rather than assumed). It returns a
// function that reads the recorded invocations, one per line.
func installFakeTmux(t *testing.T) func() []string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %q
if [ "$1" = "display-message" ]; then
	case "$3" in
	'#{window_width}') echo 120 ;;
	'#{window_height}') echo 50 ;;
	esac
fi
`, logPath)
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() []string {
		out, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read fake tmux log: %v", err)
		}
		return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	}
}

// TestTmuxOpenWindowAppliesHorizontalLayout pins the tmux half of Layout
// direction end to end, the way the herdr sibling does over its CLI fake:
// OpenWindow on a horizontal Layout queries the window width and issues -h
// splits sized in columns of it, running the same percentage math the
// vertical path runs over the height.
func TestTmuxOpenWindowAppliesHorizontalLayout(t *testing.T) {
	calls := installFakeTmux(t)

	layout := config.Layout{
		Direction: "horizontal",
		Panes: []config.Pane{
			{Size: 40},
			{Size: 40, Focus: true},
			{Size: 20},
		},
	}
	if err := NewTmux().OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := []string{
		"new-window -d -n my-task -c /worktrees/org/repo/my-task",
		"display-message -p #{window_width}",
		// 120 columns split 40/40/20 is 48/48/24: the first split carves
		// 48+24=72 columns off Pane 0 for the rest of the stack, the second
		// carves 24 off Pane 1 for Pane 2.
		"split-window -d -h -l 72 -t my-task.0 -c /worktrees/org/repo/my-task",
		"split-window -d -h -l 24 -t my-task.1 -c /worktrees/org/repo/my-task",
		"select-window -t my-task",
		"select-pane -t my-task.1",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("tmux calls:\n got %v\nwant %v", got, want)
	}
}

// TestTmuxOpenWindowDefaultsToVerticalSplits pins the direction default
// through OpenWindow: a Layout that never sets a direction queries the
// window height and splits with -v, exactly as it did before direction was
// honored.
func TestTmuxOpenWindowDefaultsToVerticalSplits(t *testing.T) {
	calls := installFakeTmux(t)

	layout := config.Layout{Panes: []config.Pane{{}, {}}}
	if err := NewTmux().OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := []string{
		"new-window -d -n my-task -c /worktrees/org/repo/my-task",
		"display-message -p #{window_height}",
		// 50 lines split evenly: the single split carves 25 lines off Pane 0
		// for Pane 1.
		"split-window -d -v -l 25 -t my-task.0 -c /worktrees/org/repo/my-task",
		"select-window -t my-task",
		"select-pane -t my-task.0",
	}
	if got := calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("tmux calls:\n got %v\nwant %v", got, want)
	}
}

func TestBuildChainedCommand(t *testing.T) {
	tests := []struct {
		name      string
		commands  []config.Command
		wantRun   string
		wantPrime string
	}{
		{
			name: "all run",
			commands: []config.Command{
				{Value: "npm install", Run: boolPtr(true)},
				{Value: "npm build", Run: boolPtr(true)},
			},
			wantRun:   "npm install && npm build",
			wantPrime: "",
		},
		{
			name: "run then prime",
			commands: []config.Command{
				{Value: "npm install", Run: boolPtr(true)},
				{Value: "npm start", Run: boolPtr(false)},
			},
			wantRun:   "npm install",
			wantPrime: "npm start",
		},
		{
			name: "prime only",
			commands: []config.Command{
				{Value: "npm start", Run: boolPtr(false)},
			},
			wantRun:   "",
			wantPrime: "npm start",
		},
		{
			name: "blank values are dropped",
			commands: []config.Command{
				{Value: "npm install", Run: boolPtr(true)},
				{Value: "", Run: boolPtr(true)},
				{Value: "npm test", Run: boolPtr(true)},
				{Value: "npm start", Run: boolPtr(false)},
				{Value: "   ", Run: boolPtr(false)},
			},
			wantRun:   "npm install && npm test",
			wantPrime: "npm start",
		},
		{
			name: "all blank values yield no commands",
			commands: []config.Command{
				{Value: "", Run: boolPtr(true)},
				{Value: " ", Run: boolPtr(false)},
			},
			wantRun:   "",
			wantPrime: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, prime := buildChainedCommand(tt.commands)
			if run != tt.wantRun {
				t.Errorf("run: got %q, want %q", run, tt.wantRun)
			}
			if prime != tt.wantPrime {
				t.Errorf("prime: got %q, want %q", prime, tt.wantPrime)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
