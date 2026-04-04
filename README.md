# devlake-cli

CLI for Apache DevLake

A thin CLI to interact with Apache DevLake APIs, orchestrate pipeline runs, and export domain layer data to SQLite.

## Prerequisites

- Apache DevLake running (API at `http://localhost:8080` by default)
- MySQL with DevLake's database (DSN: `merico:merico@tcp(localhost:3306)/lake` by default)
- Go 1.21+ and CGO enabled (for SQLite support)

## Build

```sh
go build -o devlake-cli .
```

## Usage

```sh
# List connections for a plugin
devlake-cli connections list github

# List blueprints
devlake-cli blueprints list

# Trigger a blueprint
devlake-cli blueprints trigger 1

# Trigger a blueprint and wait for completion
devlake-cli run 1

# Export domain layer tables to SQLite
devlake-cli export -o devlake.db

# Export specific tables only
devlake-cli export -o devlake.db -t commits,pull_requests,issues
```

## Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--api-url` | `http://localhost:8080` | DevLake API base URL |
| `--db-dsn` | `merico:merico@tcp(localhost:3306)/lake` | DevLake MySQL DSN |

## Architecture

```
devlake-cli
├── main.go                      # Entry point
├── cmd/                         # CLI commands (cobra)
│   ├── root.go                  # Root command and global flags
│   ├── connections.go           # connections list <plugin>
│   ├── blueprints.go            # blueprints list / trigger
│   ├── pipelines.go             # pipelines list / status
│   ├── run.go                   # run (trigger + poll)
│   └── export.go                # export MySQL → SQLite
└── internal/
    ├── api/
    │   └── client.go            # DevLake REST API client
    └── export/
        └── sqlite.go            # MySQL → SQLite export logic
```

devlake-cli stays thin: DevLake is the ETL and normalization engine. The CLI orchestrates runs via the DevLake API and exports the final normalized data into SQLite for portable querying.

## License

AGPL-3.0
