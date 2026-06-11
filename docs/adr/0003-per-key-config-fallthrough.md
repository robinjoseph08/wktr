# Per-key config fallthrough across the hierarchy

Each setting (`layout`, `multiplexer`) resolves independently down Local config, Repo config, global `repos:` entry, global top level, then built-in default; a file that omits a key is transparent for that key. The previous behavior let the mere existence of `.wktr.local.yaml` shadow `.wktr.yaml` entirely, so a local file holding only a personal `multiplexer` pin would have silently erased a repo's committed layout.

As part of making every level accept the same per-repo settings (global-only keys such as `worktree_directory`, `branch_prefix`, and `repos` are unaffected), the global `default_layout` key was renamed to `layout`. The old key triggers a hard load-time error instead of a silent ignore (which would quietly reset users to the built-in layout) or a permanent alias.
