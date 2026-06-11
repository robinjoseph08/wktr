package multiplexer

import "fmt"

// Select picks the backend for a resolved multiplexer config value given the
// environment signals. It is a pure function of its inputs so every
// combination of value and signals is unit testable (ADR-0002).
//
// auto picks whichever Multiplexer wktr is running inside. When both signals
// are present, wktr refuses to guess: nested multiplexers are
// indistinguishable from env inheritance, so the user must pin the choice in
// config. Explicit tmux and herdr pins check only that backend's gate.
func Select(value string, insideTmux, insideHerdr bool) (Multiplexer, error) {
	switch value {
	case "tmux":
		if !insideTmux {
			return nil, fmt.Errorf("multiplexer %q is configured but no tmux session was detected (TMUX is not set)", "tmux")
		}
		return NewTmux(), nil
	case "herdr":
		if !insideHerdr {
			return nil, fmt.Errorf("multiplexer %q is configured but no herdr session was detected (HERDR_ENV=1 was not found)", "herdr")
		}
		return NewHerdr(), nil
	case "auto":
		switch {
		case insideTmux && insideHerdr:
			return nil, fmt.Errorf("both tmux and herdr sessions were detected, and nested multiplexers are indistinguishable; pin %q or %q in your config", "multiplexer: tmux", "multiplexer: herdr")
		case insideHerdr:
			return NewHerdr(), nil
		case insideTmux:
			return NewTmux(), nil
		default:
			return nil, fmt.Errorf("must be run inside a supported multiplexer (tmux or herdr)")
		}
	default:
		return nil, fmt.Errorf("invalid multiplexer %q (must be %q, %q, or %q)", value, "tmux", "herdr", "auto")
	}
}

// SelectFromEnv selects the backend for a resolved multiplexer config value
// using each backend's real detection gate.
func SelectFromEnv(value string) (Multiplexer, error) {
	return Select(value, NewTmux().Detect(), NewHerdr().Detect())
}
