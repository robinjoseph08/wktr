package initialize

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
	"github.com/robinjoseph08/wktr/internal/git"
)

func Run() error {
	var writtenFiles []string

	file, err := initGlobalConfig()
	if err != nil {
		if errors.Is(err, errCancelled) {
			fmt.Println("Cancelled.")
			return nil
		}
		return err
	}
	if file != "" {
		writtenFiles = append(writtenFiles, file)
	}

	file, err = initProjectConfig()
	if err != nil {
		if errors.Is(err, errCancelled) {
			fmt.Println("Cancelled.")
			return nil
		}
		return err
	}
	if file != "" {
		writtenFiles = append(writtenFiles, file)
	}

	printSummary(writtenFiles)
	return nil
}

func initGlobalConfig() (string, error) {
	path, err := config.GlobalConfigPath()
	if err != nil {
		return "", err
	}

	exists, err := config.GlobalConfigExists()
	if err != nil {
		return "", err
	}

	if exists {
		fmt.Printf("Global config already exists at %s, skipping.\n\n", path)
		return "", nil
	}

	defaults := config.DefaultGlobalConfig()

	worktreeDir, err := promptText("Worktree directory", shortenHome(defaults.WorktreeDirectory))
	if err != nil {
		return "", err
	}

	branchPrefix, err := promptText("Branch prefix", defaults.BranchPrefix)
	if err != nil {
		return "", err
	}

	cfg := config.GlobalConfig{
		WorktreeDirectory: worktreeDir,
		BranchPrefix:      branchPrefix,
	}
	if err := config.WriteGlobal(cfg); err != nil {
		return "", fmt.Errorf("failed to write global config: %w", err)
	}

	fmt.Printf("\nWrote global config to %s\n\n", path)
	return path, nil
}

func initProjectConfig() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	repoRoot, err := git.GetRepoRoot(cwd)
	if err != nil {
		fmt.Println("Not in a git repository. To configure a project layout, run `wktr init` from inside a git repo.")
		return "", nil
	}

	orgRepo, orgRepoErr := git.GetOrgRepo(repoRoot)

	configLocation, err := promptConfigLocation(orgRepo, orgRepoErr)
	if err != nil {
		return "", err
	}

	existsMsg, err := checkExistingConfig(configLocation, repoRoot, orgRepo)
	if err != nil {
		return "", err
	}
	if existsMsg != "" {
		overwrite, err := promptConfirm(existsMsg+" Overwrite?", false)
		if err != nil {
			return "", err
		}
		if !overwrite {
			return "", nil
		}
	}

	layout, err := promptLayout()
	if err != nil {
		return "", err
	}

	rc := config.RepoConfig{Layout: layout}
	return writeProjectConfig(configLocation, repoRoot, orgRepo, rc)
}

type configLocationType int

const (
	configLocationRepo configLocationType = iota
	configLocationLocal
	configLocationGlobal
)

func promptConfigLocation(orgRepo git.OrgRepo, orgRepoErr error) (configLocationType, error) {
	options := []string{
		".wktr.yaml (committed to repo)",
		".wktr.local.yaml (gitignored, personal)",
	}
	if orgRepoErr == nil {
		options = append(options, fmt.Sprintf("Global config entry for %s", orgRepo))
	}

	idx, _, err := promptSelect("Where should project config be stored?", options, 0)
	if err != nil {
		return 0, err
	}
	return configLocationType(idx), nil
}

func checkExistingConfig(loc configLocationType, repoRoot string, orgRepo git.OrgRepo) (string, error) {
	switch loc {
	case configLocationRepo:
		path := filepath.Join(repoRoot, ".wktr.yaml")
		if _, err := os.Stat(path); err == nil {
			return fmt.Sprintf("Config already exists at %s.", path), nil
		}
	case configLocationLocal:
		path := filepath.Join(repoRoot, ".wktr.local.yaml")
		if _, err := os.Stat(path); err == nil {
			return fmt.Sprintf("Config already exists at %s.", path), nil
		}
	case configLocationGlobal:
		exists, err := config.GlobalRepoEntryExists(orgRepo.String())
		if err != nil {
			return "", err
		}
		if exists {
			return fmt.Sprintf("Global config already has an entry for %s.", orgRepo), nil
		}
	}
	return "", nil
}

