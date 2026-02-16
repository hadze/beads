# beads-cost

Standalone cost tracking sidecar for [beads](https://github.com/steveyegge/beads). Tracks LLM session costs in a local SQLite database, independent of the main Dolt-backed issue store.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Git Repository                      │
│                                                          │
│  ┌──────────────┐          ┌──────────────────────────┐  │
│  │     bd       │          │      beads-cost          │  │
│  │  (main CLI)  │          │      (sidecar)           │  │
│  │              │          │                          │  │
│  │  Cobra CLI   │          │  Cobra CLI               │  │
│  │  ┌────────┐  │          │  ┌────────────────────┐  │  │
│  │  │ create │  │          │  │ track │ status     │  │  │
│  │  │ list   │  │          │  │ sessions │ show    │  │  │
│  │  │ show   │  │          │  └────────────────────┘  │  │
│  │  │ ...    │  │          │           │              │  │
│  │  └────────┘  │          │           ▼              │  │
│  │      │       │          │  ┌────────────────────┐  │  │
│  │      ▼       │          │  │  internal/cost     │  │  │
│  │  ┌────────┐  │          │  │  Store (SQLite)    │  │  │
│  │  │  Dolt  │  │          │  └────────┬───────────┘  │  │
│  │  │ Store  │  │          │           │              │  │
│  │  └────┬───┘  │          └───────────┼──────────────┘  │
│  └───────┼──────┘                      │                 │
│          ▼                             ▼                 │
│  .beads/                       .beads/                   │
│  ├── noms/     (Dolt DB)       └── cost.db  (SQLite)     │
│  ├── *.jsonl                                             │
│  └── config.yaml                                         │
└─────────────────────────────────────────────────────────┘
```

**Key design decisions:**

- **Separate binary** — no import dependency on the beads Go module
- **Own `go.mod`** — minimal deps: `ncruces/go-sqlite3` + `spf13/cobra`
- **SQLite with WAL** — concurrent reads, no server needed
- **Shared `.beads/` directory** — `cost.db` lives next to the Dolt DB but doesn't interact with it
- **Extractable** — can be moved to its own GitHub repo at any time

## Data Model

```sql
sessions
├── id              TEXT PRIMARY KEY
├── bead_id         TEXT        -- optional link to a beads issue
├── model           TEXT        -- e.g. "claude-sonnet-4-5-20250929"
├── provider        TEXT        -- e.g. "anthropic"
├── input_tokens    INTEGER
├── output_tokens   INTEGER
├── cost_usd        REAL
├── duration_sec    REAL
├── note            TEXT
├── cache_read      INTEGER
├── cache_creation  INTEGER
└── created_at      TEXT (RFC3339)
```

## Installation

```bash
cd beads-cost
make install    # builds and copies to ~/.local/bin/
```

Or build only:

```bash
make build      # produces ./beads-cost
```

## Usage

### Record a session

```bash
beads-cost track \
  --model claude-sonnet-4-5-20250929 \
  --provider anthropic \
  --input 5000 \
  --output 1200 \
  --cost 0.042 \
  --bead PROJ-42 \
  --note "refactored auth module"
```

### View cost summary

```bash
$ beads-cost status
Sessions:   14
Total cost: $1.2340
Tokens:     245.0K in / 82.3K out
Duration:   12.5m
Period:     2025-01-10 .. 2025-01-15
```

### List sessions

```bash
beads-cost sessions              # last 20 sessions
beads-cost sessions -n 5         # last 5
beads-cost sessions --bead PROJ-42  # filter by bead
```

### Show session detail

```bash
$ beads-cost show s-1234567890
Session:    s-1234567890
Bead:       PROJ-42
Model:      claude-sonnet-4-5-20250929
Provider:   anthropic
Tokens:     5.0K in / 1.2K out
Cost:       $0.0420
Duration:   8.3s
Created:    2025-01-15T10:30:00Z
Note:       refactored auth module
```

### JSON output

All commands support `--json` for machine-readable output:

```bash
beads-cost status --json
beads-cost sessions --json | jq '.[].cost_usd' | paste -sd+ | bc
```

### Custom database location

By default, `beads-cost` walks up from the current directory looking for a `.beads/` folder. Override with:

```bash
beads-cost --db /path/to/.beads status
```

## Development

```bash
make test       # run all tests
make build      # compile binary
make clean      # remove artifacts
```

## License

Same as [beads](https://github.com/steveyegge/beads).
