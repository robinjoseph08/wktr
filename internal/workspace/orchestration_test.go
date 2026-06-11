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
// Window as existing.
type fakeMultiplexer struct {
	inside   bool
	windows  map[string]bool
	opened   []openedWindow
	focused  []string
	killed   []string
	openErr  error
	focusErr error
}

func newFakeMultiplexer() *fakeMultiplexer {
	return &fakeMultiplexer{inside: true, windows: map[string]bool{}}
}

func (f *fakeMultiplexer) Detect() bool {
	return f.inside
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
	delete(f.windows, name)
}

// initOrchestrationRepo creates a git repo with an origin remote for
// testorg/testrepo and points HOME at a temp dir so config and worktrees stay
// isolated. It returns the repo dir and the worktree base dir.
func initOrchestrationRepo(t *testing.T) (string, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

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

func TestCreateOutsideMultiplexer(t *testing.T) {
	mux := newFakeMultiplexer()
	mux.inside = false

	err := Create(mux, CreateOpts{Name: "my-task"})
	if err == nil || !strings.Contains(err.Error(), "must be run inside") {
		t.Fatalf("expected inside-session error, got %v", err)
	}
	if len(mux.opened) != 0 {
		t.Errorf("expected no windows opened, got %v", mux.opened)
	}
}

func TestCreateOpensWindowWithLayout(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(mux, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
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

func TestResumeOutsideMultiplexer(t *testing.T) {
	mux := newFakeMultiplexer()
	mux.inside = false

	err := Resume(mux, ResumeOpts{Name: "my-task"})
	if err == nil || !strings.Contains(err.Error(), "must be run inside") {
		t.Fatalf("expected inside-session error, got %v", err)
	}
	if len(mux.opened) != 0 || len(mux.focused) != 0 {
		t.Errorf("expected no windows opened or focused, got %v and %v", mux.opened, mux.focused)
	}
}

func TestCreateReturnsOpenWindowError(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()
	mux.openErr = errors.New("open failed")

	err := Create(mux, CreateOpts{Name: "my-task", Dir: repo})
	if !errors.Is(err, mux.openErr) {
		t.Fatalf("expected OpenWindow error to propagate, got %v", err)
	}
}

func TestResumeFocusesExistingWindow(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(mux, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Resume(mux, ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
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

	if err := Create(mux, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mux.focusErr = errors.New("focus failed")
	err := Resume(mux, ResumeOpts{Name: "my-task", Dir: repo})
	if !errors.Is(err, mux.focusErr) {
		t.Fatalf("expected FocusWindow error to propagate, got %v", err)
	}
}

func TestResumeOpensWindowWhenNoneExists(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(mux, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate the Window being closed, e.g. the Multiplexer session ended.
	delete(mux.windows, "my-task")

	if err := Resume(mux, ResumeOpts{Name: "my-task", Dir: repo}); err != nil {
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

func TestRemoveKillsWindowAndDeletesWorktree(t *testing.T) {
	repo, worktreeBase := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(mux, CreateOpts{Name: "my-task", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := Remove(mux, RemoveOpts{Name: "my-task", Force: true, Dir: repo}); err != nil {
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

func TestListReportsWindowsFromMultiplexer(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(mux, CreateOpts{Name: "task-open", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Create(mux, CreateOpts{Name: "task-closed", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	delete(mux.windows, "task-closed")

	infos, err := List(mux, ListOpts{Dir: repo})
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

func TestListAllReportsWindowsFromMultiplexer(t *testing.T) {
	repo, _ := initOrchestrationRepo(t)
	mux := newFakeMultiplexer()

	if err := Create(mux, CreateOpts{Name: "task-open", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := Create(mux, CreateOpts{Name: "task-closed", Dir: repo}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	delete(mux.windows, "task-closed")

	infos, err := List(mux, ListOpts{All: true, Dir: repo})
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
