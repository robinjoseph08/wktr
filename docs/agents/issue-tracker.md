# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. For multi-line bodies, feed a heredoc via `--body-file -` or use `--body "$(cat <<'EOF' ... EOF)"`.
- **Read an issue**: `gh issue view <number> --json title,body,labels,comments` for everything in one structured payload, or plain `gh issue view <number>` for a human-readable view. Note that `--comments` on its own prints only the comment thread, omits the body, and is empty for issues with no comments.
- **List issues**: `gh issue list --state open --limit 1000 --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` adjusting `--state` and adding `--label` filters as needed (the default limit is 30, so always pass `--limit`).
- **Comment on an issue**: `gh issue comment <number> --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --comment "..."`

Infer the repo from `git remote -v`; `gh` does this automatically when run inside a clone.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill needs to read an issue

Run `gh issue view <number> --json title,body,labels,comments`.
