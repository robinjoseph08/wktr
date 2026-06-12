package workspace

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/robinjoseph08/wktr/internal/config"
	"github.com/robinjoseph08/wktr/internal/git"
	"github.com/robinjoseph08/wktr/internal/multiplexer"
)

type openedWindow struct {
	name   string
	dir    string
	layout config.Layout
}

// fakeMultiplexer implements multiplexer.Multiplexer and records every call
// so orchestration tests can assert on what the workspace layer asked for.
// openErr and focusErr, when set, are returned from the corresponding calls;
// the call is still recorded, but a failed OpenWindow does not register the
// Window as existing. Detect reports detect, which defaults to false so tests
// represent the common case of a backend wktr is not inside; a Detect-gated
// Remove or List would therefore skip these fakes and fail the fan-out tests.
// killLog, when set, records this fake's label on every kill so tests can
// assert on kill order across backends.
type fakeMultiplexer struct {
	windows  map[string]bool
	opened   []openedWindow
	focused  []string
	killed   []string
	openErr  error
	focusErr error
	detect   bool
	label    string
	killLog  *[]string
}

func newFakeMultiplexer() *fakeMultiplexer {
	return &fakeMultiplexer{windows: map[string]bool{}}
}

func (f *fakeMultiplexer) Detect() bool {
	return f.detect
}

func (f *fakeMultiplexer) OpenWindow(name, dir string, layout config.Layout) error {
	f.opened = append(f.opened, openedWindow{name: name, dir: dir, layout: layout})
	if f.openErr != nil {
		return f.openErr
	}
	f.windows[name] = true
	return nil
}

func (f *fakeMultiplexer) FocusWindow(name string) error {
	f.focused = append(f.focused, name)
	if f.focusErr != nil {
		return f.focusErr
	}
	// Mirror the real backend, which errors when the Window does not exist.
	if !f.windows[name] {
		return fmt.Errorf("window %q not found", name)
	}
	return nil
}

func (f *fakeMultiplexer) WindowExists(name string) bool {
	return f.windows[name]
}

func (f *fakeMultiplexer) KillWindow(name string) {
	f.killed = append(f.killed, name)
	if f.killLog != nil {
		*f.killLog = append(*f.killLog, f.label)
	}
	delete(f.windows, name)
}

// selectorFor is a MultiplexerSelector that always picks the given backend,
// standing in for selection in tests that exercise orchestration alone.
func selectorFor(mux multiplexer.Multiplexer) MultiplexerSelector {
	return func(value string) (multiplexer.Multiplexer, error) {
		return mux, nil
	}
}

// initOrchestrationRepo creates a git repo with an origin remote for
// testorg/testrepo and points HOME at a temp dir so config and worktrees stay
// isolated. It returns the repo dir and the worktree base dir.
func initOrchestrationRepo(t *testing.T) (string, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	// Keep the developer's real git config (e.g. commit.gpgsign or anything
	// under XDG_CONFIG_HOME or the system gitconfig) from leaking into the
	// test repos.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	repo := t.TempDir()
	commands := [][]string{
		{"git", "init", repo},
		{"git", "-C", repo, "config", "user.email", "test@test.com"},
		{"git", "-C", repo, "config", "user.name", "Test"},
		{"git", "-C", repo, "remote", "add", "origin", "https://github.com/testorg/testrepo.git"},
		{"git", "-C", repo, "commit", "--allow-empty", "-m", "initial commit"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %s", args, out)
		}
	}

	return repo, filepath.Join(home, ".worktrees")
}

func TestCreateOutsideBothMultiplexers(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	t.Setenv("TMUX", "")
	t.Setenv("HERDR_ENV", "")

	err := Create(multiplexer.SelectFromEnv, CreateOpts{Name: "my-task", Dir: repo})
	if err == nil || !strings.Contains(err.Error(), "tmux") || !strings.Contains(err.Error(), "herdr") {
		t.Fatalf("expected error naming both multiplexers, got %v", err)
	}
	// Selection fails before any worktree is created.
	wantDir := filepath.Join(worktreeBase, "testorg", "testrepo", "my-task")
	if _, statErr := os.Stat(wantDir); !os.IsNotExist(statErr) {
		t.Errorf("expected no worktree dir, stat err: %v", statErr)
	}
}

