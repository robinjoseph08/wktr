package workspace

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/robinjoseph08/wktr/internal/config"
	"github.com/robinjoseph08/wktr/internal/git"
	"github.com/robinjoseph08/wktr/internal/multiplexer"
)

var validTaskName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

type CreateOpts struct {
	Name string
	From string
	Dir  string
}

type ResumeOpts struct {
	Name string
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

// MultiplexerSelector picks the Multiplexer backend for a resolved
// multiplexer config value. Only Create and Resume resolve the setting
// (ADR-0002); Remove and List never do.
type MultiplexerSelector func(value string) (multiplexer.Multiplexer, error)

func Create(selectMux MultiplexerSelector, opts CreateOpts) error {
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

	resolved, err := config.Resolve(globalCfg, mainWorktree, orgRepo.String())
	if err != nil {
		return err
	}

	mux, err := selectMux(resolved.Multiplexer)
	if err != nil {
		return err
	}

	name := opts.Name
	if name != "" {
		if !validTaskName.MatchString(name) {
			return fmt.Errorf("invalid task name %q: must start with alphanumeric and contain only alphanumeric, hyphens, or underscores", name)
		}
	} else {
		var err error
		name, err = generateUniqueName(mainWorktree, resolved.BranchPrefix)
		if err != nil {
			return err
		}
	}

	branchName := resolved.BranchPrefix + name
	if opts.Name != "" && git.BranchExists(mainWorktree, branchName) {
		return fmt.Errorf("branch %q already exists; use a different name or run `wktr remove %s` first", branchName, name)
	}

	worktreeDir := git.WorktreeDir(resolved.WorktreeDirectory, orgRepo, name)

	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0o755); err != nil {
		return fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	fmt.Printf("Creating worktree at %s...\n", worktreeDir)
	if err := git.CreateWorktree(mainWorktree, worktreeDir, branchName, opts.From); err != nil {
		return err
	}

	fmt.Println("Opening window...")
	if err := mux.OpenWindow(name, worktreeDir, resolved.Layout); err != nil {
		return err
	}

	fmt.Printf("Task %q started in a new window\n", name)
	return nil
}

func Resume(selectMux MultiplexerSelector, opts ResumeOpts) error {
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

	resolved, err := config.Resolve(globalCfg, mainWorktree, orgRepo.String())
	if err != nil {
		return err
	}

	mux, err := selectMux(resolved.Multiplexer)
	if err != nil {
		return err
	}

	name := opts.Name
	if name == "" {
		name = inferTaskName(dir, resolved.WorktreeDirectory, orgRepo)
		if name == "" {
			return fmt.Errorf("no task name provided and not in a worktree directory")
		}
	} else if !validTaskName.MatchString(name) {
		return fmt.Errorf("invalid task name %q: must start with alphanumeric and contain only alphanumeric, hyphens, or underscores", name)
	}

	worktreeDir := git.WorktreeDir(resolved.WorktreeDirectory, orgRepo, name)
	if _, err := os.Stat(worktreeDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("worktree directory not found: %s; use `wktr create %s` to create it first", worktreeDir, name)
		}
		return fmt.Errorf("failed to access worktree directory: %w", err)
	}

	if mux.WindowExists(name) {
		fmt.Printf("Window %q already exists, switching to it...\n", name)
		return mux.FocusWindow(name)
	}

	fmt.Println("Opening window...")
	if err := mux.OpenWindow(name, worktreeDir, resolved.Layout); err != nil {
		return err
	}

	fmt.Printf("Task %q resumed in a new window\n", name)
	return nil
}

// Remove deletes a Task's Worktree and branch and best-effort kills its
// Window in every Multiplexer. Tasks outlive Multiplexer sessions (ADR-0002),
// so muxes is every backend rather than a selected one, and a missing Window
// or unreachable Multiplexer is silently ignored. The current Multiplexer's
// Window is killed last, after all other cleanup, because that kill can take
// wktr down with it when wktr runs inside the Task's own Window.
func Remove(muxes []multiplexer.Multiplexer, opts RemoveOpts) error {
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

	// Remove only needs global-only keys, so it skips Resolve. This keeps
	// removal working even when a repo's .wktr.yaml or .wktr.local.yaml is
	// invalid, since removal never reads the layout.
	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	name := opts.Name
	if name == "" {
		name = inferTaskName(dir, globalCfg.WorktreeDirectory, orgRepo)
		if name == "" {
			return fmt.Errorf("no task name provided and not in a worktree directory")
		}
	}

	worktreeDir := git.WorktreeDir(globalCfg.WorktreeDirectory, orgRepo, name)
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
		_, _ = fmt.Scanln(&response)
		if !strings.HasPrefix(strings.ToLower(response), "y") {
			fmt.Println("Cancelled")
			return nil
		}
	}

	fmt.Printf("Removing task %q...\n", name)

	branchName := globalCfg.BranchPrefix + name
	if err := git.RemoveWorktree(mainWorktree, worktreeDir); err != nil {
		return err
	}
	if err := git.DeleteBranch(mainWorktree, branchName); err != nil {
		return err
	}

	// Kill the Window in every Multiplexer we are not inside first. Killing
	// the current Multiplexer's Window can take wktr down with it (when wktr
	// runs inside the Task's own Window), so that kill happens last, once
	// every other backend is cleaned up and the remaining work is done.
	var current []multiplexer.Multiplexer
	for _, mux := range muxes {
		if mux.Detect() {
			current = append(current, mux)
			continue
		}
		mux.KillWindow(name)
	}

	cleanEmptyParents(worktreeDir, globalCfg.WorktreeDirectory)

	fmt.Printf("Task %q removed\n", name)

	for _, mux := range current {
		mux.KillWindow(name)
	}

	return nil
}

// List reports the current repo's Tasks' Worktrees, or every repo's with
// opts.All, along with whether each Task has a Window open. Tasks outlive
// Multiplexer sessions (ADR-0002), so muxes is every backend rather than a
// selected one, and a Window counts as open when any backend has one.
func List(muxes []multiplexer.Multiplexer, opts ListOpts) ([]WorktreeInfo, error) {
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
		return listAll(muxes, base, globalCfg.BranchPrefix)
	}

	mainWorktree, err := git.GetMainWorktree(dir)
	if err != nil {
		return nil, err
	}

	orgRepo, err := git.GetOrgRepo(mainWorktree)
	if err != nil {
		return nil, err
	}

	return listRepo(muxes, base, orgRepo, globalCfg.BranchPrefix)
}

// windowExistsAny reports whether any backend has a Window named name.
func windowExistsAny(muxes []multiplexer.Multiplexer, name string) bool {
	for _, mux := range muxes {
		if mux.WindowExists(name) {
			return true
		}
	}
	return false
}

func listRepo(muxes []multiplexer.Multiplexer, base string, orgRepo git.OrgRepo, prefix string) ([]WorktreeInfo, error) {
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
			HasWindow: windowExistsAny(muxes, name),
			OrgRepo:   orgRepo.String(),
		})
	}
	return infos, nil
}

func listAll(muxes []multiplexer.Multiplexer, base string, prefix string) ([]WorktreeInfo, error) {
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
			repoInfos, err := listRepo(muxes, base, orgRepo, prefix)
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

func generateUniqueName(repoDir, branchPrefix string) (string, error) {
	for range 10 {
		name := generateName()
		if !git.BranchExists(repoDir, branchPrefix+name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("failed to generate a unique task name after 10 attempts")
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
		_ = os.Remove(parent)
		parent = filepath.Dir(parent)
	}
}
