package multiplexer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/robinjoseph08/wktr/internal/config"
)

// fixture returns the recorded output of a real herdr command. The herdr
// backend is tested entirely against these recordings; no test shells out to
// a live herdr.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

// fakeHerdrCLI stands in for the herdr binary. It records every invocation
// and replays recorded fixtures keyed by the "<noun> <verb>" subcommand.
// Subcommands invoked more than once per test (successive pane splits) queue
// their outputs in order via queues; queued outputs win over outputs.
type fakeHerdrCLI struct {
	calls   [][]string
	outputs map[string][]byte
	queues  map[string][][]byte
	errs    map[string]error
}

func (f *fakeHerdrCLI) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args[:2], " ")
	if queued := f.queues[key]; len(queued) > 0 {
		f.queues[key] = queued[1:]
		return queued[0], f.errs[key]
	}
	return f.outputs[key], f.errs[key]
}

func newHerdrWithCLI(cli *fakeHerdrCLI) *Herdr {
	h := NewHerdr()
	h.run = cli.run
	return h
}

func TestHerdrDetect(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	if !NewHerdr().Detect() {
		t.Error("expected Detect to be true when HERDR_ENV=1")
	}

	t.Setenv("HERDR_ENV", "")
	if NewHerdr().Detect() {
		t.Error("expected Detect to be false when HERDR_ENV is empty")
	}

	t.Setenv("HERDR_ENV", "0")
	if NewHerdr().Detect() {
		t.Error("expected Detect to be false when HERDR_ENV=0")
	}
}

func TestHerdrOpenWindowCreatesUnfocusedTabThenFocusesIt(t *testing.T) {
	cli := &fakeHerdrCLI{outputs: map[string][]byte{
		"tab create": fixture(t, "herdr_tab_created.json"),
		"tab focus":  fixture(t, "herdr_tab_focused.json"),
	}}
	h := newHerdrWithCLI(cli)

	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", config.DefaultLayout()); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := [][]string{
		{"tab", "create", "--no-focus", "--label", "my-task", "--cwd", "/worktrees/org/repo/my-task"},
		// The tab ID comes from the creation output, never constructed.
		{"tab", "focus", "w653faa4eef9f71:2"},
	}
	if !reflect.DeepEqual(cli.calls, want) {
		t.Errorf("herdr calls:\n got %v\nwant %v", cli.calls, want)
	}
}

// TestHerdrOpenWindowAppliesFullLayout drives a three-Pane Layout with
// explicit sizes, run commands, prime commands, and a focus Pane through
// OpenWindow and asserts the exact herdr command sequence. The tab-creation
// fixture and the pane fixtures were recorded in different live sessions, so
// their IDs differ; every ID asserted below is threaded from the fixture
// output that produced it, never constructed.
func TestHerdrOpenWindowAppliesFullLayout(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_focused.json"),
			"pane focus": fixture(t, "herdr_pane_focused.json"),
			// pane run and pane send-text emit nothing on success
			// (verified against a live herdr session), so the fake's
			// default empty output is the recorded behavior.
		},
		queues: map[string][][]byte{
			"pane split": {
				fixture(t, "herdr_pane_split.json"),
				fixture(t, "herdr_pane_split_second.json"),
			},
		},
	}
	h := newHerdrWithCLI(cli)

	layout := config.Layout{
		Direction: "vertical",
		Panes: []config.Pane{
			{Size: 50, Command: "npm run dev"},
			{Size: 30, Focus: true, Commands: []config.Command{
				{Value: "npm install", Run: boolPtr(true)},
				{Value: "npm test", Run: boolPtr(true)},
				{Value: "npm start", Run: boolPtr(false)},
			}},
			{Size: 20, Command: "vim .", Run: boolPtr(false)},
		},
	}

	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := [][]string{
		{"tab", "create", "--no-focus", "--label", "my-task", "--cwd", "/worktrees/org/repo/my-task"},
		// Pane 1 splits the root Pane (ID from the creation output); the
		// top pane keeps 50/100 of the window.
		{"pane", "split", "w653faa4eef9f71-3", "--direction", "down", "--ratio", "0.5000", "--no-focus", "--cwd", "/worktrees/org/repo/my-task"},
		// Pane 2 splits Pane 1 (ID from the first split's output); the top
		// pane keeps 30/(30+20) of the remainder.
		{"pane", "split", "w65403395f73d84-3", "--direction", "down", "--ratio", "0.6000", "--no-focus", "--cwd", "/worktrees/org/repo/my-task"},
		// Run commands execute atomically; a Pane's run commands chain
		// with && and its prime command is sent as bare text.
		{"pane", "run", "w653faa4eef9f71-3", "npm run dev"},
		{"pane", "run", "w65403395f73d84-3", "npm install && npm test"},
		{"pane", "send-text", "w65403395f73d84-3", "npm start"},
		{"pane", "send-text", "w65403395f73d84-6", "vim ."},
		// herdr has no focus-by-ID, so the focus Pane (index 1) is reached
		// as the down neighbor of the Pane above it.
		{"pane", "focus", "--direction", "down", "--pane", "w653faa4eef9f71-3"},
		{"tab", "focus", "w653faa4eef9f71:2"},
	}
	if !reflect.DeepEqual(cli.calls, want) {
		t.Errorf("herdr calls:\n got %v\nwant %v", cli.calls, want)
	}
}

