package multiplexer

import (
	"testing"

	"github.com/robinjoseph08/wktr/internal/config"
)

func TestCalculateSizes_EvenDistribution(t *testing.T) {
	panes := []config.Pane{{}, {}, {}}
	sizes := calculateSizes(panes, 99)

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
	sizes := calculateSizes(panes, 100)

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
	sizes := calculateSizes(panes, 60)

	if len(sizes) != 1 || sizes[0] != 100 {
		t.Errorf("expected [100], got %v", sizes)
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
