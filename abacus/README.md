# abacus

*Counting beads, one token at a time.*

LLM cost tracking sidecar for [beads](https://github.com/steveyegge/beads). Stores session costs in a local SQLite database, independent of the main Dolt-backed issue store.

## Architecture

```
 github.com/steveyegge/beads          github.com/hadze/abacus
 ─────────────────────────            ────────────────────────────
 ┌──────────────────────┐             ┌──────────────────────────┐
 │        bd            │             │        abacus            │
 │     (main CLI)       │             │       (sidecar)          │
 │                      │             │                          │
 │  100+ commands       │             │  track · status          │
 │  Dolt/MySQL engine   │             │  sessions · show         │
 │  ~200 dependencies   │             │  2 dependencies          │
 │  ~80 MB binary       │             │  ~12 MB binary           │
 │         │            │             │         │                │
 │         ▼            │             │         ▼                │
 │  ┌────────────┐      │             │  ┌────────────────┐      │
 │  │    Dolt    │      │             │  │    SQLite      │      │
 │  │   Store    │      │             │  │    (WAL)       │      │
 │  └─────┬──────┘      │             │  └───────┬────────┘      │
 └────────┼─────────────┘             └──────────┼───────────────┘
          │                                      │
          ▼                                      ▼
   .beads/                                .beads/
   ├── noms/          ◄── no coupling ──► └── cost.db
   ├── *.jsonl
   └── config.yaml
```

Two separate repos, two separate binaries, zero shared code.
The only convention they share is the `.beads/` directory.

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

Schema is versioned (`schema_version` table) for forward-compatible migrations.

## Installation

```bash
make install    # builds and copies to ~/.local/bin/
```

Or build only:

```bash
make build      # produces ./abacus
```

## Usage

### Record a session

```bash
abacus track \
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
$ abacus status
Sessions:   14
Total cost: $1.2340
Tokens:     245.0K in / 82.3K out
Duration:   12.5m
Period:     2025-01-10 .. 2025-01-15
```

### List sessions

```bash
abacus sessions              # last 20
abacus sessions -n 5         # last 5
abacus sessions --bead PROJ-42  # filter by bead
```

### Show session detail

```bash
$ abacus show s-1234567890
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
abacus status --json
abacus sessions --json | jq '.[].cost_usd' | paste -sd+ | bc
```

### Custom database location

By default, `abacus` walks up from the current directory looking for a `.beads/` folder. Override with:

```bash
abacus --db /path/to/.beads status
```

## Development

```bash
make test       # run all tests
make build      # compile binary
make clean      # remove artifacts
```

## License

Same as [beads](https://github.com/steveyegge/beads).