// TestHerdrOpenWindowAppliesHorizontalLayout is the horizontal sibling of
// TestHerdrOpenWindowAppliesFullLayout: the same explicit sizes produce right
// splits with identical ratios, and the focus Pane is reached as the right
// neighbor of the Pane before it instead of the down neighbor.
func TestHerdrOpenWindowAppliesHorizontalLayout(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_focused.json"),
			"pane focus": fixture(t, "herdr_pane_focused.json"),
		},
		queues: map[string][][]byte{
			"pane split": {
				fixture(t, "herdr_pane_split.json"),
				fixture(t, "herdr_pane_split_second.json"),
			},
		},
	}
	h := newHerdrWithCLI(cli)

	layout := config.Layout{
		Direction: "horizontal",
		Panes: []config.Pane{
			{Size: 50},
			{Size: 30, Focus: true},
			{Size: 20},
		},
	}

	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := [][]string{
		{"tab", "create", "--no-focus", "--label", "my-task", "--cwd", "/worktrees/org/repo/my-task"},
		// The ratio math is direction-independent: the left pane keeps the
		// same fraction of the region a top pane would in a vertical Layout.
		{"pane", "split", "w653faa4eef9f71-3", "--direction", "right", "--ratio", "0.5000", "--no-focus", "--cwd", "/worktrees/org/repo/my-task"},
		{"pane", "split", "w65403395f73d84-3", "--direction", "right", "--ratio", "0.6000", "--no-focus", "--cwd", "/worktrees/org/repo/my-task"},
		// The focus Pane (index 1) sits beside its predecessor, not below it.
		{"pane", "focus", "--direction", "right", "--pane", "w653faa4eef9f71-3"},
		{"tab", "focus", "w653faa4eef9f71:2"},
	}
	if !reflect.DeepEqual(cli.calls, want) {
		t.Errorf("herdr calls:\n got %v\nwant %v", cli.calls, want)
	}
}

// TestHerdrOpenWindowDefaultsToDownSplits pins the direction default through
// OpenWindow: a Layout that never sets a direction splits down, exactly as it
// did before direction was honored.
func TestHerdrOpenWindowDefaultsToDownSplits(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_focused.json"),
		},
		queues: map[string][][]byte{
			"pane split": {fixture(t, "herdr_pane_split.json")},
		},
	}
	h := newHerdrWithCLI(cli)

	layout := config.Layout{Panes: []config.Pane{{}, {}}}
	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := []string{"pane", "split", "w653faa4eef9f71-3", "--direction", "down", "--ratio", "0.5000", "--no-focus", "--cwd", "/worktrees/org/repo/my-task"}
	var splitCalls [][]string
	for _, call := range cli.calls {
		if call[0] == "pane" && call[1] == "split" {
			splitCalls = append(splitCalls, call)
		}
	}
	if len(splitCalls) != 1 || !reflect.DeepEqual(splitCalls[0], want) {
		t.Errorf("split calls: got %v, want exactly one %v", splitCalls, want)
	}
}

