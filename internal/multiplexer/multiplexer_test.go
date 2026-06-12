package multiplexer

import "testing"

// All backs the Multiplexer-agnostic commands (ADR-0002): remove and list fan
// out over every backend instead of resolving the multiplexer setting, so All
// must return each backend exactly once and never consult detection.
func TestAllReturnsEveryBackend(t *testing.T) {
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
}
