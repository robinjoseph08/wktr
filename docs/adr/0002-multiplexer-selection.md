# Multiplexer selection: auto-detect, error on ambiguity, Multiplexer-agnostic cleanup

The `multiplexer` setting defaults to `auto`: inside herdr (`HERDR_ENV=1`) use herdr, inside tmux (`TMUX` set) use tmux, and being inside neither is an error. When both signals are present, wktr errors and asks the user to pin `multiplexer` in config rather than silently picking one, because env vars inherit through nesting (tmux inside herdr and herdr inside tmux look identical), so any hardcoded precedence sends Windows to the wrong Multiplexer for someone.

## Consequences

- Only `create` and `resume` resolve the setting, since they must build a Window somewhere. `remove` and `list` never resolve it: Tasks outlive Multiplexer sessions, so `remove` best-effort kills the Task's Window in every Multiplexer and `list` reports a Window as open if it exists in any of them.
- `resume` acts only on the current Multiplexer; a Task opened elsewhere gets a fresh Window here, and both are cleaned up at remove time.
- wktr never manages herdr workspaces. Windows are created as herdr tabs in whatever workspace the user is in, which composes with organizing herdr as one workspace per repo without wktr owning that lifecycle.
