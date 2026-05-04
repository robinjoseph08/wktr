package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultGlobalConfig(t *testing.T) {
	cfg := DefaultGlobalConfig()

	home, _ := os.UserHomeDir()
	expectedDir := filepath.Join(home, ".worktrees")

	if cfg.WorktreeDirectory != expectedDir {
		t.Errorf("expected worktree_directory %q, got %q", expectedDir, cfg.WorktreeDirectory)
	}
	if cfg.BranchPrefix != "wktr/" {
		t.Errorf("expected branch_prefix %q, got %q", "wktr/", cfg.BranchPrefix)
	}
}

func TestDefaultLayout(t *testing.T) {
	layout := DefaultLayout()
	if layout.Direction != "vertical" {
		t.Errorf("expected direction %q, got %q", "vertical", layout.Direction)
	}
	if len(layout.Panes) != 1 {
		t.Errorf("expected 1 pane, got %d", len(layout.Panes))
	}
}

func TestLoadGlobalFromFile(t *testing.T) {
	dir := t.TempDir()

	cfg := GlobalConfig{
		WorktreeDirectory: "/custom/path",
		BranchPrefix:      "feat/",
		DefaultLayout: &Layout{
			Direction: "vertical",
			Panes: []Pane{
				{Command: "echo hello"},
			},
		},
		Repos: map[string]RepoConfig{
			"org/repo": {
				Layout: Layout{
					Direction: "vertical",
					Panes: []Pane{
						{Command: "npm start"},
					},
				},
			},
		},
	}

	data, _ := yaml.Marshal(cfg)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loaded, err := LoadGlobalFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if loaded.WorktreeDirectory != "/custom/path" {
		t.Errorf("expected worktree_directory %q, got %q", "/custom/path", loaded.WorktreeDirectory)
	}
	if loaded.BranchPrefix != "feat/" {
		t.Errorf("expected branch_prefix %q, got %q", "feat/", loaded.BranchPrefix)
	}
	if loaded.DefaultLayout == nil {
		t.Fatal("expected default_layout to be set")
	}
	if len(loaded.DefaultLayout.Panes) != 1 {
		t.Errorf("expected 1 pane in default_layout, got %d", len(loaded.DefaultLayout.Panes))
	}
	if _, ok := loaded.Repos["org/repo"]; !ok {
		t.Error("expected repos to contain org/repo")
	}
}