// TestHerdrSplitDirection is the herdr sibling of TestTmuxSplitGeometry: the
// Layout direction maps onto herdr's split terms, with the unset default
// meaning vertical.
func TestHerdrSplitDirection(t *testing.T) {
	tests := []struct {
		direction string
		want      string
	}{
		{direction: "", want: "down"},
		{direction: "vertical", want: "down"},
		{direction: "horizontal", want: "right"},
	}
	for _, tt := range tests {
		if got := herdrSplitDirection(tt.direction); got != tt.want {
			t.Errorf("herdrSplitDirection(%q): got %q, want %q", tt.direction, got, tt.want)
		}
	}
}

// TestHerdrOpenWindowFocusesRootPaneWithoutExplicitFocus checks the focus
// default: with no Pane marked focus, the root Pane keeps the tab's focus, so
// no pane focus command is issued before the tab focus.
func TestHerdrOpenWindowFocusesRootPaneWithoutExplicitFocus(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_focused.json"),
		},
		queues: map[string][][]byte{
			"pane split": {fixture(t, "herdr_pane_split.json")},
		},
	}
	h := newHerdrWithCLI(cli)

	layout := config.Layout{Direction: "vertical", Panes: []config.Pane{{}, {}}}
	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	for _, call := range cli.calls {
		if call[0] == "pane" && call[1] == "focus" {
			t.Errorf("expected no pane focus call when no Pane is marked focus, got %v", cli.calls)
		}
	}
	last := cli.calls[len(cli.calls)-1]
	if !reflect.DeepEqual(last, []string{"tab", "focus", "w653faa4eef9f71:2"}) {
		t.Errorf("expected the tab focus to close out setup, got %v", cli.calls)
	}
}

// TestHerdrOpenWindowSurfacesLayoutErrors covers the failure path of each
// Layout setup step: the error envelope's message surfaces, the
// half-assembled tab is closed, and setup halts before the tab is focused.
// The pane-not-found fixture was recorded from a failing pane run; it stands
// in for every pane command's error envelope since only the envelope shape
// matters here.
func TestHerdrOpenWindowSurfacesLayoutErrors(t *testing.T) {
	twoPanes := config.Layout{Direction: "vertical", Panes: []config.Pane{
		{Command: "npm run dev"},
		{Command: "vim .", Run: boolPtr(false), Focus: true},
	}}

	tests := []struct {
		name string
		fail string // subcommand whose invocation fails
	}{
		{name: "split failure", fail: "pane split"},
		{name: "run failure", fail: "pane run"},
		{name: "prime failure", fail: "pane send-text"},
		{name: "pane focus failure", fail: "pane focus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := &fakeHerdrCLI{
				outputs: map[string][]byte{
					"tab create": fixture(t, "herdr_tab_created.json"),
					"tab close":  fixture(t, "herdr_tab_closed.json"),
					"tab focus":  fixture(t, "herdr_tab_focused.json"),
					"pane focus": fixture(t, "herdr_pane_focused.json"),
				},
				queues: map[string][][]byte{
					"pane split": {fixture(t, "herdr_pane_split.json")},
				},
			}
			cli.outputs[tt.fail] = fixture(t, "herdr_pane_not_found.json")
			delete(cli.queues, tt.fail)
			cli.errs = map[string]error{tt.fail: errors.New("exit status 1")}
			h := newHerdrWithCLI(cli)

			err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", twoPanes)
			if err == nil || !strings.Contains(err.Error(), "pane bogus-99 not found") {
				t.Fatalf("expected herdr error message to be surfaced, got %v", err)
			}
			if !strings.Contains(err.Error(), "herdr "+tt.fail) {
				t.Errorf("expected the failing subcommand to be named, got %v", err)
			}
			closed := false
			for _, call := range cli.calls {
				if call[0] == "tab" && call[1] == "focus" {
					t.Errorf("expected no tab focus after a failed %s, got %v", tt.fail, cli.calls)
				}
				if reflect.DeepEqual(call, []string{"tab", "close", "w653faa4eef9f71:2"}) {
					closed = true
				}
			}
			if !closed {
				t.Errorf("expected the half-assembled tab to be closed after a failed %s, got %v", tt.fail, cli.calls)
			}
		})
	}
}

