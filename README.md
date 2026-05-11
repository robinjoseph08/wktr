# wktr

A CLI tool to manage git worktrees with automatic tmux window and pane configuration.

## Install

### Homebrew

```bash
brew install robinjoseph08/tap/wktr
```

### Go

```bash
go install github.com/robinjoseph08/wktr@latest
```

## Usage

```bash
# Create a new worktree with a tmux window
wktr create my-feature

# Create with auto-generated name
wktr create

# Create from a specific branch/ref
wktr create my-feature --from main

# List worktrees for the current repo
wktr list

# List all worktrees across all repos
wktr list --all

# Remove a worktree (with confirmation)
wktr remove my-feature

# Remove without confirmation
wktr remove my-feature --force

# Remove the current worktree (inferred from cwd)
wktr remove
```

## Configuration

### Global config

`~/.config/wktr/config.yaml`

```yaml
# Where worktrees are stored (default: ~/.worktrees)
worktree_directory: ~/.worktrees

# Branch name prefix (default: wktr/)
branch_prefix: wktr/

# Default layout for repos without a .wktr.yaml
default_layout:
  direction: vertical
  panes:
    - command: "claude --dangerously-skip-permissions"
      run: true
      focus: true
      size: 34
    - command: ""
      size: 33
    - command: ""
      size: 33

# Per-repo layouts (for repos you don't own)
repos:
  shishobooks/shisho:
    layout:
      direction: vertical
      panes:
        - command: "claude --dangerously-skip-permissions"
          run: true
          focus: true
        - command: "mise start"
          run: false
        - command: "mise setup"
          run: true

  shishobooks/plugins:
    layout:
      direction: vertical
      panes:
        - command: "claude --dangerously-skip-permissions"
          run: true
          focus: true
        - command: "pnpm start"
          run: false
        - command: "pnpm install"
          run: true
```

### Per-repo config

`.wktr.yaml` (committed to repo)

```yaml
layout:
  direction: vertical
  panes:
    - command: "claude --dangerously-skip-permissions"
      run: true
      focus: true
    - commands:
        - value: "npm install"
          run: true
        - value: "npm run dev"
          run: false
    - command: ""
```

`.wktr.local.yaml` (gitignored, personal overrides)

```yaml
layout:
  direction: vertical
  panes:
    - command: "claude --dangerously-skip-permissions"
      run: true
      focus: true
      size: 50
    - command: "npm run dev"
      run: false
    - command: "npm install"
      run: true
```

### Config precedence

1. `.wktr.local.yaml` (highest priority)
2. `.wktr.yaml`
3. Global `repos[org/repo]` entry
4. Global `default_layout`
5. Built-in default (single empty shell pane)

Each level fully replaces the layout — no merging.

### Pane options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | `""` | Command to type in the pane |
| `commands` | list | — | Multiple commands (overrides `command`) |
| `run` | bool | `true` | Whether to execute the command (false = type only) |
| `size` | int | even split | Pane height as percentage |
| `focus` | bool | `false` | Whether this pane gets focus after creation |

### Commands list format

For panes that need to run a command and then prime another:

```yaml
panes:
  - commands:
      - value: "npm install"
        run: true
      - value: "npm start"
        run: false
```

Commands with `run: true` are chained with `&&`. The final `run: false` command is typed after the chain completes.

## Directory structure

Worktrees are stored at:

```
{worktree_directory}/{org}/{repo}/{task_name}
```

For example:

```
~/.worktrees/
  shishobooks/
    shisho/
      fix-auth/
      add-logging/
    plugins/
      update-deps/
  robinjoseph08/
    wktr/
      new-feature/
```

## Releasing

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions. Pushing a version tag triggers the pipeline, which builds binaries for linux/darwin (amd64/arm64), creates a GitHub release, and updates the Homebrew tap.

```bash
# Tag the release (replace with the desired version)
git tag vX.Y.Z

# Push the tag to trigger the release workflow
git push origin vX.Y.Z
```

The workflow builds the binaries, publishes a GitHub release with archives, and pushes a formula update to [robinjoseph08/homebrew-tap](https://github.com/robinjoseph08/homebrew-tap) so `brew upgrade wktr` picks up the new version.

## Requirements

- git
- tmux (must be run inside a tmux session)
