package multiplexer

import "testing"

// All backs the Multiplexer-agnostic commands (ADR-0002): remove and list fan
// out over every backend instead of resolving the multiplexer setting, so All
// must return each backend exactly once, in fixed order, regardless of which
// Multiplexer wktr is running inside.
func TestAllReturnsEveryBackend(t *testing.T) {
	tests := []struct {
		name  string
		tmux  string
		herdr string
	}{
		{"outside both multiplexers", "", ""},
		{"inside tmux only", "/tmp/tmux-1000/default,1234,0", ""},
		{"inside herdr only", "", "1"},
		{"inside both multiplexers", "/tmp/tmux-1000/default,1234,0", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMUX", tt.tmux)
			t.Setenv("HERDR_ENV", tt.herdr)

			muxes := All()
			if len(muxes) != 2 {
				t.Fatalf("expected 2 backends, got %d", len(muxes))
			}
			if _, ok := muxes[0].(*Tmux); !ok {
				t.Errorf("expected first backend to be tmux, got %T", muxes[0])
			}
			if _, ok := muxes[1].(*Herdr); !ok {
				t.Errorf("expected second backend to be herdr, got %T", muxes[1])
			}
		})
	}
}
