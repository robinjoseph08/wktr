package multiplexer

import (
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
	tests := []struct {
		name      string
		direction string
		wantFlag  string
		wantDim   string
	}{
		{name: "unset direction defaults to vertical", direction: "", wantFlag: "-v", wantDim: "#{window_height}"},
		{name: "vertical stacks panes with -v splits sized from the height", direction: "vertical", wantFlag: "-v", wantDim: "#{window_height}"},
		{name: "horizontal places panes side by side with -h splits sized from the width", direction: "horizontal", wantFlag: "-h", wantDim: "#{window_width}"},
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
			if geo.fallback <= 0 {
				t.Errorf("fallback: got %d, want a positive dimension", geo.fallback)
			}
		})
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
