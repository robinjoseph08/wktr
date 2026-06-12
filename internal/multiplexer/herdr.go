package multiplexer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
)

// Herdr is the herdr backend. A Task's Window maps to a herdr tab labeled
// with the Task name. herdr addresses tabs and panes by opaque IDs emitted as
// JSON, so the backend threads IDs from command output to command input and
// never constructs them. Tabs are created in whatever workspace the user is
// in; wktr never creates or manages herdr workspaces. Splits are sized by
// ratio rather than tmux's absolute lines, run commands use herdr's atomic
// run operation, and prime commands use its send-text operation.
type Herdr struct {
	// run invokes the herdr CLI and returns the JSON envelope it emitted.
	// Tests replace it to replay recorded fixtures.
	run func(args ...string) ([]byte, error)
}

var _ Multiplexer = (*Herdr)(nil)

// NewHerdr returns the herdr backend.
func NewHerdr() *Herdr {
	return &Herdr{run: runHerdr}
}

func (h *Herdr) Detect() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

func (h *Herdr) OpenWindow(name, dir string, layout config.Layout) error {
	// Errors from the herdr helpers already identify the herdr subcommand
	// that failed, so they propagate unwrapped.
	created, err := h.createTab(name, dir)
	if err != nil {
		return err
	}
	// The tab is created without focus and focused only after setup, so the
	// user never watches the Layout assemble.
	if err := h.setupPanes(created.RootPane.PaneID, dir, layout); err != nil {
		// Close the half-assembled tab so a setup failure does not strand
		// an unfocused tab the user has to hunt down. Best effort: the
		// setup error is the one worth reporting. A focus failure below
		// leaves the tab in place, since by then it is fully built and
		// usable.
		_, _ = h.command("tab", "close", created.Tab.TabID)
		return err
	}
	return h.focusTab(created.Tab.TabID)
}

// setupPanes applies the Layout inside a freshly created tab: it splits the
// root Pane into the configured stack along the Layout direction, sends each
// Pane's run and prime commands, and focuses the configured Pane.
func (h *Herdr) setupPanes(rootPaneID, dir string, layout config.Layout) error {
	panes := layout.Panes
	if len(panes) == 0 {
		return nil
	}

	paneIDs := make([]string, len(panes))
	paneIDs[0] = rootPaneID

	splitDirection := herdrSplitDirection(layout.Direction)
	ratios := splitRatios(panes)
	for i := 1; i < len(panes); i++ {
		// Pane i is created by splitting Pane i-1 along the Layout direction
		// (top to bottom or left to right), so each split targets the pane ID
		// returned by the previous one.
		paneID, err := h.splitPane(paneIDs[i-1], dir, splitDirection, ratios[i-1])
		if err != nil {
			return err
		}
		paneIDs[i] = paneID
	}

	for i, pane := range panes {
		if err := h.sendPaneCommands(paneIDs[i], pane); err != nil {
			return err
		}
	}

	// A tab's focused pane stays the root pane through --no-focus splits
	// (verified against a live herdr session), so only a non-root focus
	// Pane needs an explicit focus.
	if focusIdx := focusIndex(panes); focusIdx > 0 {
		return h.focusAdjacentPane(paneIDs[focusIdx-1], splitDirection)
	}
	return nil
}

// sendPaneCommands sends a Pane's configured commands: run commands execute
// via herdr's atomic run operation (a list chains with && as in tmux) and
// prime commands land as typed-but-unexecuted text via send-text.
func (h *Herdr) sendPaneCommands(paneID string, pane config.Pane) error {
	if len(pane.Commands) > 0 {
		run, prime := buildChainedCommand(pane.Commands)
		if run != "" {
			if err := h.runInPane(paneID, run); err != nil {
				return err
			}
		}
		if prime != "" {
			return h.primePane(paneID, prime)
		}
		return nil
	}

	// Blank commands are dropped on the single-command path too, matching
	// buildChainedCommand.
	if strings.TrimSpace(pane.Command) == "" {
		return nil
	}

	run := true
	if pane.Run != nil {
		run = *pane.Run
	}
	if run {
		return h.runInPane(paneID, pane.Command)
	}
	return h.primePane(paneID, pane.Command)
}

func (h *Herdr) FocusWindow(name string) error {
	tab, err := h.findTab(name)
	if err != nil {
		return err
	}
	if tab == nil {
		return fmt.Errorf("herdr tab %q not found", name)
	}
	return h.focusTab(tab.TabID)
}

func (h *Herdr) WindowExists(name string) bool {
	tab, err := h.findTab(name)
	return err == nil && tab != nil
}

