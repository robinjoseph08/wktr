package multiplexer

import (
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
)

// Layout math and command shaping shared by every backend. Backends differ in
// how they express a split's size (tmux in absolute lines, herdr as a ratio),
// but they normalize a Layout's configured percentages identically.

// normalizePercentages resolves each Pane's configured percentage size. Panes
// without an explicit size share the remaining percentage evenly.
func normalizePercentages(panes []config.Pane) []int {
	percents := make([]int, len(panes))

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
			percents[i] = p.Size
		} else {
			percents[i] = defaultSize
		}
	}

	return percents
}

// buildChainedCommand collapses a Pane's command list into a single run
// command (chained with &&) and at most one prime command left typed but not
// executed. Blank values are dropped so a stray empty entry cannot produce a
// malformed chain (a leading or trailing && is a shell syntax error).
func buildChainedCommand(commands []config.Command) (string, string) {
	var runCmds []string
	var primeCmd string

	for _, c := range commands {
		if strings.TrimSpace(c.Value) == "" {
			continue
		}
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

// focusIndex returns the index of the first Pane marked focus, defaulting to
// the first Pane.
func focusIndex(panes []config.Pane) int {
	for i, pane := range panes {
		if pane.Focus {
			return i
		}
	}
	return 0
}