func promptLayout() (config.Layout, error) {
	_, direction, err := promptSelect("Pane direction", []string{"vertical", "horizontal"}, 0)
	if err != nil {
		return config.Layout{}, err
	}

	numPanes, err := promptNumber("Number of panes", 3, 1, 10)
	if err != nil {
		return config.Layout{}, err
	}

	panes := make([]config.Pane, numPanes)
	for i := range numPanes {
		cmd, err := promptText(fmt.Sprintf("Pane %d command (blank for empty shell)", i+1), "")
		if err != nil {
			return config.Layout{}, err
		}
		if cmd != "" {
			panes[i] = config.Pane{Command: cmd}
		}
	}

	if numPanes > 1 {
		labels := make([]string, numPanes)
		for i, p := range panes {
			desc := "(empty shell)"
			if p.Command != "" {
				desc = p.Command
			}
			labels[i] = fmt.Sprintf("Pane %d: %s", i+1, desc)
		}

		focusIdx, _, err := promptSelect("Which pane should have focus?", labels, 0)
		if err != nil {
			return config.Layout{}, err
		}
		panes[focusIdx].Focus = true
	} else {
		panes[0].Focus = true
	}

	return config.Layout{Direction: direction, Panes: panes}, nil
}

func writeProjectConfig(loc configLocationType, repoRoot string, orgRepo git.OrgRepo, rc config.RepoConfig) (string, error) {
	switch loc {
	case configLocationRepo:
		path := filepath.Join(repoRoot, ".wktr.yaml")
		if err := config.WriteRepoConfig(repoRoot, ".wktr.yaml", rc); err != nil {
			return "", fmt.Errorf("failed to write .wktr.yaml: %w", err)
		}
		fmt.Printf("\nWrote %s\n", path)
		return path, nil
	case configLocationLocal:
		path := filepath.Join(repoRoot, ".wktr.local.yaml")
		if err := config.WriteRepoConfig(repoRoot, ".wktr.local.yaml", rc); err != nil {
			return "", fmt.Errorf("failed to write .wktr.local.yaml: %w", err)
		}
		fmt.Printf("\nWrote %s\n", path)
		if !gitignoreContains(repoRoot, ".wktr.local.yaml") {
			fmt.Println("Remember to add .wktr.local.yaml to your .gitignore.")
		}
		return path, nil
	case configLocationGlobal:
		if err := config.AddGlobalRepoEntry(orgRepo.String(), rc); err != nil {
			return "", fmt.Errorf("failed to add global repo entry: %w", err)
		}
		path, err := config.GlobalConfigPath()
		if err != nil {
			return "", err
		}
		fmt.Printf("\nAdded %s entry to %s\n", orgRepo, path)
		return path, nil
	}
	return "", nil
}

func gitignoreContains(repoRoot, entry string) bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

func printSummary(writtenFiles []string) {
	if len(writtenFiles) == 0 {
		return
	}
	seen := make(map[string]bool)
	fmt.Println()
	fmt.Println("Configuration complete!")
	fmt.Println()
	fmt.Println("Files written:")
	for _, f := range writtenFiles {
		if seen[f] {
			continue
		}
		seen[f] = true
		fmt.Printf("  %s\n", f)
	}
	fmt.Println()
	fmt.Println("See the README for more configuration options:")
	fmt.Println("  https://github.com/robinjoseph08/wktr#configuration")
	fmt.Println()
	fmt.Println("Create your first worktree:")
	fmt.Println("  wktr create my-feature")
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && len("~/"+rel) < len(path) {
		return "~/" + rel
	}
	return path
}
