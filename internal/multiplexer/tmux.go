package multiplexer

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
)

// Tmux is the tmux backend. It addresses Windows by name across all sessions,
// sizes splits in absolute lines or columns derived from the window dimension
// along the Layout direction, and sends run and prime commands via send-keys
// with or without Enter.
type Tmux struct{}

var _ Multiplexer = (*Tmux)(nil)

// NewTmux returns the tmux backend.
func NewTmux() *Tmux {
	return &Tmux{}
}

func (t *Tmux) Detect() bool {
	return os.Getenv("TMUX") != ""
}

func (t *Tmux) OpenWindow(name, dir string, layout config.Layout) error {
	cmd := exec.Command("tmux", "new-window", "-d", "-n", name, "-c", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create tmux window: %s", strings.TrimSpace(string(out)))
	}
	if err := t.setupPanes(name, dir, layout); err != nil {
		// Kill the half-assembled Window so a failed split does not strand
		// a detached Window. Best effort: the setup error is the one worth
		// reporting.
		t.KillWindow(name)
		return err
	}
	return nil
}

func (t *Tmux) FocusWindow(name string) error {
	target := t.findWindow(name)
	if target == "" {
		return fmt.Errorf("tmux window %q not found", name)
	}
	cmd := exec.Command("tmux", "select-window", "-t", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to select tmux window: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func (t *Tmux) WindowExists(name string) bool {
	return t.findWindow(name) != ""
}

func (t *Tmux) KillWindow(name string) {
	target := t.findWindow(name)
	if target == "" {
		return
	}
	cmd := exec.Command("tmux", "kill-window", "-t", target)
	_ = cmd.Run()
}

func (t *Tmux) findWindow(name string) string {
	cmd := exec.Command("tmux", "list-windows", "-a", "-F", "#{session_name}:#{window_name}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[1] == name {
			return line
		}
	}
	return ""
}

func (t *Tmux) setupPanes(windowName string, dir string, layout config.Layout) error {
	panes := layout.Panes
	if len(panes) == 0 {
		return nil
	}

	if len(panes) > 1 {
		geo := tmuxSplitGeometry(layout.Direction)
		sizes := t.calculateSizes(panes, t.windowDimension(geo))

		for i := 1; i < len(panes); i++ {
			splitSize := 0
			for j := i; j < len(panes); j++ {
				splitSize += sizes[j]
			}
			target := fmt.Sprintf("%s.%d", windowName, i-1)
			cmd := exec.Command("tmux", "split-window", "-d", geo.flag, "-l", strconv.Itoa(splitSize), "-t", target, "-c", dir)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to split pane: %s", strings.TrimSpace(string(out)))
			}
		}
	}

	for i, pane := range panes {
		target := fmt.Sprintf("%s.%d", windowName, i)
		t.sendPaneCommands(target, pane)
	}

	focusIdx := focusIndex(panes)

	_ = exec.Command("tmux", "select-window", "-t", windowName).Run()
	_ = exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s.%d", windowName, focusIdx)).Run()

	return nil
}

// splitGeometry maps a Layout direction onto tmux's split terms: the
// split-window direction flag, the format variable for the window dimension
// being divided, and the dimension to assume when tmux cannot report one.
type splitGeometry struct {
	flag      string
	dimension string
	fallback  int
}

// tmuxSplitGeometry resolves the geometry for a Layout direction. Vertical
// (the default, including an unset direction) stacks Panes top to bottom: -v
// splits sized in lines of the window height. Horizontal places them side by
// side: -h splits sized in columns of the window width. The fallbacks
// approximate a full-screen terminal so the percentage math still yields
// usable splits when tmux cannot be queried.
func tmuxSplitGeometry(direction string) splitGeometry {
	if isHorizontal(direction) {
		return splitGeometry{flag: "-h", dimension: "#{window_width}", fallback: 200}
	}
	return splitGeometry{flag: "-v", dimension: "#{window_height}", fallback: 60}
}

// windowDimension asks tmux for the current window's size along the
// geometry's axis, falling back to the geometry's default when tmux cannot
// be queried.
func (t *Tmux) windowDimension(geo splitGeometry) int {
	cmd := exec.Command("tmux", "display-message", "-p", geo.dimension)
	out, err := cmd.Output()
	if err != nil {
		return geo.fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return geo.fallback
	}
	return n
}

func (t *Tmux) sendPaneCommands(target string, pane config.Pane) {
	if len(pane.Commands) > 0 {
		run, prime := buildChainedCommand(pane.Commands)
		if run != "" {
			_ = exec.Command("tmux", "send-keys", "-t", target, run, "Enter").Run()
		}
		if prime != "" {
			_ = exec.Command("tmux", "send-keys", "-t", target, prime).Run()
		}
		return
	}

	// Blank commands are dropped on the single-command path too, matching
	// buildChainedCommand.
	if strings.TrimSpace(pane.Command) == "" {
		return
	}

	run := true
	if pane.Run != nil {
		run = *pane.Run
	}

	if run {
		_ = exec.Command("tmux", "send-keys", "-t", target, pane.Command, "Enter").Run()
	} else {
		_ = exec.Command("tmux", "send-keys", "-t", target, pane.Command).Run()
	}
}

// calculateSizes converts each Pane's normalized percentage size into an
// absolute share of the given window dimension: lines of the height for
// vertical Layouts, columns of the width for horizontal ones.
func (t *Tmux) calculateSizes(panes []config.Pane, windowDimension int) []int {
	percents := normalizePercentages(panes)
	sizes := make([]int, len(panes))
	for i, percent := range percents {
		sizes[i] = percent * windowDimension / 100
	}
	return sizes
}