func TestCreateErrorsWhenBothMultiplexersDetected(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	t.Setenv("TMUX", "/tmp/tmux-501/default,1234,0")
	t.Setenv("HERDR_ENV", "1")

	// "pin" is unique to the ambiguity message; the neither-detected error
	// also mentions "multiplexer", so a looser match could pass on the
	// wrong failure mode.
	err := Create(multiplexer.SelectFromEnv, CreateOpts{Name: "my-task", Dir: repo})
	if err == nil || !strings.Contains(err.Error(), "pin") {
		t.Fatalf("expected error telling the user to pin multiplexer, got %v", err)
	}
	// Selection fails before any worktree is created.
	wantDir := filepath.Join(worktreeBase, "testorg", "testrepo", "my-task")
	if _, statErr := os.Stat(wantDir); !os.IsNotExist(statErr) {
		t.Errorf("expected no worktree dir, stat err: %v", statErr)
	}
}

func TestCreatePassesResolvedMultiplexerToSelector(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	repoYAML := "multiplexer: herdr\n"
	if err := os.WriteFile(filepath.Join(repo, ".wktr.yaml"), []byte(repoYAML), 0o644); err != nil {
		t.Fatalf("failed to write .wktr.yaml: %v", err)
	}

	mux := newFakeMultiplexer()
	var gotValue string
	selector := func(value string) (multiplexer.Multiplexer, error) {
		gotValue = value
		return mux, nil
	}

	if err := Create(selector, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotValue != "herdr" {
		t.Errorf("expected resolved multiplexer %q passed to selector, got %q", "herdr", gotValue)
	}
}

func TestCreateDefaultsSelectorValueToAuto(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)

	mux := newFakeMultiplexer()
	var gotValue string
	selector := func(value string) (multiplexer.Multiplexer, error) {
		gotValue = value
		return mux, nil
	}

	if err := Create(selector, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotValue != "auto" {
		t.Errorf("expected default multiplexer %q passed to selector, got %q", "auto", gotValue)
	}
}

func TestCreateOpensWindowWithLayout(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	wantDir := filepath.Join(worktreeBase, "testorg", "testrepo", "my-task")
	if len(mux.opened) != 1 {
		t.Fatalf("expected 1 window opened, got %d", len(mux.opened))
	}
	got := mux.opened[0]
	if got.name != "my-task" {
		t.Errorf("window name: got %q, want %q", got.name, "my-task")
	}
	if got.dir != wantDir {
		t.Errorf("window dir: got %q, want %q", got.dir, wantDir)
	}
	if !reflect.DeepEqual(got.layout, config.DefaultLayout()) {
		t.Errorf("window layout: got %+v, want default layout", got.layout)
	}

	if !git.BranchExists(repo, "wktr/my-task") {
		t.Error("expected branch wktr/my-task to exist")
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Errorf("expected worktree dir to exist: %v", err)
	}
}

func TestResumeOutsideBothMultiplexers(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	t.Setenv("TMUX", "")
	t.Setenv("HERDR_ENV", "")

	err := Resume(multiplexer.SelectFromEnv, ResumeOpts{Name: "my-task", Dir: repo})
	if err == nil || !strings.Contains(err.Error(), "tmux") || !strings.Contains(err.Error(), "herdr") {
		t.Fatalf("expected error naming both multiplexers, got %v", err)
	}
}

func TestResumeErrorsWhenBothMultiplexersDetected(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	t.Setenv("TMUX", "/tmp/tmux-501/default,1234,0")
	t.Setenv("HERDR_ENV", "1")

	// "pin" is unique to the ambiguity message; the neither-detected error
	// also mentions "multiplexer", so a looser match could pass on the
	// wrong failure mode.
	err := Resume(multiplexer.SelectFromEnv, ResumeOpts{Name: "my-task", Dir: repo})
	if err == nil || !strings.Contains(err.Error(), "pin") {
		t.Fatalf("expected error telling the user to pin multiplexer, got %v", err)
	}
}

func TestResumePassesResolvedMultiplexerToSelector(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	repoYAML := "multiplexer: herdr\n"
	if err := os.WriteFile(filepath.Join(repo, ".wktr.yaml"), []byte(repoYAML), 0o644); err != nil {
		t.Fatalf("failed to write .wktr.yaml: %v", err)
	}

	mux := newFakeMultiplexer()
	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var gotValue string
	selector := func(value string) (multiplexer.Multiplexer, error) {
		gotValue = value
		return mux, nil
	}

	if err := Resume(selector, ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if gotValue != "herdr" {
		t.Errorf("expected resolved multiplexer %q passed to selector, got %q", "herdr", gotValue)
	}
}

func TestResumeDefaultsSelectorValueToAuto(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)

	mux := newFakeMultiplexer()
	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var gotValue string
	selector := func(value string) (multiplexer.Multiplexer, error) {
		gotValue = value
		return mux, nil
	}

	if err := Resume(selector, ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if gotValue != "auto" {
		t.Errorf("expected default multiplexer %q passed to selector, got %q", "auto", gotValue)
	}
}

func TestCreateReturnsOpenWindowError(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()
	mux.openErr = errors.New("open failed")

	err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo})
	if !errors.Is(err, mux.openErr) {
		t.Fatalf("expected OpenWindow error to propagate, got %v", err)
	}
}