// TestHerdrOpenWindowFocusesDeeperFocusPane pins the focus reference for a
// focus Pane below index 1: the Pane at index i is focused as the down
// neighbor of Pane i-1, so focusing index 2 must reference the second Pane's
// ID (returned by the first split), not the root Pane's.
func TestHerdrOpenWindowFocusesDeeperFocusPane(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_focused.json"),
			"pane focus": fixture(t, "herdr_pane_focused.json"),
		},
		queues: map[string][][]byte{
			"pane split": {
				fixture(t, "herdr_pane_split.json"),
				fixture(t, "herdr_pane_split_second.json"),
			},
		},
	}
	h := newHerdrWithCLI(cli)

	layout := config.Layout{Direction: "vertical", Panes: []config.Pane{{}, {}, {Focus: true}}}
	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	want := []string{"pane", "focus", "--direction", "down", "--pane", "w65403395f73d84-3"}
	var focusCalls [][]string
	for _, call := range cli.calls {
		if call[0] == "pane" && call[1] == "focus" {
			focusCalls = append(focusCalls, call)
		}
	}
	if len(focusCalls) != 1 || !reflect.DeepEqual(focusCalls[0], want) {
		t.Errorf("pane focus calls: got %v, want exactly one %v", focusCalls, want)
	}
}

func TestHerdrOpenWindowSurfacesCreateError(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			// The fixture was recorded from a different failing command;
			// it stands in for any error envelope since only the
			// envelope shape matters here.
			"tab create": fixture(t, "herdr_tab_not_found.json"),
		},
		errs: map[string]error{
			"tab create": errors.New("exit status 1"),
		},
	}
	h := newHerdrWithCLI(cli)

	err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", config.DefaultLayout())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The message from herdr's JSON error envelope is surfaced to the user.
	if !strings.Contains(err.Error(), "tab bogus:99 not found") {
		t.Errorf("expected herdr error message to be surfaced, got: %v", err)
	}
	for _, call := range cli.calls {
		if call[1] == "focus" {
			t.Errorf("expected no focus call after a failed create, got %v", cli.calls)
		}
	}
}

func TestHerdrOpenWindowSurfacesFocusError(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_not_found.json"),
		},
		errs: map[string]error{
			"tab focus": errors.New("exit status 1"),
		},
	}
	h := newHerdrWithCLI(cli)

	err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", config.DefaultLayout())
	if err == nil || !strings.Contains(err.Error(), "tab bogus:99 not found") {
		t.Fatalf("expected herdr focus error to be surfaced, got %v", err)
	}
	// A focus failure happens after setup succeeded, so the fully built tab
	// is left in place rather than closed like a half-assembled one.
	for _, call := range cli.calls {
		if call[0] == "tab" && call[1] == "close" {
			t.Errorf("expected the fully built tab to be left in place on focus failure, got %v", cli.calls)
		}
	}
}

// TestHerdrOpenWindowSkipsBlankCommand pins the blank-drop rule on the
// single-command path: a whitespace-only command sends nothing to the Pane.
func TestHerdrOpenWindowSkipsBlankCommand(t *testing.T) {
	cli := &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab create": fixture(t, "herdr_tab_created.json"),
			"tab focus":  fixture(t, "herdr_tab_focused.json"),
		},
	}
	h := newHerdrWithCLI(cli)

	layout := config.Layout{Panes: []config.Pane{{Command: "   "}}}
	if err := h.OpenWindow("my-task", "/worktrees/org/repo/my-task", layout); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	for _, call := range cli.calls {
		if call[0] == "pane" {
			t.Errorf("expected no pane commands for a blank command, got %v", cli.calls)
		}
	}
}

