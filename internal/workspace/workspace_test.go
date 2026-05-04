package workspace

import (
	"testing"

	"github.com/robinjoseph08/wktr/internal/git"
)

func TestGenerateName(t *testing.T) {
	name := generateName()
	if len(name) != 6 {
		t.Errorf("expected 6 chars, got %d: %q", len(name), name)
	}

	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			t.Errorf("unexpected character %c in name %q", c, name)
		}
	}

	name2 := generateName()
	if name == name2 {
		t.Error("expected different names on successive calls")
	}
}

func TestInferTaskName(t *testing.T) {
	tests := []struct {
		name         string
		cwd          string
		worktreeBase string
		orgRepo      git.OrgRepo
		want         string
	}{
		{
			name:         "inside worktree root",
			cwd:          "/home/user/.worktrees/org/repo/my-task",
			worktreeBase: "/home/user/.worktrees",
			orgRepo:      git.OrgRepo{Org: "org", Repo: "repo"},
			want:         "my-task",
		},
		{
			name:         "inside worktree subdirectory",
			cwd:          "/home/user/.worktrees/org/repo/my-task/src/pkg",
			worktreeBase: "/home/user/.worktrees",
			orgRepo:      git.OrgRepo{Org: "org", Repo: "repo"},
			want:         "my-task",
		},
		{
			name:         "not inside worktree",
			cwd:          "/home/user/code/repo",
			worktreeBase: "/home/user/.worktrees",
			orgRepo:      git.OrgRepo{Org: "org", Repo: "repo"},
			want:         "",
		},
		{
			name:         "inside different repo worktree",
			cwd:          "/home/user/.worktrees/other-org/other-repo/task",
			worktreeBase: "/home/user/.worktrees",
			orgRepo:      git.OrgRepo{Org: "org", Repo: "repo"},
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferTaskName(tt.cwd, tt.worktreeBase, tt.orgRepo)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidTaskName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"simple", "my-task", true},
		{"with underscore", "my_task", true},
		{"alphanumeric", "fix123", true},
		{"starts with number", "1fix", true},
		{"has dot", "fix.1", false},
		{"has colon", "fix:1", false},
		{"has space", "fix bug", false},
		{"has slash", "fix/bug", false},
		{"starts with hyphen", "-fix", false},
		{"starts with underscore", "_fix", false},
		{"has double dot", "fix..bug", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validTaskName.MatchString(tt.input)
			if got != tt.valid {
				t.Errorf("validTaskName(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestCleanEmptyParents(t *testing.T) {
	dir := t.TempDir()

	// cleanEmptyParents should not panic or error on non-existent paths
	cleanEmptyParents(dir+"/a/b/c", dir)
}
