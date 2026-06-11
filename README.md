# wktr

A CLI tool to manage git worktrees with automatic window and pane configuration in your terminal multiplexer (tmux or herdr).

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
# Create a new worktree with a window in your multiplexer
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

# Which multiplexer to open windows in: tmux, herdr, or auto (default: auto)
multiplexer: auto

# Layout for repos that don't set their own
layout:
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

### Multiplexer selection

The `multiplexer` key controls which terminal multiplexer `create` and `resume` open windows in. Valid values:

| Value | Behavior |
|-------|----------|
| `auto` (default) | Detect the multiplexer wktr is running inside and use it |
| `tmux` | Always use tmux; only checks that you are inside a tmux session |
| `herdr` | Always use herdr; only checks that you are inside a herdr session |

Auto-detection looks at the environment: `HERDR_ENV=1` means you are inside herdr, and a non-empty `TMUX` variable means you are inside tmux.

- Inside exactly one of them, `auto` picks that one.
- Inside neither, `create` and `resume` fail with an error naming both supported multiplexers.
- Inside both (nested multiplexers, where one inherits the other's environment variables), wktr refuses to guess and asks you to pin `multiplexer: tmux` or `multiplexer: herdr` in your config.

The key resolves at every config level with the same per-key fallthrough as `layout`, so a repo can commit a choice in `.wktr.yaml`, you can override it personally in `.wktr.local.yaml`, and a permanent pin can live in the global config. Invalid values fail at config load time with an error naming the valid options.

Only `create` and `resume` resolve the setting; `remove` and `list` never need it. Note that `remove` and `list` currently only see tmux windows, so `wktr remove` does not close a herdr window yet; multiplexer-agnostic cleanup is a planned follow-up.

In herdr, a task's window is a tab labeled with the task name, created in whatever herdr workspace you are currently in. wktr never creates or manages herdr workspaces. Herdr windows currently open with a single default pane; pane layouts are not applied in herdr yet.

### Config precedence

Every level accepts the same per-repo keys (`layout` and `multiplexer`). Each key resolves independently down the levels, and the first level that sets it wins:

1. `.wktr.local.yaml` (highest priority)
2. `.wktr.yaml`
3. Global `repos[org/repo]` entry
4. Global top level
5. Built-in default (a single empty shell pane for `layout`, `auto` for `multiplexer`)

A file that omits a key is transparent for that key. For example, a `.wktr.local.yaml` without a `layout` key doesn't hide the layout in `.wktr.yaml`; resolution just continues to the next level.

`layout` itself is atomic: the winning level's layout applies wholesale, and panes are never merged across levels.

Global-only keys (`worktree_directory`, `branch_prefix`, `repos`) live solely in the global config.

Note: the global `default_layout` key was renamed to `layout`. Configs that still use `default_layout` fail to load with an error pointing at the rename.

### Layout options

A layout has a `direction` and a list of `panes`. `direction` is optional; when set, it must be `vertical` or `horizontal`, and any other value fails at load time.

### Pane options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | `""` | Command to type in the pane |
| `commands` | list | none | Multiple commands (overrides `command`) |
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
- tmux or [herdr](https://herdr.dev) (`create` and `resume` must run inside one of them)
