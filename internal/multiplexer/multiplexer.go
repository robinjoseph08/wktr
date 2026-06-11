// Package multiplexer defines the Multiplexer abstraction from ADR-0001 and
// the named backends that implement it. A Multiplexer is the terminal program
// (tmux or herdr) that hosts Windows for Tasks.
package multiplexer

import "github.com/robinjoseph08/wktr/internal/config"

// Multiplexer hosts Windows for Tasks. All backend-specific complexity
// (target addressing, size math, command sending) lives inside the backend;
// callers address Windows only by Task name.
type Multiplexer interface {
	// Detect reports whether wktr is currently running inside this
	// Multiplexer.
	Detect() bool
	// OpenWindow opens a new Window named name rooted at dir and applies
	// the Layout: pane splits, run and prime commands, and focus. Not
	// every backend applies the full Layout yet; the herdr backend
	// currently opens the single default Pane only (see Herdr).
	OpenWindow(name, dir string, layout config.Layout) error
	// FocusWindow switches to the existing Window named name.
	FocusWindow(name string) error
	// WindowExists reports whether a Window named name exists.
	WindowExists(name string) bool
	// KillWindow closes the Window named name. It is best-effort: a missing
	// Window or a failed kill is not an error.
	KillWindow(name string)
}
