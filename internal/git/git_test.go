package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantOrg string
		wantRepo string
		wantErr bool
	}{
		{
			name:     "SSH format",
			url:      "git@github.com:shishobooks/shisho.git",
			wantOrg:  "shishobooks",
			wantRepo: "shisho",
		},
		{
			name:     "HTTPS format with .git",
			url:      "https://github.com/robinjoseph08/wktr.git",
			wantOrg:  "robinjoseph08",
			wantRepo: "wktr",
		},
		{
			name:     "HTTPS format without .git",
			url:      "https://github.com/robinjoseph08/wktr",
			wantOrg:  "robinjoseph08",
			wantRepo: "wktr",
		},
		{
			name:     "SSH with port",
			url:      "ssh://git@github.com:22/org/repo.git",
			wantOrg:  "org",
			wantRepo: "repo",
		},
		{
			name:    "invalid URL",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRemoteURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Org != tt.wantOrg {
				t.Errorf("org: got %q, want %q", result.Org, tt.wantOrg)
			}
			if result.Repo != tt.wantRepo {
				t.Errorf("repo: got %q, want %q", result.Repo, tt.wantRepo)
			}
		})
	}
}

func TestOrgRepoString(t *testing.T) {
	or := OrgRepo{Org: "myorg", Repo: "myrepo"}
	if or.String() != "myorg/myrepo" {
		t.Errorf("expected %q, got %q", "myorg/myrepo", or.String())
	}
}

func TestWorktreeDir(t *testing.T) {
	result := WorktreeDir("/home/user/.worktrees", OrgRepo{Org: "org", Repo: "repo"}, "my-task")
	expected := filepath.Join("/home/user/.worktrees", "org", "repo", "my-task")
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestBranchExists(t *testing.T) {
	dir := initTestRepo(t)

	if !BranchExists(dir, "master") && !BranchExists(dir, "main") {
		t.Error("expected default branch to exist")
	}

	if BranchExists(dir, "nonexistent-branch") {
		t.Error("expected nonexistent branch to not exist")
	}
}

func TestBranchExistsIgnoresTags(t *testing.T) {
	dir := initTestRepo(t)

	// Create a tag with a name that looks like a branch
	cmd := exec.Command("git", "-C", dir, "tag", "wktr/fake-task")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create tag: %s", out)
	}

	// BranchExists should return false for tags
	if BranchExists(dir, "wktr/fake-task") {
		t.Error("expected BranchExists to return false for a tag")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	dir := initTestRepo(t)

	if HasUncommittedChanges(dir) {
		t.Error("expected clean repo to have no uncommitted changes")
	}

	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if !HasUncommittedChanges(dir) {
		t.Error("expected repo with new file to have uncommitted changes")
	}
}

func TestCreateAndRemoveWorktree(t *testing.T) {
	dir := initTestRepo(t)
	worktreeDir := filepath.Join(t.TempDir(), "my-worktree")

	err := CreateWorktree(dir, worktreeDir, "wktr/test-task", "")
	if err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		t.Error("expected worktree directory to exist")
	}

	if !BranchExists(dir, "wktr/test-task") {
		t.Error("expected branch to exist after worktree creation")
	}

	err = RemoveWorktree(dir, worktreeDir)
	if err != nil {
		t.Fatalf("failed to remove worktree: %v", err)
	}

	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Error("expected worktree directory to be removed")
	}
}

func TestRemoveWorktreeWithUntrackedFiles(t *testing.T) {
	dir := initTestRepo(t)
	worktreeDir := filepath.Join(t.TempDir(), "my-worktree")

	if err := CreateWorktree(dir, worktreeDir, "wktr/dirty-task", ""); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktreeDir, "untracked.txt"), []byte("leftover"), 0o644); err != nil {
		t.Fatalf("failed to write untracked file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(worktreeDir, "build", "cache"), 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeDir, "build", "cache", "artifact"), []byte("data"), 0o644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	if err := RemoveWorktree(dir, worktreeDir); err != nil {
		t.Fatalf("expected RemoveWorktree to succeed with untracked files, got: %v", err)
	}

	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Error("expected worktree directory to be removed")
	}
}

func TestGetMainWorktree(t *testing.T) {
	dir := initTestRepo(t)

	main, err := GetMainWorktree(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolvedDir, _ := filepath.EvalSymlinks(dir)
	resolvedMain, _ := filepath.EvalSymlinks(main)
	if resolvedMain != resolvedDir {
		t.Errorf("expected %q, got %q", resolvedDir, resolvedMain)
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	commands := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %v: %s", args, out)
		}
	}

	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd := exec.Command("git", "-C", dir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to git add: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "initial commit")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to commit: %s", out)
	}

	return dir
}