func (h *Herdr) KillWindow(name string) {
	tab, err := h.findTab(name)
	if err != nil || tab == nil {
		return
	}
	_, _ = h.command("tab", "close", tab.TabID)
}

type herdrTab struct {
	TabID string `json:"tab_id"`
	Label string `json:"label"`
}

type herdrPane struct {
	PaneID string `json:"pane_id"`
}

type herdrTabCreated struct {
	Tab      herdrTab  `json:"tab"`
	RootPane herdrPane `json:"root_pane"`
}

func (h *Herdr) createTab(name, dir string) (herdrTabCreated, error) {
	result, err := h.command("tab", "create", "--no-focus", "--label", name, "--cwd", dir)
	if err != nil {
		return herdrTabCreated{}, err
	}
	return parseTabCreated(result)
}

func (h *Herdr) focusTab(tabID string) error {
	_, err := h.command("tab", "focus", tabID)
	return err
}

// herdrSplitDirection maps a Layout direction onto herdr's split terms:
// vertical Layouts (the default, including an unset direction) stack Panes
// with down splits, horizontal Layouts place them side by side with right
// splits.
func herdrSplitDirection(direction string) string {
	if isHorizontal(direction) {
		return "right"
	}
	return "down"
}

// splitPane splits the given pane along direction (down or right), the new
// pane taking the bottom or right of the region, and returns the new pane's
// ID. ratio is the fraction of the region the original pane keeps.
func (h *Herdr) splitPane(paneID, dir, direction string, ratio float64) (string, error) {
	result, err := h.command("pane", "split", paneID,
		"--direction", direction,
		"--ratio", strconv.FormatFloat(ratio, 'f', 4, 64),
		"--no-focus", "--cwd", dir)
	if err != nil {
		return "", err
	}
	return parsePaneSplit(result)
}

func (h *Herdr) runInPane(paneID, command string) error {
	return h.commandNoResult("pane", "run", paneID, command)
}

func (h *Herdr) primePane(paneID, text string) error {
	return h.commandNoResult("pane", "send-text", paneID, text)
}

// focusAdjacentPane focuses the pane next to the given pane along the split
// direction: below it in a vertical Layout, to its right in a horizontal
// one. herdr has no focus-by-ID operation, only focus relative to a
// reference pane, and the Layout's Panes are stacked along the split
// direction, so the Pane at index i is the neighbor of the Pane at index
// i-1.
func (h *Herdr) focusAdjacentPane(paneID, direction string) error {
	_, err := h.command("pane", "focus", "--direction", direction, "--pane", paneID)
	return err
}

// findTab looks a tab up by label over the tab listing. The listing spans all
// workspaces (verified against a live herdr server: an unflagged tab list
// returns tabs from every workspace, not just the focused one), so lookup by
// label finds a Window anywhere, mirroring tmux's any-session search. It
// returns nil when no tab carries the label.
func (h *Herdr) findTab(name string) (*herdrTab, error) {
	result, err := h.command("tab", "list")
	if err != nil {
		return nil, err
	}
	tabs, err := parseTabList(result)
	if err != nil {
		return nil, err
	}
	for _, tab := range tabs {
		if tab.Label == name {
			return &tab, nil
		}
	}
	return nil, nil
}

// splitRatios computes the --ratio for each split that builds the Layout's
// Pane stack: ratios[i-1] sizes the split of Pane i-1 that creates Pane i
// (zero-indexed, ordered along the Layout direction). herdr's ratio is the
// fraction of the split region kept by the original pane regardless of
// direction: the top of a down split (verified against a live herdr session)
// or the left of a right split (verified against herdr's source, whose
// split_rect sizes the original pane by ratio on both axes), so each ratio
// is Pane i-1's percentage over the percentages of
// Panes i-1 through n. When that remainder is zero (every remaining Pane
// normalized to 0%), the split falls back to dividing the region evenly.
//
// Degenerate ratios are not guarded because herdr clamps them itself
// (verified live: 0.0, 1.0, and 1.5 all split successfully and clamp to the
// 0.1..0.9 range), so the 1.0 a zero-percent tail produces yields a thin
// bottom pane rather than a failed split.
func splitRatios(panes []config.Pane) []float64 {
	// No Panes need no ratios; the guard also keeps the capacity arithmetic
	// below non-negative.
	if len(panes) == 0 {
		return nil
	}
	percents := normalizePercentages(panes)
	ratios := make([]float64, 0, len(panes)-1)
	for i := 1; i < len(panes); i++ {
		remaining := 0
		for _, p := range percents[i-1:] {
			remaining += p
		}
		if remaining <= 0 {
			ratios = append(ratios, 1.0/float64(len(panes)-i+1))
			continue
		}
		ratios = append(ratios, float64(percents[i-1])/float64(remaining))
	}
	return ratios
}