// TestHerdrCommandErrorPaths covers the envelope-unwrapping fallbacks for
// output that is not a herdr JSON envelope.
func TestHerdrCommandErrorPaths(t *testing.T) {
	t.Run("run error with non-JSON output includes both", func(t *testing.T) {
		cli := &fakeHerdrCLI{
			outputs: map[string][]byte{"tab list": []byte("herdr: something broke")},
			errs:    map[string]error{"tab list": errors.New("exit status 2")},
		}
		h := newHerdrWithCLI(cli)

		_, err := h.findTab("my-task")
		if err == nil || !strings.Contains(err.Error(), "exit status 2") || !strings.Contains(err.Error(), "something broke") {
			t.Fatalf("expected run error and output detail, got %v", err)
		}
	})

	t.Run("run error with empty output has no dangling separator", func(t *testing.T) {
		runErr := errors.New("executable file not found")
		cli := &fakeHerdrCLI{errs: map[string]error{"tab list": runErr}}
		h := newHerdrWithCLI(cli)

		_, err := h.findTab("my-task")
		if !errors.Is(err, runErr) {
			t.Fatalf("expected run error to propagate, got %v", err)
		}
		if strings.HasSuffix(err.Error(), ":") || strings.HasSuffix(err.Error(), ": ") {
			t.Errorf("expected no dangling detail separator, got %q", err.Error())
		}
	})

	t.Run("non-envelope output without a run error is unexpected", func(t *testing.T) {
		cli := &fakeHerdrCLI{outputs: map[string][]byte{"tab list": []byte("not json")}}
		h := newHerdrWithCLI(cli)

		_, err := h.findTab("my-task")
		if err == nil || !strings.Contains(err.Error(), "unexpected output") {
			t.Fatalf("expected unexpected-output error, got %v", err)
		}
	})

	t.Run("envelope with neither result nor error is unexpected", func(t *testing.T) {
		cli := &fakeHerdrCLI{outputs: map[string][]byte{"tab list": []byte(`{"id":"cli:tab:list"}`)}}
		h := newHerdrWithCLI(cli)

		_, err := h.findTab("my-task")
		if err == nil || !strings.Contains(err.Error(), "unexpected output") {
			t.Fatalf("expected unexpected-output error, got %v", err)
		}
	})

	t.Run("envelope with a null result is unexpected", func(t *testing.T) {
		cli := &fakeHerdrCLI{outputs: map[string][]byte{"tab list": []byte(`{"id":"cli:tab:list","result":null}`)}}
		h := newHerdrWithCLI(cli)

		_, err := h.findTab("my-task")
		if err == nil || !strings.Contains(err.Error(), "unexpected output") {
			t.Fatalf("expected unexpected-output error, got %v", err)
		}
	})

	t.Run("envelope error without a message is unexpected, not a dangling separator", func(t *testing.T) {
		cli := &fakeHerdrCLI{outputs: map[string][]byte{"tab list": []byte(`{"id":"cli:tab:list","error":{}}`)}}
		h := newHerdrWithCLI(cli)

		_, err := h.findTab("my-task")
		if err == nil || !strings.Contains(err.Error(), "unexpected output") {
			t.Fatalf("expected unexpected-output error, got %v", err)
		}
	})
}

func TestHerdrWindowExists(t *testing.T) {
	cli := &fakeHerdrCLI{outputs: map[string][]byte{
		"tab list": fixture(t, "herdr_tab_list.json"),
	}}
	h := newHerdrWithCLI(cli)

	if !h.WindowExists("my-task") {
		t.Error("expected my-task to exist in the tab listing")
	}
	if h.WindowExists("missing-task") {
		t.Error("expected missing-task to not exist in the tab listing")
	}
}

func TestHerdrWindowExistsFalseWhenListFails(t *testing.T) {
	cli := &fakeHerdrCLI{errs: map[string]error{
		"tab list": errors.New("exit status 1"),
	}}
	h := newHerdrWithCLI(cli)

	if h.WindowExists("my-task") {
		t.Error("expected WindowExists to be false when the tab listing fails")
	}
}

func TestHerdrFocusWindowLooksUpTabByLabel(t *testing.T) {
	cli := &fakeHerdrCLI{outputs: map[string][]byte{
		"tab list":  fixture(t, "herdr_tab_list.json"),
		"tab focus": fixture(t, "herdr_tab_focused.json"),
	}}
	h := newHerdrWithCLI(cli)

	if err := h.FocusWindow("my-task"); err != nil {
		t.Fatalf("FocusWindow: %v", err)
	}

	// The tab ID is threaded from the listing output into the focus call.
	last := cli.calls[len(cli.calls)-1]
	want := []string{"tab", "focus", "w653faa4eef9f71:2"}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("focus call: got %v, want %v", last, want)
	}
}

