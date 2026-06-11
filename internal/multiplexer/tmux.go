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
// sizes splits in absolute lines derived from the window height, and sends
// run and prime commands via send-keys with or without Enter.
type Tmux struct{}

var _ Multiplexer = (*Tmux)(nil)

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
	return t.setupPanes(name, dir, layout)
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
		sizes := calculateSizes(panes, t.windowHeight())

		for i := 1; i < len(panes); i++ {
			splitSize := 0
			for j := i; j < len(panes); j++ {
				splitSize += sizes[j]
			}
			target := fmt.Sprintf("%s.%d", windowName, i-1)
			cmd := exec.Command("tmux", "split-window", "-d", "-v", "-l", strconv.Itoa(splitSize), "-t", target, "-c", dir)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to split pane: %s", strings.TrimSpace(string(out)))
			}
		}
	}

	for i, pane := range panes {
		target := fmt.Sprintf("%s.%d", windowName, i)
		sendPaneCommands(target, pane)
	}

	focusIdx := 0
	for i, pane := range panes {
		if pane.Focus {
			focusIdx = i
			break
		}
	}

	_ = exec.Command("tmux", "select-window", "-t", windowName).Run()
	_ = exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s.%d", windowName, focusIdx)).Run()

	return nil
}

func (t *Tmux) windowHeight() int {
	cmd := exec.Command("tmux", "display-message", "-p", "#{window_height}")
	out, err := cmd.Output()
	if err != nil {
		return 60
	}
	h, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 60
	}
	return h
}

func sendPaneCommands(target string, pane config.Pane) {
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

	if pane.Command == "" {
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

// buildChainedCommand collapses a Pane's command list into a single run
// command (chained with &&) and at most one prime command left typed but not
// executed.
func buildChainedCommand(commands []config.Command) (string, string) {
	var runCmds []string
	var primeCmd string

	for _, c := range commands {
		run := true
		if c.Run != nil {
			run = *c.Run
		}
		if run {
			runCmds = append(runCmds, c.Value)
		} else {
			primeCmd = c.Value
		}
	}

	return strings.Join(runCmds, " && "), primeCmd
}

// calculateSizes converts each Pane's percentage size into absolute lines of
// the given window height. Panes without an explicit size share the remaining
// percentage evenly.
func calculateSizes(panes []config.Pane, windowHeight int) []int {
	if len(panes) <= 1 {
		return []int{100}
	}

	sizes := make([]int, len(panes))

	specifiedTotal := 0
	unspecifiedCount := 0
	for _, p := range panes {
		if p.Size > 0 {
			specifiedTotal += p.Size
		} else {
			unspecifiedCount++
		}
	}

	remainingPercent := 100 - specifiedTotal
	defaultSize := 0
	if unspecifiedCount > 0 {
		defaultSize = remainingPercent / unspecifiedCount
	}

	for i, p := range panes {
		if p.Size > 0 {
			sizes[i] = p.Size * windowHeight / 100
		} else {
			sizes[i] = defaultSize * windowHeight / 100
		}
	}

	return sizes
}