// herdrEnvelope is the JSON wrapper around every herdr CLI response: a result
// payload on success or an error object on failure. The envelope also carries
// a machine-readable error code (see the recorded fixtures); only the message
// is decoded because nothing consumes the code yet.
type herdrEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// command runs a herdr CLI command and unwraps its JSON envelope, returning
// the raw result payload or the error herdr reported. Errors identify the
// command by its noun and verb (e.g. "tab create") without the flag noise.
func (h *Herdr) command(noun, verb string, extra ...string) (json.RawMessage, error) {
	subcommand := noun + " " + verb
	args := append([]string{noun, verb}, extra...)
	out, runErr := h.run(args...)

	if runErr == nil {
		var envelope herdrEnvelope
		// Success needs a real result and no error object; anything else
		// (a null or missing result, an error with or without a message)
		// falls through to the error diagnostics.
		if err := json.Unmarshal(out, &envelope); err == nil && envelope.Error == nil && len(envelope.Result) > 0 && string(envelope.Result) != "null" {
			return envelope.Result, nil
		}
	}
	return nil, herdrCommandError(subcommand, out, runErr)
}

// commandNoResult runs a herdr CLI command whose success emits nothing: pane
// run and pane send-text exit zero with empty output (verified against a live
// herdr session) and emit the usual JSON error envelope only on failure.
func (h *Herdr) commandNoResult(noun, verb string, extra ...string) error {
	subcommand := noun + " " + verb
	args := append([]string{noun, verb}, extra...)
	out, runErr := h.run(args...)
	if runErr == nil {
		return nil
	}
	return herdrCommandError(subcommand, out, runErr)
}

// herdrCommandError shapes a failed herdr command's output into an error:
// the message from herdr's JSON error envelope when there is one, otherwise
// the run error and any non-envelope output.
func herdrCommandError(subcommand string, out []byte, runErr error) error {
	var envelope herdrEnvelope
	if err := json.Unmarshal(out, &envelope); err == nil && envelope.Error != nil && envelope.Error.Message != "" {
		return fmt.Errorf("herdr %s: %s", subcommand, envelope.Error.Message)
	}
	detail := strings.TrimSpace(string(out))
	if runErr != nil {
		if detail == "" {
			return fmt.Errorf("herdr %s: %w", subcommand, runErr)
		}
		return fmt.Errorf("herdr %s: %w: %s", subcommand, runErr, detail)
	}
	return fmt.Errorf("herdr %s: unexpected output: %q", subcommand, detail)
}

func parseTabCreated(result []byte) (herdrTabCreated, error) {
	var created herdrTabCreated
	if err := json.Unmarshal(result, &created); err != nil {
		return herdrTabCreated{}, fmt.Errorf("failed to parse herdr tab creation output: %w", err)
	}
	if created.Tab.TabID == "" {
		return herdrTabCreated{}, fmt.Errorf("herdr tab creation output did not include a tab ID")
	}
	// The root pane ID seeds the Layout setup (it is the target of the
	// first split), so a payload without one is rejected here rather than
	// surfacing as an opaque split error later.
	if created.RootPane.PaneID == "" {
		return herdrTabCreated{}, fmt.Errorf("herdr tab creation output did not include a root pane ID")
	}
	return created, nil
}

func parsePaneSplit(result []byte) (string, error) {
	var split struct {
		Pane herdrPane `json:"pane"`
	}
	if err := json.Unmarshal(result, &split); err != nil {
		return "", fmt.Errorf("failed to parse herdr pane split output: %w", err)
	}
	if split.Pane.PaneID == "" {
		return "", fmt.Errorf("herdr pane split output did not include a pane ID")
	}
	return split.Pane.PaneID, nil
}

func parseTabList(result []byte) ([]herdrTab, error) {
	var list struct {
		Tabs []herdrTab `json:"tabs"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		return nil, fmt.Errorf("failed to parse herdr tab listing: %w", err)
	}
	return list.Tabs, nil
}

// runHerdr shells out to the herdr CLI. herdr writes its JSON envelope to
// stdout on success and to stderr on failure.
func runHerdr(args ...string) ([]byte, error) {
	cmd := exec.Command("herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stderr.Bytes(), err
	}
	return stdout.Bytes(), nil
}