func TestHerdrFocusWindowErrorsWhenTabMissing(t *testing.T) {
	cli := &fakeHerdrCLI{outputs: map[string][]byte{
		"tab list": fixture(t, "herdr_tab_list.json"),
	}}
	h := newHerdrWithCLI(cli)

	err := h.FocusWindow("missing-task")
	if err == nil || !strings.Contains(err.Error(), "missing-task") {
		t.Fatalf("expected not-found error naming the window, got %v", err)
	}
	for _, call := range cli.calls {
		if call[1] == "focus" {
			t.Errorf("expected no focus call for a missing tab, got %v", cli.calls)
		}
	}
}

func TestHerdrKillWindowClosesTabByLabel(t *testing.T) {
	cli := &fakeHerdrCLI{outputs: map[string][]byte{
		"tab list":  fixture(t, "herdr_tab_list.json"),
		"tab close": fixture(t, "herdr_tab_closed.json"),
	}}
	h := newHerdrWithCLI(cli)

	h.KillWindow("my-task")

	last := cli.calls[len(cli.calls)-1]
	want := []string{"tab", "close", "w653faa4eef9f71:2"}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("close call: got %v, want %v", last, want)
	}
}

func TestHerdrKillWindowIsBestEffort(t *testing.T) {
	// A missing tab issues no close call and does not panic.
	cli := &fakeHerdrCLI{outputs: map[string][]byte{
		"tab list": fixture(t, "herdr_tab_list.json"),
	}}
	h := newHerdrWithCLI(cli)

	h.KillWindow("missing-task")

	for _, call := range cli.calls {
		if call[1] == "close" {
			t.Errorf("expected no close call for a missing tab, got %v", cli.calls)
		}
	}

	// A failing tab listing issues no close call and does not panic.
	cli = &fakeHerdrCLI{errs: map[string]error{"tab list": errors.New("exit status 1")}}
	newHerdrWithCLI(cli).KillWindow("my-task")
	for _, call := range cli.calls {
		if call[1] == "close" {
			t.Errorf("expected no close call when the listing fails, got %v", cli.calls)
		}
	}

	// A failing close is swallowed. The not-found fixture was recorded from
	// a different failing command; it stands in for any error envelope
	// since only the envelope shape matters here.
	cli = &fakeHerdrCLI{
		outputs: map[string][]byte{
			"tab list":  fixture(t, "herdr_tab_list.json"),
			"tab close": fixture(t, "herdr_tab_not_found.json"),
		},
		errs: map[string]error{
			"tab close": errors.New("exit status 1"),
		},
	}
	newHerdrWithCLI(cli).KillWindow("my-task")

	// The close was attempted and its failure swallowed, not skipped.
	last := cli.calls[len(cli.calls)-1]
	want := []string{"tab", "close", "w653faa4eef9f71:2"}
	if !reflect.DeepEqual(last, want) {
		t.Errorf("expected a close attempt, got %v", cli.calls)
	}
}

