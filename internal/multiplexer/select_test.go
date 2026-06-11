package multiplexer

import (
	"strings"
	"testing"
)

func TestSelect(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		insideTmux  bool
		insideHerdr bool
		want        string
		wantErrSubs []string
	}{
		// auto: pick whichever Multiplexer wktr is inside, error on none or
		// both.
		{
			name:        "auto outside both errors naming tmux and herdr",
			value:       "auto",
			wantErrSubs: []string{"tmux", "herdr"},
		},
		{
			name:       "auto inside tmux selects tmux",
			value:      "auto",
			insideTmux: true,
			want:       "tmux",
		},
		{
			name:        "auto inside herdr selects herdr",
			value:       "auto",
			insideHerdr: true,
			want:        "herdr",
		},
		{
			name:        "auto inside both errors with pin instructions",
			value:       "auto",
			insideTmux:  true,
			insideHerdr: true,
			wantErrSubs: []string{"both", "pin", "multiplexer"},
		},

		// tmux pin: check only the tmux gate.
		{
			name:        "tmux pin outside both errors on the tmux gate",
			value:       "tmux",
			wantErrSubs: []string{"tmux", "TMUX"},
		},
		{
			name:       "tmux pin inside tmux selects tmux",
			value:      "tmux",
			insideTmux: true,
			want:       "tmux",
		},
		{
			name:        "tmux pin inside herdr only errors on the tmux gate",
			value:       "tmux",
			insideHerdr: true,
			wantErrSubs: []string{"tmux", "TMUX"},
		},
		{
			name:        "tmux pin inside both selects tmux without ambiguity",
			value:       "tmux",
			insideTmux:  true,
			insideHerdr: true,
			want:        "tmux",
		},

		// herdr pin: check only the herdr gate.
		{
			name:        "herdr pin outside both errors on the herdr gate",
			value:       "herdr",
			wantErrSubs: []string{"herdr", "HERDR_ENV"},
		},
		{
			name:        "herdr pin inside tmux only errors on the herdr gate",
			value:       "herdr",
			insideTmux:  true,
			wantErrSubs: []string{"herdr", "HERDR_ENV"},
		},
		{
			name:        "herdr pin inside herdr selects herdr",
			value:       "herdr",
			insideHerdr: true,
			want:        "herdr",
		},
		{
			name:        "herdr pin inside both selects herdr without ambiguity",
			value:       "herdr",
			insideTmux:  true,
			insideHerdr: true,
			want:        "herdr",
		},

		// Unknown values are rejected at config load time; Select still guards
		// against them.
		{
			name:        "unknown value errors naming the valid options",
			value:       "screen",
			insideTmux:  true,
			insideHerdr: true,
			wantErrSubs: []string{`"screen"`, "tmux", "herdr", "auto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, err := Select(tt.value, tt.insideTmux, tt.insideHerdr)

			if len(tt.wantErrSubs) > 0 {
				if err == nil {
					t.Fatalf("expected error, got backend %T", mux)
				}
				for _, sub := range tt.wantErrSubs {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("expected error to contain %q, got: %v", sub, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got string
			switch mux.(type) {
			case *Tmux:
				got = "tmux"
			case *Herdr:
				got = "herdr"
			default:
				t.Fatalf("unexpected backend type %T", mux)
			}
			if got != tt.want {
				t.Errorf("expected backend %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSelectFromEnvUsesRealSignals(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("HERDR_ENV", "1")

	mux, err := SelectFromEnv("auto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := mux.(*Herdr); !ok {
		t.Errorf("expected herdr backend, got %T", mux)
	}

	t.Setenv("TMUX", "/tmp/tmux-501/default,1234,0")
	if _, err := SelectFromEnv("auto"); err == nil {
		t.Error("expected ambiguity error when both signals are set")
	}
}
