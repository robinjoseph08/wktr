package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Command struct {
	Value string `yaml:"value"`
	Run   *bool  `yaml:"run,omitempty"`
}

type Pane struct {
	Command  string    `yaml:"command,omitempty"`
	Commands []Command `yaml:"commands,omitempty"`
	Run      *bool     `yaml:"run,omitempty"`
	Size     int       `yaml:"size,omitempty"`
	Focus    bool      `yaml:"focus,omitempty"`
}

type Layout struct {
	Direction string `yaml:"direction"`
	Panes     []Pane `yaml:"panes"`
}

type RepoConfig struct {
	Layout      *Layout `yaml:"layout,omitempty"`
	Multiplexer string  `yaml:"multiplexer,omitempty"`
}

type GlobalConfig struct {
	WorktreeDirectory string                `yaml:"worktree_directory"`
	BranchPrefix      string                `yaml:"branch_prefix"`
	Layout            *Layout               `yaml:"layout,omitempty"`
	Multiplexer       string                `yaml:"multiplexer,omitempty"`
	Repos             map[string]RepoConfig `yaml:"repos,omitempty"`
}

type ResolvedConfig struct {
	WorktreeDirectory string
	BranchPrefix      string
	Layout            Layout
	Multiplexer       string
}

// DefaultMultiplexer is the built-in multiplexer value when no config level
// sets one: auto-detect the Multiplexer wktr is running inside.
const DefaultMultiplexer = "auto"

func DefaultGlobalConfig() GlobalConfig {
	home, _ := os.UserHomeDir()
	return GlobalConfig{
		WorktreeDirectory: filepath.Join(home, ".worktrees"),
		BranchPrefix:      "wktr/",
	}
}

func DefaultLayout() Layout {
	return Layout{
		Direction: "vertical",
		Panes:     []Pane{{}},
	}
}

func GlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "wktr", "config.yaml"), nil
}

func LoadGlobal() (GlobalConfig, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return DefaultGlobalConfig(), nil
	}
	return LoadGlobalFrom(path)
}

func LoadGlobalFrom(path string) (GlobalConfig, error) {
	cfg := DefaultGlobalConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	// Probe for the renamed key by presence, not value, so even a bare
	// "default_layout:" with a null value triggers the error.
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal(data, &keys); err == nil {
		if _, ok := keys["default_layout"]; ok {
			return cfg, fmt.Errorf("invalid %s: %q has been renamed to %q", path, "default_layout", "layout")
		}
	}

	var fileCfg GlobalConfig
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return cfg, fmt.Errorf("invalid %s: %w", path, err)
	}

	if fileCfg.WorktreeDirectory != "" {
		cfg.WorktreeDirectory = expandHome(fileCfg.WorktreeDirectory)
	}
	if fileCfg.BranchPrefix != "" {
		cfg.BranchPrefix = fileCfg.BranchPrefix
	}
	if fileCfg.Layout != nil {
		cfg.Layout = fileCfg.Layout
	}
	if fileCfg.Multiplexer != "" {
		cfg.Multiplexer = fileCfg.Multiplexer
	}
	if fileCfg.Repos != nil {
		cfg.Repos = fileCfg.Repos
	}

	if err := validateLayout(cfg.Layout); err != nil {
		return cfg, fmt.Errorf("invalid %s: %w", path, err)
	}
	if err := validateMultiplexer(cfg.Multiplexer); err != nil {
		return cfg, fmt.Errorf("invalid %s: %w", path, err)
	}
	for orgRepo, rc := range cfg.Repos {
		if err := validateLayout(rc.Layout); err != nil {
			return cfg, fmt.Errorf("invalid %s: repos entry %q: %w", path, orgRepo, err)
		}
		if err := validateMultiplexer(rc.Multiplexer); err != nil {
			return cfg, fmt.Errorf("invalid %s: repos entry %q: %w", path, orgRepo, err)
		}
	}

	return cfg, nil
}

// Resolve builds the effective config for a repo. Each per-repo setting
// resolves independently down the hierarchy: Local config, Repo config,
// global repos entry, global top level, built-in default. A file that omits
// a key is transparent for that key.
func Resolve(global GlobalConfig, repoDir string, orgRepo string) (ResolvedConfig, error) {
	local, err := loadRepoFile(filepath.Join(repoDir, ".wktr.local.yaml"))
	if err != nil {
		return ResolvedConfig{}, err
	}
	repo, err := loadRepoFile(filepath.Join(repoDir, ".wktr.yaml"))
	if err != nil {
		return ResolvedConfig{}, err
	}

	return ResolvedConfig{
		WorktreeDirectory: global.WorktreeDirectory,
		BranchPrefix:      global.BranchPrefix,
		Layout:            resolveLayout(local.Layout, repo.Layout, global.Repos[orgRepo].Layout, global.Layout),
		Multiplexer:       resolveMultiplexer(local.Multiplexer, repo.Multiplexer, global.Repos[orgRepo].Multiplexer, global.Multiplexer),
	}, nil
}

// resolveLayout picks the first level that sets a layout. The winning layout
// applies wholesale: panes are never merged across levels.
func resolveLayout(levels ...*Layout) Layout {
	for _, layout := range levels {
		if layout != nil {
			return *layout
		}
	}
	return DefaultLayout()
}

// resolveMultiplexer picks the first level that sets a multiplexer, falling
// through to the auto default.
func resolveMultiplexer(levels ...string) string {
	for _, value := range levels {
		if value != "" {
			return value
		}
	}
	return DefaultMultiplexer
}

func loadRepoFile(path string) (RepoConfig, error) {
	var rc RepoConfig

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return rc, nil
		}
		return rc, err
	}

	if err := yaml.Unmarshal(data, &rc); err != nil {
		return RepoConfig{}, fmt.Errorf("invalid %s: %w", path, err)
	}
	if err := validateLayout(rc.Layout); err != nil {
		return RepoConfig{}, fmt.Errorf("invalid %s: %w", path, err)
	}
	if err := validateMultiplexer(rc.Multiplexer); err != nil {
		return RepoConfig{}, fmt.Errorf("invalid %s: %w", path, err)
	}

	return rc, nil
}

func validateLayout(layout *Layout) error {
	if layout == nil {
		return nil
	}
	switch layout.Direction {
	case "", "vertical", "horizontal":
		return nil
	default:
		return fmt.Errorf("invalid layout direction %q (must be %q or %q)", layout.Direction, "vertical", "horizontal")
	}
}

func validateMultiplexer(value string) error {
	switch value {
	case "", "tmux", "herdr", "auto":
		return nil
	default:
		return fmt.Errorf("invalid multiplexer %q (must be %q, %q, or %q)", value, "tmux", "herdr", "auto")
	}
}

func expandHome(path string) string {
	if len(path) > 1 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
