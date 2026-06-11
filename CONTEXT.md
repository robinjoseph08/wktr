# wktr

A CLI that turns git worktrees into disposable working environments: each unit of work gets its own branch, directory, and terminal window.

## Language

**Task**:
A named unit of work, realized as a git branch plus a Worktree, optionally open in a Window per Multiplexer.
_Avoid_: feature, job, ticket

**Worktree**:
The git working copy backing a Task, namespaced by org/repo under the worktree directory.
_Avoid_: checkout, clone

**Window**:
The Multiplexer surface opened for a Task, holding its Panes.
_Avoid_: tab (herdr's word for its realization of a Window). One carve-out: herdr backend error messages and docs explaining the herdr mapping say "tab" because they describe the concrete herdr surface, and herdr has its own unrelated window concept that "herdr window" would collide with.

**Multiplexer**:
The terminal program (tmux or herdr) that hosts Windows.
_Avoid_: backend (reserved for the Go implementations inside wktr)

**Pane**:
One subdivision of a Window, optionally primed with or running configured commands.

**Layout**:
The configured arrangement of Panes (direction, sizes, commands, focus) applied when a Window opens.
_Avoid_: template, preset

### Configuration

**Global config**:
The user-level config file at `~/.config/wktr/config.yaml`, the outermost level of the hierarchy.

**Repo config**:
The committed `.wktr.yaml` at a repo's root, shared by everyone who clones it.

**Local config**:
The `.wktr.local.yaml` at a repo's root, holding personal machine-specific overrides and meant to stay uncommitted.

## Relationships

- A **Task** owns exactly one branch and one **Worktree**
- A **Window** is hosted by exactly one **Multiplexer**
- A **Task** may have **Windows** open in more than one **Multiplexer** at once (at most one per Multiplexer); removing the Task closes all of them
- A **Window** contains one or more **Panes**, arranged by a **Layout**
- Settings resolve per key: **Local config** over **Repo config** over **Global config** (ADR-0003 records the full five-level chain)

## Example dialogue

> **Dev:** "I created a **Task** in tmux on Monday. Today I'm inside herdr and run resume. What opens?"
>
> **Domain expert:** "A fresh **Window** in herdr with the **Layout** applied. The tmux Window keeps running; a Task can be open in both **Multiplexers**, and removing the Task cleans up both."

## Flagged ambiguities

- "tab" (herdr's word) was used interchangeably with "window" (tmux's word) for the per-Task surface. Resolved: **Window** is the canonical term regardless of Multiplexer.
- "backend" and "multiplexer" were used interchangeably. Resolved: **Multiplexer** is the domain term; "backend" refers only to the Go implementations and stays out of user-facing language.

## Status

Parts of this model are decided (see `docs/adr/`) but not yet implemented: applying a Layout in herdr (herdr Windows open with a single default Pane), Multiplexer-agnostic cleanup (`remove` and `list` only see tmux Windows today, so a Task open in both Multiplexers is only partly cleaned up), and honoring a Layout's direction (direction is validated at load but not yet applied). `create` and `resume` select between tmux and herdr per ADR-0002.