// TestHerdrSplitRatios is the ratio-based sibling of the tmux
// calculateSizes tests: both consume the same percentage normalization, but
// herdr sizes each split as the fraction of the region the top pane keeps.
func TestHerdrSplitRatios(t *testing.T) {
	tests := []struct {
		name  string
		panes []config.Pane
		want  []float64
	}{
		{
			name:  "no panes need no splits",
			panes: nil,
			want:  []float64{},
		},
		{
			name:  "single pane needs no splits",
			panes: []config.Pane{{}},
			want:  []float64{},
		},
		{
			name:  "explicit sizes",
			panes: []config.Pane{{Size: 50}, {Size: 30}, {Size: 20}},
			// Split 1 leaves pane 0 with 50/100 of the window; split 2
			// leaves pane 1 with 30/(30+20) of the remainder.
			want: []float64{0.5, 0.6},
		},
		{
			name:  "even distribution",
			panes: []config.Pane{{}, {}, {}},
			// 3 panes, no explicit size = 33% each: 33/99, then 33/66.
			want: []float64{1.0 / 3.0, 0.5},
		},
		{
			name:  "explicit size with unspecified remainder",
			panes: []config.Pane{{Size: 50}, {}, {}},
			// The unspecified panes share the remaining 50% evenly.
			want: []float64{0.5, 0.5},
		},
		{
			name:  "zero-percent tail falls back to an even split",
			panes: []config.Pane{{Size: 100}, {}, {}},
			// Panes 1 and 2 normalize to 0%, so their split has no
			// percentages left to divide and falls back to an even split.
			want: []float64{1.0, 0.5},
		},
		{
			name:  "deeper zero-percent tail divides evenly among what remains",
			panes: []config.Pane{{Size: 100}, {}, {}, {}},
			// Each fallback split divides the leftover region evenly among
			// the Panes that remain: three ways, then two.
			want: []float64{1.0, 1.0 / 3.0, 0.5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitRatios(tt.panes)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d ratios, got %v", len(tt.want), got)
			}
			for i := range tt.want {
				if diff := got[i] - tt.want[i]; diff > 1e-9 || diff < -1e-9 {
					t.Errorf("ratio %d: got %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// resultOf unwraps the JSON envelope around a recorded herdr response,
// mirroring what the backend does before parsing a payload.
func resultOf(t *testing.T, raw []byte) json.RawMessage {
	t.Helper()
	var envelope herdrEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("failed to unwrap herdr envelope: %v", err)
	}
	return envelope.Result
}

func TestHerdrTabCreationParsesOpaqueIDs(t *testing.T) {
	created, err := parseTabCreated(resultOf(t, fixture(t, "herdr_tab_created.json")))
	if err != nil {
		t.Fatalf("parseTabCreated: %v", err)
	}
	if created.Tab.TabID != "w653faa4eef9f71:2" {
		t.Errorf("tab ID: got %q, want %q", created.Tab.TabID, "w653faa4eef9f71:2")
	}
	if created.Tab.Label != "my-task" {
		t.Errorf("tab label: got %q, want %q", created.Tab.Label, "my-task")
	}
	if created.RootPane.PaneID != "w653faa4eef9f71-3" {
		t.Errorf("root pane ID: got %q, want %q", created.RootPane.PaneID, "w653faa4eef9f71-3")
	}
}

func TestHerdrTabCreationErrorsWithoutTabID(t *testing.T) {
	_, err := parseTabCreated([]byte(`{"tab":{"label":"my-task"}}`))
	if err == nil || !strings.Contains(err.Error(), "tab ID") {
		t.Fatalf("expected missing tab ID error, got %v", err)
	}
}

func TestHerdrTabCreationErrorsWithoutRootPaneID(t *testing.T) {
	_, err := parseTabCreated([]byte(`{"tab":{"tab_id":"w653faa4eef9f71:2","label":"my-task"}}`))
	if err == nil || !strings.Contains(err.Error(), "root pane ID") {
		t.Fatalf("expected missing root pane ID error, got %v", err)
	}
}

func TestHerdrPaneSplitParsesOpaqueID(t *testing.T) {
	paneID, err := parsePaneSplit(resultOf(t, fixture(t, "herdr_pane_split.json")))
	if err != nil {
		t.Fatalf("parsePaneSplit: %v", err)
	}
	if paneID != "w65403395f73d84-3" {
		t.Errorf("pane ID: got %q, want %q", paneID, "w65403395f73d84-3")
	}
}

func TestHerdrPaneSplitErrorsWithoutPaneID(t *testing.T) {
	_, err := parsePaneSplit([]byte(`{"pane":{"focused":false}}`))
	if err == nil || !strings.Contains(err.Error(), "pane ID") {
		t.Fatalf("expected missing pane ID error, got %v", err)
	}
}

func TestHerdrTabListParsesTabs(t *testing.T) {
	tabs, err := parseTabList(resultOf(t, fixture(t, "herdr_tab_list.json")))
	if err != nil {
		t.Fatalf("parseTabList: %v", err)
	}
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(tabs))
	}
	if tabs[1].Label != "my-task" || tabs[1].TabID != "w653faa4eef9f71:2" {
		t.Errorf("tab 1: got %+v, want label my-task with ID w653faa4eef9f71:2", tabs[1])
	}
}
