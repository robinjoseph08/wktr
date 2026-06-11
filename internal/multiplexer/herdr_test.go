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
type fakeHerdrCLI struct {
	calls   [][]string
	outputs map[string][]byte
	errs    map[string]error
}

func (f *fakeHerdrCLI) run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args[:2], " ")
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
