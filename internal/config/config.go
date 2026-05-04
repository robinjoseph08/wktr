package config

import (
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
	Layout Layout `yaml:"layout"`
}

type GlobalConfig struct {
	WorktreeDirectory string                `yaml:"worktree_directory"`
	BranchPrefix      string                `yaml:"branch_prefix"`
	DefaultLayout     *Layout               `yaml:"default_layout,omitempty"`
	Repos             map[string]RepoConfig `yaml:"repos,omitempty"`
}

type ResolvedConfig struct {
	WorktreeDirectory string
	BranchPrefix      string
	Layout            Layout
}

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
		Panes:    []Pane{{}},
	}
}

func LoadGlobal() (GlobalConfig, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return DefaultGlobalConfig(), nil
	}
	return LoadGlobalFrom(filepath.Join(configDir, "wktr", "config.yaml"))
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

	var fileCfg GlobalConfig
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return cfg, err
	}

	if fileCfg.WorktreeDirectory != "" {
		cfg.WorktreeDirectory = expandHome(fileCfg.WorktreeDirectory)
	}
	if fileCfg.BranchPrefix != "" {
		cfg.BranchPrefix = fileCfg.BranchPrefix
	}
	if fileCfg.DefaultLayout != nil {
		cfg.DefaultLayout = fileCfg.DefaultLayout
	}
	if fileCfg.Repos != nil {
		cfg.Repos = fileCfg.Repos
	}

	return cfg, nil
}

func LoadRepo(dir string) (*Layout, error) {
	localPath := filepath.Join(dir, ".wktr.local.yaml")
	if layout, err := loadLayoutFile(localPath); err == nil {
		return layout, nil
	}

	repoPath := filepath.Join(dir, ".wktr.yaml")
	if layout, err := loadLayoutFile(repoPath); err == nil {
		return layout, nil
	}

	return nil, nil
}

func Resolve(global GlobalConfig, repoDir string, orgRepo string) ResolvedConfig {
	resolved := ResolvedConfig{
		WorktreeDirectory: global.WorktreeDirectory,
		BranchPrefix:      global.BranchPrefix,
	}

	repoLayout, _ := LoadRepo(repoDir)
	if repoLayout != nil {
		resolved.Layout = *repoLayout
		return resolved
	}

	if rc, ok := global.Repos[orgRepo]; ok {
		resolved.Layout = rc.Layout
		return resolved
	}

	if global.DefaultLayout != nil {
		resolved.Layout = *global.DefaultLayout
		return resolved
	}

	resolved.Layout = DefaultLayout()
	return resolved
}

func loadLayoutFile(path string) (*Layout, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rc RepoConfig
	if err := yaml.Unmarshal(data, &rc); err != nil {
		return nil, err
	}

	return &rc.Layout, nil
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
