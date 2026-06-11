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
		Layout: &Layout{
			Direction: "vertical",
			Panes: []Pane{
				{Command: "echo hello"},
			},
		},
		Repos: map[string]RepoConfig{
			"org/repo": {
				Layout: &Layout{
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
	if loaded.Layout == nil {
		t.Fatal("expected layout to be set")
	}
	if len(loaded.Layout.Panes) != 1 {
		t.Errorf("expected 1 pane in layout, got %d", len(loaded.Layout.Panes))
	}
	if _, ok := loaded.Repos["org/repo"]; !ok {
		t.Error("expected repos to contain org/repo")
	}
}

func TestLoadGlobalFromDefaultLayoutRename(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
	}{
		{
			name: "default_layout with a value",
			yamlData: `
worktree_directory: /custom/path
default_layout:
  direction: vertical
  panes:
    - command: "echo hello"
`,
		},
		{
			name: "default_layout with a null value",
			yamlData: `
worktree_directory: /custom/path
default_layout:
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")

			if err := os.WriteFile(path, []byte(tt.yamlData), 0o644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			_, err := LoadGlobalFrom(path)
			if err == nil {
				t.Fatal("expected error for default_layout key, got nil")
			}
			if !strings.Contains(err.Error(), "default_layout") {
				t.Errorf("expected error to mention default_layout, got: %v", err)
			}
			if !strings.Contains(err.Error(), `renamed to "layout"`) {
				t.Errorf("expected error to name the rename to layout, got: %v", err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to mention file path, got: %v", err)
			}
		})
	}
}

func TestLoadGlobalFromInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(":\tinvalid: {{yaml"), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadGlobalFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("expected error to mention file path, got: %v", err)
	}
}

func TestLoadGlobalFromLayoutDirection(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		wantErrSubs []string
	}{
		{
			name: "invalid direction in top-level layout",
			yamlData: `
layout:
  direction: diagonal
  panes:
    - command: "echo hello"
`,
			wantErrSubs: []string{`"diagonal"`, "vertical", "horizontal"},
		},
		{
			name: "invalid direction in repos entry layout",
			yamlData: `
repos:
  org/repo:
    layout:
      direction: sideways
      panes:
        - command: "echo hello"
`,
			wantErrSubs: []string{`"sideways"`, "vertical", "horizontal"},
		},
		{
			name: "vertical direction is valid",
			yamlData: `
layout:
  direction: vertical
  panes:
    - command: "echo hello"
`,
		},
		{
			name: "horizontal direction is valid",
			yamlData: `
layout:
  direction: horizontal
  panes:
    - command: "echo hello"
`,
		},
		{
			name: "omitted direction is valid",
			yamlData: `
layout:
  panes:
    - command: "echo hello"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yamlData), 0o644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			_, err := LoadGlobalFrom(path)
			if len(tt.wantErrSubs) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error for invalid direction, got nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to mention file path, got: %v", err)
			}
			for _, sub := range tt.wantErrSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("expected error to contain %q, got: %v", sub, err)
				}
			}
		})
	}
}

func layoutYAML(command string) string {
	return "layout:\n  direction: vertical\n  panes:\n    - command: \"" + command + "\"\n"
}

