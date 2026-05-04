package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGlobalConfigPath(t *testing.T) {
	path, err := GlobalConfigPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("expected config.yaml, got %s", filepath.Base(path))
	}
}

func TestWriteAndLoadRepoConfig(t *testing.T) {
	dir := t.TempDir()

	rc := RepoConfig{
		Layout: Layout{
			Direction: "horizontal",
			Panes: []Pane{
				{Command: "make build"},
				{Command: "make test", Focus: true},
			},
		},
	}

	if err := WriteRepoConfig(dir, ".wktr.yaml", rc); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	layout, err := LoadRepo(dir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if layout.Direction != "horizontal" {
		t.Errorf("expected horizontal, got %q", layout.Direction)
	}
	if len(layout.Panes) != 2 {
		t.Errorf("expected 2 panes, got %d", len(layout.Panes))
	}
	if layout.Panes[0].Command != "make build" {
		t.Errorf("expected 'make build', got %q", layout.Panes[0].Command)
	}
	if !layout.Panes[1].Focus {
		t.Error("expected pane 2 to have focus")
	}
}

func TestWriteAndLoadLocalConfig(t *testing.T) {
	dir := t.TempDir()

	rc := RepoConfig{
		Layout: Layout{
			Direction: "vertical",
			Panes:    []Pane{{Command: "local-cmd"}},
		},
	}

	if err := WriteRepoConfig(dir, ".wktr.local.yaml", rc); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	layout, err := LoadRepo(dir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if layout.Panes[0].Command != "local-cmd" {
		t.Errorf("expected 'local-cmd', got %q", layout.Panes[0].Command)
	}
}

func TestWriteAndLoadGlobalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wktr", "config.yaml")

	cfg := GlobalConfig{
		WorktreeDirectory: "~/.worktrees",
		BranchPrefix:      "feat/",
		Repos: map[string]RepoConfig{
			"org/repo": {
				Layout: Layout{
					Direction: "vertical",
					Panes:    []Pane{{Command: "go test ./..."}},
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	loaded, err := LoadGlobalFrom(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	if loaded.BranchPrefix != "feat/" {
		t.Errorf("expected branch_prefix %q, got %q", "feat/", loaded.BranchPrefix)
	}

	entry, ok := loaded.Repos["org/repo"]
	if !ok {
		t.Fatal("expected repos to contain org/repo")
	}
	if entry.Layout.Panes[0].Command != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", entry.Layout.Panes[0].Command)
	}
}

func TestGlobalRepoEntryExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wktr", "config.yaml")

	cfg := GlobalConfig{
		WorktreeDirectory: "~/.worktrees",
		BranchPrefix:      "wktr/",
		Repos: map[string]RepoConfig{
			"org/repo": {
				Layout: Layout{
					Direction: "vertical",
					Panes:    []Pane{{Command: "test"}},
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	loaded, err := LoadGlobalFrom(path)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	_, ok := loaded.Repos["org/repo"]
	if !ok {
		t.Error("expected org/repo to exist")
	}

	_, ok = loaded.Repos["other/repo"]
	if ok {
		t.Error("expected other/repo to not exist")
	}
}