func TestLoadRepo(t *testing.T) {
	dir := t.TempDir()

	repoConfig := RepoConfig{
		Layout: Layout{
			Direction: "vertical",
			Panes: []Pane{
				{Command: "make build"},
				{Command: "make test"},
			},
		},
	}
	data, _ := yaml.Marshal(repoConfig)
	if err := os.WriteFile(filepath.Join(dir, ".wktr.yaml"), data, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	layout, err := LoadRepo(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if layout == nil {
		t.Fatal("expected layout to be loaded")
	}
	if len(layout.Panes) != 2 {
		t.Errorf("expected 2 panes, got %d", len(layout.Panes))
	}
}

func TestLoadRepoLocalOverridesRepo(t *testing.T) {
	dir := t.TempDir()

	repoConfig := RepoConfig{
		Layout: Layout{
			Direction: "vertical",
			Panes:    []Pane{{Command: "repo-command"}},
		},
	}
	data, _ := yaml.Marshal(repoConfig)
	if err := os.WriteFile(filepath.Join(dir, ".wktr.yaml"), data, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	localConfig := RepoConfig{
		Layout: Layout{
			Direction: "vertical",
			Panes:    []Pane{{Command: "local-command"}},
		},
	}
	data, _ = yaml.Marshal(localConfig)
	if err := os.WriteFile(filepath.Join(dir, ".wktr.local.yaml"), data, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	layout, err := LoadRepo(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if layout == nil {
		t.Fatal("expected layout to be loaded")
	}
	if layout.Panes[0].Command != "local-command" {
		t.Errorf("expected local config to win, got command %q", layout.Panes[0].Command)
	}
}

func TestLoadRepoInvalidYAML(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, ".wktr.yaml"), []byte(":\tinvalid: {{yaml"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := LoadRepo(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), ".wktr.yaml") {
		t.Errorf("expected error to mention file path, got: %v", err)
	}
}

func TestLoadRepoInvalidLocalYAML(t *testing.T) {
	dir := t.TempDir()

	repoConfig := RepoConfig{
		Layout: Layout{
			Direction: "vertical",
			Panes:    []Pane{{Command: "repo-command"}},
		},
	}
	data, _ := yaml.Marshal(repoConfig)
	if err := os.WriteFile(filepath.Join(dir, ".wktr.yaml"), data, 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".wktr.local.yaml"), []byte(":\tinvalid: {{yaml"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := LoadRepo(dir)
	if err == nil {
		t.Fatal("expected error for invalid local YAML, got nil")
	}
	if !strings.Contains(err.Error(), ".wktr.local.yaml") {
		t.Errorf("expected error to mention local file path, got: %v", err)
	}
}

func TestResolve_Precedence(t *testing.T) {
	global := GlobalConfig{
		WorktreeDirectory: "/worktrees",
		BranchPrefix:      "wktr/",
		DefaultLayout: &Layout{
			Direction: "vertical",
			Panes:    []Pane{{Command: "default-cmd"}},
		},
		Repos: map[string]RepoConfig{
			"org/repo": {
				Layout: Layout{
					Direction: "vertical",
					Panes:    []Pane{{Command: "global-repo-cmd"}},
				},
			},
		},
	}

	t.Run("uses repo config file when present", func(t *testing.T) {
		dir := t.TempDir()
		rc := RepoConfig{
			Layout: Layout{
				Direction: "vertical",
				Panes:    []Pane{{Command: "file-cmd"}},
			},
		}
		data, _ := yaml.Marshal(rc)
		if err := os.WriteFile(filepath.Join(dir, ".wktr.yaml"), data, 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		resolved := Resolve(global, dir, "org/repo")
		if resolved.Layout.Panes[0].Command != "file-cmd" {
			t.Errorf("expected file-cmd, got %q", resolved.Layout.Panes[0].Command)
		}
	})

	t.Run("falls back to global repos entry", func(t *testing.T) {
		dir := t.TempDir()
		resolved := Resolve(global, dir, "org/repo")
		if resolved.Layout.Panes[0].Command != "global-repo-cmd" {
			t.Errorf("expected global-repo-cmd, got %q", resolved.Layout.Panes[0].Command)
		}
	})

	t.Run("falls back to default layout", func(t *testing.T) {
		dir := t.TempDir()
		resolved := Resolve(global, dir, "other/repo")
		if resolved.Layout.Panes[0].Command != "default-cmd" {
			t.Errorf("expected default-cmd, got %q", resolved.Layout.Panes[0].Command)
		}
	})

	t.Run("falls back to hardcoded default", func(t *testing.T) {
		noDefault := GlobalConfig{
			WorktreeDirectory: "/worktrees",
			BranchPrefix:      "wktr/",
		}
		dir := t.TempDir()
		resolved := Resolve(noDefault, dir, "any/repo")
		if resolved.Layout.Direction != "vertical" {
			t.Errorf("expected vertical, got %q", resolved.Layout.Direction)
		}
		if len(resolved.Layout.Panes) != 1 {
			t.Errorf("expected 1 pane, got %d", len(resolved.Layout.Panes))
		}
	})
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		result := expandHome(tt.input)
		if result != tt.expected {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestPaneRunField(t *testing.T) {
	yamlData := `
layout:
  direction: vertical
  panes:
    - command: "echo run-true"
      run: true
    - command: "echo run-false"
      run: false
    - command: "echo no-run"
`
	var rc RepoConfig
	if err := yaml.Unmarshal([]byte(yamlData), &rc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	panes := rc.Layout.Panes

	if panes[0].Run == nil || *panes[0].Run != true {
		t.Error("expected pane 0 run=true")
	}
	if panes[1].Run == nil || *panes[1].Run != false {
		t.Error("expected pane 1 run=false")
	}
	if panes[2].Run != nil {
		t.Error("expected pane 2 run=nil")
	}
}

func TestCommandsList(t *testing.T) {
	yamlData := `
layout:
  direction: vertical
  panes:
    - commands:
        - value: "npm install"
          run: true
        - value: "npm start"
          run: false
`
	var rc RepoConfig
	if err := yaml.Unmarshal([]byte(yamlData), &rc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	pane := rc.Layout.Panes[0]
	if len(pane.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(pane.Commands))
	}
	if pane.Commands[0].Value != "npm install" {
		t.Errorf("expected 'npm install', got %q", pane.Commands[0].Value)
	}
	if pane.Commands[0].Run == nil || *pane.Commands[0].Run != true {
		t.Error("expected command 0 run=true")
	}
	if pane.Commands[1].Value != "npm start" {
		t.Errorf("expected 'npm start', got %q", pane.Commands[1].Value)
	}
	if pane.Commands[1].Run == nil || *pane.Commands[1].Run != false {
		t.Error("expected command 1 run=false")
	}
}