func TestLoadGlobalFromMultiplexer(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		wantErrSubs []string
		want        string
	}{
		{
			name:     "tmux is valid",
			yamlData: "multiplexer: tmux\n",
			want:     "tmux",
		},
		{
			name:     "herdr is valid",
			yamlData: "multiplexer: herdr\n",
			want:     "herdr",
		},
		{
			name:     "auto is valid",
			yamlData: "multiplexer: auto\n",
			want:     "auto",
		},
		{
			name:     "omitted is valid",
			yamlData: "branch_prefix: feat/\n",
			want:     "",
		},
		{
			name:        "invalid value at top level",
			yamlData:    "multiplexer: screen\n",
			wantErrSubs: []string{`"screen"`, "tmux", "herdr", "auto"},
		},
		{
			name:        "invalid value in repos entry",
			yamlData:    "repos:\n  org/repo:\n    multiplexer: zellij\n",
			wantErrSubs: []string{`"zellij"`, "tmux", "herdr", "auto", `"org/repo"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yamlData), 0o644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			loaded, err := LoadGlobalFrom(path)
			if len(tt.wantErrSubs) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if loaded.Multiplexer != tt.want {
					t.Errorf("expected multiplexer %q, got %q", tt.want, loaded.Multiplexer)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error for invalid multiplexer, got nil")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("expected error to mention file path, got: %v", err)
			}
			for _, sub := range tt.wantErrSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("expected error to contain %q, got: %v", sub, err)
				}
			}
		})
	}
}

func TestResolve_MultiplexerFallthrough(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		global  GlobalConfig
		orgRepo string
		want    string
	}{
		{
			name: "local config wins over all levels",
			files: map[string]string{
				".wktr.local.yaml": "multiplexer: herdr\n",
				".wktr.yaml":       "multiplexer: tmux\n",
			},
			global: GlobalConfig{
				Multiplexer: "tmux",
				Repos:       map[string]RepoConfig{"org/repo": {Multiplexer: "tmux"}},
			},
			orgRepo: "org/repo",
			want:    "herdr",
		},
		{
			name: "local config omitting the key falls through to repo config",
			files: map[string]string{
				".wktr.local.yaml": layoutYAML("local-cmd"),
				".wktr.yaml":       "multiplexer: herdr\n",
			},
			global: GlobalConfig{
				Multiplexer: "tmux",
				Repos:       map[string]RepoConfig{"org/repo": {Multiplexer: "tmux"}},
			},
			orgRepo: "org/repo",
			want:    "herdr",
		},
		{
			name: "repo config omitting the key falls through to global repos entry",
			files: map[string]string{
				".wktr.yaml": layoutYAML("repo-cmd"),
			},
			global: GlobalConfig{
				Multiplexer: "tmux",
				Repos:       map[string]RepoConfig{"org/repo": {Multiplexer: "herdr"}},
			},
			orgRepo: "org/repo",
			want:    "herdr",
		},
		{
			name: "repos entry omitting the key falls through to global top level",
			global: GlobalConfig{
				Multiplexer: "herdr",
				Repos:       map[string]RepoConfig{"org/repo": {}},
			},
			orgRepo: "org/repo",
			want:    "herdr",
		},
		{
			name:    "nothing set falls through to the auto default",
			global:  GlobalConfig{},
			orgRepo: "org/repo",
			want:    "auto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("failed to write %s: %v", name, err)
				}
			}

			resolved, err := Resolve(tt.global, dir, tt.orgRepo)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolved.Multiplexer != tt.want {
				t.Errorf("expected multiplexer %q, got %q", tt.want, resolved.Multiplexer)
			}
		})
	}
}

func TestResolve_InvalidMultiplexerInRepoFiles(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantErrSubs []string
	}{
		{
			name: "invalid multiplexer in repo config",
			files: map[string]string{
				".wktr.yaml": "multiplexer: screen\n",
			},
			wantErrSubs: []string{".wktr.yaml", `"screen"`, "tmux", "herdr", "auto"},
		},
		{
			name: "invalid multiplexer in local config",
			files: map[string]string{
				".wktr.local.yaml": "multiplexer: zellij\n",
				".wktr.yaml":       "multiplexer: tmux\n",
			},
			wantErrSubs: []string{".wktr.local.yaml", `"zellij"`, "tmux", "herdr", "auto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("failed to write %s: %v", name, err)
				}
			}

			_, err := Resolve(GlobalConfig{}, dir, "org/repo")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, sub := range tt.wantErrSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("expected error to contain %q, got: %v", sub, err)
				}
			}
		})
	}
}

func TestResolve_PerKeyFallthrough(t *testing.T) {
	globalLayout := &Layout{
		Direction: "vertical",
		Panes:     []Pane{{Command: "global-cmd"}},
	}
	reposEntryLayout := &Layout{
		Direction: "vertical",
		Panes:     []Pane{{Command: "repos-entry-cmd"}},
	}

	tests := []struct {
		name      string
		files     map[string]string
		global    GlobalConfig
		orgRepo   string
		wantCmd   string
		wantPanes int
	}{
		{
			name: "local config wins over all levels",
			files: map[string]string{
				".wktr.local.yaml": layoutYAML("local-cmd"),
				".wktr.yaml":       layoutYAML("repo-cmd"),
			},
			global: GlobalConfig{
				Layout: globalLayout,
				Repos:  map[string]RepoConfig{"org/repo": {Layout: reposEntryLayout}},
			},
			orgRepo:   "org/repo",
			wantCmd:   "local-cmd",
			wantPanes: 1,
		},
		{
			name: "local config omitting layout falls through to repo config",
			files: map[string]string{
				".wktr.local.yaml": "# personal overrides, no layout key\n",
				".wktr.yaml":       layoutYAML("repo-cmd"),
			},
			global: GlobalConfig{
				Layout: globalLayout,
				Repos:  map[string]RepoConfig{"org/repo": {Layout: reposEntryLayout}},
			},
			orgRepo:   "org/repo",
			wantCmd:   "repo-cmd",
			wantPanes: 1,
		},
		{
			name: "repo config omitting layout falls through to global repos entry",
			files: map[string]string{
				".wktr.yaml": "# repo config, no layout key\n",
			},
			global: GlobalConfig{
				Layout: globalLayout,
				Repos:  map[string]RepoConfig{"org/repo": {Layout: reposEntryLayout}},
			},
			orgRepo:   "org/repo",
			wantCmd:   "repos-entry-cmd",
			wantPanes: 1,
		},
		{
			name: "no repo files falls through to global repos entry",
			global: GlobalConfig{
				Layout: globalLayout,
				Repos:  map[string]RepoConfig{"org/repo": {Layout: reposEntryLayout}},
			},
			orgRepo:   "org/repo",
			wantCmd:   "repos-entry-cmd",
			wantPanes: 1,
		},
		{
			name: "repos entry omitting layout falls through to global layout",
			global: GlobalConfig{
				Layout: globalLayout,
				Repos:  map[string]RepoConfig{"org/repo": {}},
			},
			orgRepo:   "org/repo",
			wantCmd:   "global-cmd",
			wantPanes: 1,
		},
		{
			name: "no repos entry falls through to global layout",
			global: GlobalConfig{
				Layout: globalLayout,
				Repos:  map[string]RepoConfig{"org/repo": {Layout: reposEntryLayout}},
			},
			orgRepo:   "other/repo",
			wantCmd:   "global-cmd",
			wantPanes: 1,
		},
		{
			name:      "nothing set falls through to built-in default",
			global:    GlobalConfig{},
			orgRepo:   "org/repo",
			wantCmd:   "",
			wantPanes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("failed to write %s: %v", name, err)
				}
			}

			resolved, err := Resolve(tt.global, dir, tt.orgRepo)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resolved.Layout.Panes) != tt.wantPanes {
				t.Fatalf("expected %d panes, got %d", tt.wantPanes, len(resolved.Layout.Panes))
			}
			if resolved.Layout.Panes[0].Command != tt.wantCmd {
				t.Errorf("expected command %q, got %q", tt.wantCmd, resolved.Layout.Panes[0].Command)
			}
		})
	}
}

func TestResolve_LayoutIsAtomic(t *testing.T) {
	dir := t.TempDir()

	localYAML := "layout:\n  direction: horizontal\n  panes:\n    - command: \"local-cmd\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".wktr.local.yaml"), []byte(localYAML), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	repoYAML := "layout:\n  direction: vertical\n  panes:\n    - command: \"repo-cmd-1\"\n    - command: \"repo-cmd-2\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".wktr.yaml"), []byte(repoYAML), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	resolved, err := Resolve(GlobalConfig{}, dir, "org/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.Layout.Direction != "horizontal" {
		t.Errorf("expected direction %q, got %q", "horizontal", resolved.Layout.Direction)
	}
	if len(resolved.Layout.Panes) != 1 {
		t.Fatalf("expected winning layout to apply wholesale with 1 pane, got %d", len(resolved.Layout.Panes))
	}
	if resolved.Layout.Panes[0].Command != "local-cmd" {
		t.Errorf("expected command %q, got %q", "local-cmd", resolved.Layout.Panes[0].Command)
	}
}

func TestResolve_GlobalOnlyKeys(t *testing.T) {
	dir := t.TempDir()
	global := GlobalConfig{
		WorktreeDirectory: "/worktrees",
		BranchPrefix:      "wktr/",
	}

	resolved, err := Resolve(global, dir, "org/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved.WorktreeDirectory != "/worktrees" {
		t.Errorf("expected worktree_directory %q, got %q", "/worktrees", resolved.WorktreeDirectory)
	}
	if resolved.BranchPrefix != "wktr/" {
		t.Errorf("expected branch_prefix %q, got %q", "wktr/", resolved.BranchPrefix)
	}
}

func TestResolve_InvalidRepoFiles(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantErrSubs []string
	}{
		{
			name: "invalid YAML in repo config",
			files: map[string]string{
				".wktr.yaml": ":\tinvalid: {{yaml",
			},
			wantErrSubs: []string{".wktr.yaml"},
		},
		{
			name: "invalid YAML in local config",
			files: map[string]string{
				".wktr.local.yaml": ":\tinvalid: {{yaml",
				".wktr.yaml":       layoutYAML("repo-cmd"),
			},
			wantErrSubs: []string{".wktr.local.yaml"},
		},
		{
			name: "invalid direction in repo config",
			files: map[string]string{
				".wktr.yaml": "layout:\n  direction: diagonal\n  panes:\n    - command: \"echo\"\n",
			},
			wantErrSubs: []string{".wktr.yaml", `"diagonal"`, "vertical", "horizontal"},
		},
		{
			name: "invalid direction in local config",
			files: map[string]string{
				".wktr.local.yaml": "layout:\n  direction: sideways\n  panes:\n    - command: \"echo\"\n",
				".wktr.yaml":       layoutYAML("repo-cmd"),
			},
			wantErrSubs: []string{".wktr.local.yaml", `"sideways"`, "vertical", "horizontal"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("failed to write %s: %v", name, err)
				}
			}

			_, err := Resolve(GlobalConfig{}, dir, "org/repo")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, sub := range tt.wantErrSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("expected error to contain %q, got: %v", sub, err)
				}
			}
		})
	}
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
