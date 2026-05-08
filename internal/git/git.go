package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var remotePattern = regexp.MustCompile(`[/:]([^/]+)/([^/.]+?)(?:\.git)?$`)

type OrgRepo struct {
	Org  string
	Repo string
}

func (or OrgRepo) String() string {
	return or.Org + "/" + or.Repo
}

func ParseRemoteURL(url string) (OrgRepo, error) {
	matches := remotePattern.FindStringSubmatch(url)
	if matches == nil {
		return OrgRepo{}, fmt.Errorf("cannot parse org/repo from remote URL: %s", url)
	}
	return OrgRepo{Org: matches[1], Repo: matches[2]}, nil
}

func GetRemoteURL(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get remote URL: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func GetOrgRepo(dir string) (OrgRepo, error) {
	url, err := GetRemoteURL(dir)
	if err != nil {
		return OrgRepo{}, err
	}
	return ParseRemoteURL(url)
}

func GetRepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

func GetMainWorktree(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			return strings.TrimPrefix(line, "worktree "), nil
		}
	}
	return "", fmt.Errorf("could not find main worktree")
}

func CreateWorktree(repoDir, worktreeDir, branchName, from string) error {
	args := []string{"-C", repoDir, "worktree", "add", worktreeDir, "-b", branchName}
	if from != "" {
		args = append(args, from)
	}
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create worktree: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func RemoveWorktree(repoDir, worktreeDir string) error {
	cmd := exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", worktreeDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		if _, statErr := os.Stat(worktreeDir); os.IsNotExist(statErr) {
			return fmt.Errorf("failed to remove worktree: %s", strings.TrimSpace(string(out)))
		}
		if err := os.RemoveAll(worktreeDir); err != nil {
			return fmt.Errorf("failed to remove worktree directory: %w", err)
		}
		pruneCmd := exec.Command("git", "-C", repoDir, "worktree", "prune")
		if out, err := pruneCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to prune worktrees: %s", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func DeleteBranch(repoDir, branchName string) error {
	cmd := exec.Command("git", "-C", repoDir, "branch", "-D", branchName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete branch: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func BranchExists(repoDir, branchName string) bool {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", "refs/heads/"+branchName)
	return cmd.Run() == nil
}

func HasUncommittedChanges(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func WorktreeDir(base string, or OrgRepo, taskName string) string {
	return filepath.Join(base, or.Org, or.Repo, taskName)
}
