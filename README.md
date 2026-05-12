# gh-search

`gh-search` searches GitHub issues and discussions in one repository.

Input is a repository URL and a query string. Output is either a table for humans or JSON for scripts.

It uses the same auth setup as `gh`. If `gh auth login` works, this works. `GH_TOKEN` works too.

## Install

```bash
go install ./cmd/gh-search
```

## Usage

```bash
gh-search <repo-url> --query <keywords> [--state open|closed|all] [--type issue|discussion|all] [--limit N] [--sort relevance|created|created-asc|created-desc|updated|updated-asc|updated-desc] [--json] [--plain-links]
```

## Flags

- `--query`: required.
- `--state`: filter by state. Default: `all`.
- `--type`: search `issue`, `discussion`, or `all`. Default: `all`.
- `--limit`: max results per selected result type. `0` means no client-side cap.
- `--sort`: `relevance`, `created`, `created-asc`, `created-desc`, `updated`, `updated-asc`, or `updated-desc`.
- `--json`: print JSON.
- `--plain-links`: disable terminal hyperlinks and print a plain `Direct Links` section.

## Examples

```bash
gh-search https://github.com/moby/moby --query "container exit code" --state all
gh-search https://github.com/kubernetes/kubernetes --query "pod pending" --type discussion
gh-search https://github.com/cli/cli --query "extension" --type issue --limit 5 --sort created-asc
gh-search https://github.com/cli/go-gh --query "graphql client" --json --type issue --limit 3
gh-search https://github.com/hyprwm/Hyprland --query "GPU Reset" --state open --limit 4 --plain-links
```

## Notes

- Issues and discussions are queried separately.
- `--limit` applies per selected result type.
- Pagination is automatic.
- Table output uses terminal hyperlinks when possible.
- Piped output falls back to plain URLs.
- GitHub search limits still apply.