func TestResumeFocusesExistingWindow(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Resume(selectorFor(mux), ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if len(mux.opened) != 1 {
		t.Errorf("expected no additional window opened, got %d total", len(mux.opened))
	}
	if !reflect.DeepEqual(mux.focused, []string{"my-task"}) {
		t.Errorf("expected window %q focused, got %v", "my-task", mux.focused)
	}
}

func TestResumeReturnsFocusWindowError(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mux.focusErr = errors.New("focus failed")
	err := Resume(selectorFor(mux), ResumeOpts{Name: "my-task", Dir: repo})
	if !errors.Is(err, mux.focusErr) {
		t.Fatalf("expected FocusWindow error to propagate, got %v", err)
	}
}

func TestResumeOpensWindowWhenNoneExists(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate the Window being closed, e.g. the Multiplexer session ended.
	delete(mux.windows, "my-task")

	if err := Resume(selectorFor(mux), ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if len(mux.focused) != 0 {
		t.Errorf("expected no window focused, got %v", mux.focused)
	}
	if len(mux.opened) != 2 {
		t.Fatalf("expected a second window opened, got %d total", len(mux.opened))
	}
	got := mux.opened[1]
	wantDir := filepath.Join(worktreeBase, "testorg", "testrepo", "my-task")
	if got.name != "my-task" || got.dir != wantDir {
		t.Errorf("got window %q at %q, want %q at %q", got.name, got.dir, "my-task", wantDir)
	}
	if !reflect.DeepEqual(got.layout, config.DefaultLayout()) {
		t.Errorf("window layout: got %+v, want default layout", got.layout)
	}
}

func TestResumeOpensFreshWindowWhenTaskOpenOnlyInOtherMultiplexer(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	current := newFakeMultiplexer()
	other := newFakeMultiplexer()

	if err := Create(selectorFor(other), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The Task's Window lives only in the other Multiplexer. Resume acts on
	// the current one alone (ADR-0002), so it opens a fresh Window here
	// instead of trying to focus the Window over there.
	if err := Resume(selectorFor(current), ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if len(current.opened) != 1 {
		t.Fatalf("expected 1 window opened in current multiplexer, got %d", len(current.opened))
	}
	got := current.opened[0]
	wantDir := filepath.Join(worktreeBase, "testorg", "testrepo", "my-task")
	if got.name != "my-task" || got.dir != wantDir {
		t.Errorf("got window %q at %q, want %q at %q", got.name, got.dir, "my-task", wantDir)
	}
	if !reflect.DeepEqual(got.layout, config.DefaultLayout()) {
		t.Errorf("window layout: got %+v, want default layout", got.layout)
	}
	if len(current.focused) != 0 {
		t.Errorf("expected no window focused in current multiplexer, got %v", current.focused)
	}

	// The other Multiplexer's Window keeps running untouched.
	if !other.windows["my-task"] {
		t.Error("expected the other multiplexer to keep its window")
	}
	if len(other.opened) != 1 || len(other.focused) != 0 || len(other.killed) != 0 {
		t.Errorf("expected the other multiplexer untouched by resume, got opened %d focused %v killed %v",
			len(other.opened), other.focused, other.killed)
	}
}

func TestResumeFocusesCurrentWindowWhenTaskOpenInBothMultiplexers(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	current := newFakeMultiplexer()
	other := newFakeMultiplexer()

	if err := Create(selectorFor(current), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	other.windows["my-task"] = true

	if err := Resume(selectorFor(current), ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if !reflect.DeepEqual(current.focused, []string{"my-task"}) {
		t.Errorf("expected window %q focused in current multiplexer, got %v", "my-task", current.focused)
	}
	if len(current.opened) != 1 {
		t.Errorf("expected no additional window opened, got %d total", len(current.opened))
	}
	if len(other.focused) != 0 || len(other.opened) != 0 || len(other.killed) != 0 {
		t.Errorf("expected the other multiplexer untouched by resume, got opened %d focused %v killed %v",
			len(other.opened), other.focused, other.killed)
	}
}

func TestRemoveKillsWindowAndDeletesWorktree(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Remove([]multiplexer.Multiplexer{mux}, RemoveOpts{Name: "my-task", Force: true, Dir: repo}); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if !reflect.DeepEqual(mux.killed, []string{"my-task"}) {
		t.Errorf("expected window %q killed, got %v", "my-task", mux.killed)
	}
	if git.BranchExists(repo, "wktr/my-task") {
		t.Error("expected branch wktr/my-task to be deleted")
	}
	worktreeDir := filepath.Join(worktreeBase, "testorg", "testrepo", "my-task")
	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Errorf("expected worktree dir to be removed, stat err: %v", err)
	}
}

func TestRemoveKillsWindowInEveryMultiplexer(t *testing.T) {
	tests := []struct {
		name    string
		inTmux  bool
		inHerdr bool
	}{
		{"window in neither multiplexer", false, false},
		{"window in tmux only", true, false},
		{"window in herdr only", false, true},
		{"window in both multiplexers", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := initOrchestrationRepo(t)
			// Remove never resolves the multiplexer setting (ADR-0002), so it
			// must succeed outside both Multiplexers and never hit selection
			// errors. The env is cleared and both fakes report not being the
			// current Multiplexer, so a detection-gated kill would fail here.
			t.Setenv("TMUX", "")
			t.Setenv("HERDR_ENV", "")

			tmuxMux := newFakeMultiplexer()
			herdrMux := newFakeMultiplexer()
			if err := Create(selectorFor(tmuxMux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			delete(tmuxMux.windows, "my-task")
			if tt.inTmux {
				tmuxMux.windows["my-task"] = true
			}
			if tt.inHerdr {
				herdrMux.windows["my-task"] = true
			}

			err := Remove([]multiplexer.Multiplexer{tmuxMux, herdrMux}, RemoveOpts{Name: "my-task", Force: true, Dir: repo})
			if err != nil {
				t.Fatalf("Remove: %v", err)
			}

			// The kill fans out best-effort: every backend is asked to close
			// the Window whether or not it has one, and an absent Window is
			// silently ignored.
			if !reflect.DeepEqual(tmuxMux.killed, []string{"my-task"}) {
				t.Errorf("tmux: expected window %q killed, got %v", "my-task", tmuxMux.killed)
			}
			if !reflect.DeepEqual(herdrMux.killed, []string{"my-task"}) {
				t.Errorf("herdr: expected window %q killed, got %v", "my-task", herdrMux.killed)
			}
			if tmuxMux.windows["my-task"] || herdrMux.windows["my-task"] {
				t.Error("expected no multiplexer to retain the window")
			}
		})
	}
}

func TestRemoveKillsCurrentMultiplexerWindowLast(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)

	// Killing the current Multiplexer's Window can take wktr down with it
	// (when wktr runs inside the Task's own Window), so the other backend's
	// Window must already be gone by then or it would be orphaned.
	var killLog []string
	current := newFakeMultiplexer()
	current.detect = true
	current.label = "current"
	current.killLog = &killLog
	other := newFakeMultiplexer()
	other.label = "other"
	other.killLog = &killLog

	if err := Create(selectorFor(current), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	other.windows["my-task"] = true

	// The current Multiplexer comes first in muxes, mirroring the order
	// multiplexer.All() yields when wktr runs inside tmux.
	err := Remove([]multiplexer.Multiplexer{current, other}, RemoveOpts{Name: "my-task", Force: true, Dir: repo})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if !reflect.DeepEqual(killLog, []string{"other", "current"}) {
		t.Errorf("kill order: got %v, want the other multiplexer killed before the current one", killLog)
	}
}

func TestRemoveKillsNestedMultiplexerWindowsInReverseOrder(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)

	// Nested Multiplexers both detect as current because one inherits the
	// other's env signal. wktr cannot tell which one truly hosts it, but
	// killing a Task's herdr tab never reaches a process inside a tmux pane,
	// so the herdr kill (last in All() order) must run first.
	var killLog []string
	tmuxMux := newFakeMultiplexer()
	tmuxMux.detect = true
	tmuxMux.label = "tmux"
	tmuxMux.killLog = &killLog
	herdrMux := newFakeMultiplexer()
	herdrMux.detect = true
	herdrMux.label = "herdr"
	herdrMux.killLog = &killLog

	if err := Create(selectorFor(tmuxMux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	herdrMux.windows["my-task"] = true

	err := Remove([]multiplexer.Multiplexer{tmuxMux, herdrMux}, RemoveOpts{Name: "my-task", Force: true, Dir: repo})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if !reflect.DeepEqual(killLog, []string{"herdr", "tmux"}) {
		t.Errorf("kill order: got %v, want herdr killed before tmux when both are current", killLog)
	}
}

func TestListReportsWindowsFromMultiplexer(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "task-open", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Create(selectorFor(mux), CreateOpts{Name: "task-closed", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	delete(mux.windows, "task-closed")

	infos, err := List([]multiplexer.Multiplexer{mux}, ListOpts{Dir: repo})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	hasWindow := map[string]bool{}
	for _, info := range infos {
		hasWindow[info.Name] = info.HasWindow
	}
	want := map[string]bool{"task-open": true, "task-closed": false}
	if !reflect.DeepEqual(hasWindow, want) {
		t.Errorf("got %v, want %v", hasWindow, want)
	}
}

func TestListReportsWindowOpenInAnyMultiplexer(t *testing.T) {
	tests := []struct {
		name    string
		inTmux  bool
		inHerdr bool
		want    bool
	}{
		{"window in neither multiplexer", false, false, false},
		{"window in tmux only", true, false, true},
		{"window in herdr only", false, true, true},
		{"window in both multiplexers", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := initOrchestrationRepo(t)
			// List never resolves the multiplexer setting (ADR-0002), so it
			// must succeed outside both Multiplexers and never hit selection
			// errors. The env is cleared and both fakes report not being the
			// current Multiplexer, so a detection-gated probe would fail here.
			t.Setenv("TMUX", "")
			t.Setenv("HERDR_ENV", "")

			tmuxMux := newFakeMultiplexer()
			herdrMux := newFakeMultiplexer()
			if err := Create(selectorFor(tmuxMux), CreateOpts{Name: "my-task", Dir: repo}); err != nil {
				t.Fatalf("Create: %v", err)
			}
			delete(tmuxMux.windows, "my-task")
			if tt.inTmux {
				tmuxMux.windows["my-task"] = true
			}
			if tt.inHerdr {
				herdrMux.windows["my-task"] = true
			}

			infos, err := List([]multiplexer.Multiplexer{tmuxMux, herdrMux}, ListOpts{Dir: repo})
			if err != nil {
				t.Fatalf("List: %v", err)
			}

			if len(infos) != 1 {
				t.Fatalf("expected 1 worktree, got %d", len(infos))
			}
			if infos[0].HasWindow != tt.want {
				t.Errorf("HasWindow: got %v, want %v", infos[0].HasWindow, tt.want)
			}
		})
	}
}

func TestListAllReportsWindowsFromMultiplexer(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()
	other := newFakeMultiplexer()

	if err := Create(selectorFor(mux), CreateOpts{Name: "task-open", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Create(selectorFor(mux), CreateOpts{Name: "task-closed", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The only open Window lives in the second backend, so list --all must
	// fan out over every backend (ADR-0002) to report it.
	delete(mux.windows, "task-open")
	delete(mux.windows, "task-closed")
	other.windows["task-open"] = true

	infos, err := List([]multiplexer.Multiplexer{mux, other}, ListOpts{All: true, Dir: repo})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	hasWindow := map[string]bool{}
	for _, info := range infos {
		if info.OrgRepo != "testorg/testrepo" {
			t.Errorf("unexpected org/repo %q for task %q", info.OrgRepo, info.Name)
		}
		hasWindow[info.Name] = info.HasWindow
	}
	want := map[string]bool{"task-open": true, "task-closed": false}
	if !reflect.DeepEqual(hasWindow, want) {
		t.Errorf("got %v, want %v", hasWindow, want)
	}
}
