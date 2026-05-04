package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
)

func InTmux() bool {
	return os.Getenv("TMUX") != ""
}

func CreateWindow(name, dir string) error {
	cmd := exec.Command("tmux", "new-window", "-d", "-n", name, "-c", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create tmux window: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func SetupPanes(windowName string, dir string, layout config.Layout) error {
	panes := layout.Panes
	if len(panes) == 0 {
		return nil
	}

	sizes := calculateSizes(panes)

	for i := 1; i < len(panes); i++ {
		size := sizes[i]
		target := fmt.Sprintf("%s.0", windowName)
		cmd := exec.Command("tmux", "split-window", "-d", "-v", "-l", strconv.Itoa(size), "-t", target, "-c", dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to split pane: %s", strings.TrimSpace(string(out)))
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

	exec.Command("tmux", "select-window", "-t", windowName).Run()
	exec.Command("tmux", "select-pane", "-t", fmt.Sprintf("%s.%d", windowName, focusIdx)).Run()

	return nil
}

func KillWindow(windowName string) error {
	target := FindWindow(windowName)
	if target == "" {
		return nil
	}
	cmd := exec.Command("tmux", "kill-window", "-t", target)
	cmd.Run()
	return nil
}

func FindWindow(name string) string {
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

func WindowExists(name string) bool {
	return FindWindow(name) != ""
}

func sendPaneCommands(target string, pane config.Pane) {
	if len(pane.Commands) > 0 {
		sendMultipleCommands(target, pane.Commands)
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
		exec.Command("tmux", "send-keys", "-t", target, pane.Command, "Enter").Run()
	} else {
		exec.Command("tmux", "send-keys", "-t", target, pane.Command).Run()
	}
}

func sendMultipleCommands(target string, commands []config.Command) {
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

	if len(runCmds) > 0 {
		chained := strings.Join(runCmds, " && ")
		exec.Command("tmux", "send-keys", "-t", target, chained, "Enter").Run()
	}

	if primeCmd != "" {
		exec.Command("tmux", "send-keys", "-t", target, primeCmd).Run()
	}
}

func calculateSizes(panes []config.Pane) []int {
	if len(panes) <= 1 {
		return []int{100}
	}

	windowHeight := getWindowHeight()
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

func getWindowHeight() int {
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
