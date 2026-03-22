# disk-cleaner

Recursively scans `~/go/src` for git repositories and deletes build artifact directories that are ignored by `.gitignore`. Also removes `node_modules` directories unconditionally.

## Target directories

| Directory | Condition |
|-----------|-----------|
| `dist` | Deleted if ignored by `.gitignore` |
| `bin` | Deleted if ignored by `.gitignore` |
| `.go` | Deleted if ignored by `.gitignore` |
| `node_modules` | Always deleted |

## Install

```bash
go install github.com/appscodelabs/disk-cleaner@latest
```

Or build locally:

```bash
go build -o disk-cleaner .
```

## Usage

```bash
# Preview what would be deleted and total disk space savings
./disk-cleaner --dry-run

# Delete directories
./disk-cleaner

# Show detailed output
./disk-cleaner --verbose
```

## Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview deletions and show disk space that will be freed |
| `--verbose` | Print detailed output including skipped directories |
| `-h, --help` | Show help |

## How it works

1. Walks `~/go/src` to find git repositories (at depth `hosting/org/repo`, e.g. `github.com/user/repo`)
2. For each repo, recursively searches for target directories
3. Uses `git check-ignore` to verify the directory is listed in `.gitignore`
4. Deletes matching directories and reports total disk space freed
5. `node_modules` is deleted without checking `.gitignore`

## Requirements

- Git must be installed and available on `PATH`
