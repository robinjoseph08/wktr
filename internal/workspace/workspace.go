package workspace

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
	"github.com/robinjoseph08/wktr/internal/git"
	"github.com/robinjoseph08/wktr/internal/tmux"
)

type CreateOpts struct {
	Name string
	From string
	Dir  string
}

type RemoveOpts struct {
	Name  string
	Force bool
	Dir   string
}

type ListOpts struct {
	All bool
	Dir string
}

type WorktreeInfo struct {
	Name       string
	Branch     string
	Dir        string
	HasWindow  bool
	OrgRepo    string
	HasChanges bool
}

func Create(opts CreateOpts) error {
	if !tmux.InTmux() {
		return fmt.Errorf("must be run inside a tmux session")
	}

	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	mainWorktree, err := git.GetMainWorktree(dir)
	if err != nil {
		return err
	}

	orgRepo, err := git.GetOrgRepo(mainWorktree)
	if err != nil {
		return err
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	resolved := config.Resolve(globalCfg, mainWorktree, orgRepo.String())

	name := opts.Name
	if name == "" {
		name = generateName()
	}

	branchName := resolved.BranchPrefix + name
	if git.BranchExists(mainWorktree, branchName) {
		return fmt.Errorf("branch %q already exists — use a different name or run `wktr remove %s` first", branchName, name)
	}

	worktreeDir := git.WorktreeDir(resolved.WorktreeDirectory, orgRepo, name)

	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0o755); err != nil {
		return fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	fmt.Printf("Creating worktree at %s...\n", worktreeDir)
	if err := git.CreateWorktree(mainWorktree, worktreeDir, branchName, opts.From); err != nil {
		return err
	}

	fmt.Println("Opening tmux window...")
	if err := tmux.CreateWindow(name, worktreeDir); err != nil {
		return err
	}

	if err := tmux.SetupPanes(name, worktreeDir, resolved.Layout); err != nil {
		return err
	}

	fmt.Printf("Task %q started in new tmux window\n", name)
	return nil
}

func Remove(opts RemoveOpts) error {
	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	mainWorktree, err := git.GetMainWorktree(dir)
	if err != nil {
		return err
	}

	orgRepo, err := git.GetOrgRepo(mainWorktree)
	if err != nil {
		return err
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	resolved := config.Resolve(globalCfg, mainWorktree, orgRepo.String())

	name := opts.Name
	if name == "" {
		name = inferTaskName(dir, resolved.WorktreeDirectory, orgRepo)
		if name == "" {
			return fmt.Errorf("no task name provided and not in a worktree directory")
		}
	}

	worktreeDir := git.WorktreeDir(resolved.WorktreeDirectory, orgRepo, name)
	if _, err := os.Stat(worktreeDir); os.IsNotExist(err) {
		return fmt.Errorf("worktree directory not found: %s", worktreeDir)
	}

	if !opts.Force {
		hasChanges := git.HasUncommittedChanges(worktreeDir)
		if hasChanges {
			fmt.Printf("This will delete the worktree for task %q.\n", name)
			fmt.Println("WARNING: There are uncommitted changes that will be lost.")
		} else {
			fmt.Printf("This will delete the worktree and branch for task %q.\n", name)
		}
		fmt.Print("Are you sure? [y/N] ")
		var response string
		fmt.Scanln(&response)
		if !strings.HasPrefix(strings.ToLower(response), "y") {
			fmt.Println("Cancelled")
			return nil
		}
	}

	fmt.Printf("Removing task %q...\n", name)

	branchName := resolved.BranchPrefix + name
	if err := git.RemoveWorktree(mainWorktree, worktreeDir); err != nil {
		return err
	}
	if err := git.DeleteBranch(mainWorktree, branchName); err != nil {
		return err
	}

	tmux.KillWindow(name)

	cleanEmptyParents(worktreeDir, resolved.WorktreeDirectory)

	fmt.Printf("Task %q removed\n", name)
	return nil
}

func List(opts ListOpts) ([]WorktreeInfo, error) {
	dir := opts.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	base := globalCfg.WorktreeDirectory

	if opts.All {
		return listAll(base, globalCfg.BranchPrefix)
	}

	mainWorktree, err := git.GetMainWorktree(dir)
	if err != nil {
		return nil, err
	}

	orgRepo, err := git.GetOrgRepo(mainWorktree)
	if err != nil {
		return nil, err
	}

	return listRepo(base, orgRepo, globalCfg.BranchPrefix)
}

func listRepo(base string, orgRepo git.OrgRepo, prefix string) ([]WorktreeInfo, error) {
	repoDir := filepath.Join(base, orgRepo.Org, orgRepo.Repo)
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var infos []WorktreeInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		infos = append(infos, WorktreeInfo{
			Name:      name,
			Branch:    prefix + name,
			Dir:       filepath.Join(repoDir, name),
			HasWindow: tmux.WindowExists(name),
			OrgRepo:   orgRepo.String(),
		})
	}
	return infos, nil
}

func listAll(base string, prefix string) ([]WorktreeInfo, error) {
	var infos []WorktreeInfo

	orgs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, org := range orgs {
		if !org.IsDir() {
			continue
		}
		repos, err := os.ReadDir(filepath.Join(base, org.Name()))
		if err != nil {
			continue
		}
		for _, repo := range repos {
			if !repo.IsDir() {
				continue
			}
			orgRepo := git.OrgRepo{Org: org.Name(), Repo: repo.Name()}
			repoInfos, err := listRepo(base, orgRepo, prefix)
			if err != nil {
				continue
			}
			infos = append(infos, repoInfos...)
		}
	}

	return infos, nil
}

func inferTaskName(cwd, worktreeBase string, orgRepo git.OrgRepo) string {
	repoBase := filepath.Join(worktreeBase, orgRepo.Org, orgRepo.Repo)
	if !strings.HasPrefix(cwd, repoBase+"/") {
		return ""
	}
	remainder := strings.TrimPrefix(cwd, repoBase+"/")
	parts := strings.SplitN(remainder, "/", 2)
	return parts[0]
}

func generateName() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func cleanEmptyParents(dir, stopAt string) {
	parent := filepath.Dir(dir)
	for parent != stopAt && parent != "/" {
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(parent)
		parent = filepath.Dir(parent)
	}
}
