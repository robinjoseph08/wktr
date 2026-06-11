package multiplexer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
)

// Herdr is the herdr backend. A Task's Window maps to a herdr tab labeled
// with the Task name. herdr addresses tabs and panes by opaque IDs emitted as
// JSON, so the backend threads IDs from command output to command input and
// never constructs them. Tabs are created in whatever workspace the user is
// in; wktr never creates or manages herdr workspaces.
//
// This slice opens the single default Pane only. Full Layout support (Pane
// splits, run and prime commands) is a follow-up.
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
	// The Layout is not applied yet: only the tab's single default Pane
	// opens. The layout parameter stays so the Multiplexer interface holds.
	_ = layout

	created, err := h.createTab(name, dir)
	if err != nil {
		return fmt.Errorf("failed to create herdr tab: %w", err)
	}
	// The tab is created without focus and focused only after setup.
	if err := h.focusTab(created.Tab.TabID); err != nil {
		return fmt.Errorf("failed to focus herdr tab: %w", err)
	}
	return nil
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

// findTab looks a tab up by label over the tab listing, which spans all
// workspaces. It returns nil when no tab carries the label.
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

// herdrEnvelope is the JSON wrapper around every herdr CLI response: a result
// payload on success or an error object on failure.
type herdrEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// command runs a herdr CLI command and unwraps its JSON envelope, returning
// the raw result payload or the error herdr reported.
func (h *Herdr) command(args ...string) (json.RawMessage, error) {
	out, runErr := h.run(args...)

	var envelope herdrEnvelope
	if err := json.Unmarshal(out, &envelope); err == nil {
		if envelope.Error != nil {
			return nil, fmt.Errorf("herdr %s: %s", strings.Join(args[:2], " "), envelope.Error.Message)
		}
		if runErr == nil {
			return envelope.Result, nil
		}
	}
	if runErr != nil {
		return nil, fmt.Errorf("herdr %s: %w: %s", strings.Join(args[:2], " "), runErr, strings.TrimSpace(string(out)))
	}
	return nil, fmt.Errorf("herdr %s: unexpected output: %s", strings.Join(args[:2], " "), strings.TrimSpace(string(out)))
}

func parseTabCreated(result []byte) (herdrTabCreated, error) {
	var created herdrTabCreated
	if err := json.Unmarshal(result, &created); err != nil {
		return herdrTabCreated{}, fmt.Errorf("failed to parse herdr tab creation output: %w", err)
	}
	if created.Tab.TabID == "" {
		return herdrTabCreated{}, fmt.Errorf("herdr tab creation output did not include a tab ID")
	}
	return created, nil
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
