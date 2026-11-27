# Arcadia

A game launcher and organizer for Windows, inspired by LaunchBox, focused on **robust play time tracking**: time is
counted from the game's process tree, not from the focused window.

Current version: **1.3.1**.

## What it does

- **Library** in three columns: platforms and filters on the left, games in the middle, game details on the right.
- **Play time** counted by the agent, with pause by keyboard (`Ctrl+Alt+P`) or controller (`Select + Y` held for 1 s); `Select + D-pad up/down` changes the Windows volume
  and the option to start a game without counting time.
- **Import** of executables, ROM folders (with subfolders and grouped discs) and the LaunchBox library, with a review
  window before anything is saved.
- **Emulators** per platform or per game.
- **Metadata and images** fetched from IGDB, SteamGridDB and Steam, and video from ScreenScraper. Each provider's keys
  are entered inside the application.
- **Console mode** (Big Box): a full-screen view navigable with the controller, opened with `F11`. `Select + B` held
  for 1 s closes the running game, and `Select + A` held for 1 s opens the Big Box (also `arcadia.exe --bigbox`), with
  the Strip, Mosaic and Shelf themes.
- **Time to beat** (main story, main + extras, completionist) from HowLongToBeat, with IGDB as fallback.
- **Manual editing** of game data, images (cover, screenshots, background) and total play time.
- **Session editing**: add, edit and delete the sessions of a game (start, end and played time) from the details panel,
  keeping the total, first and last played and the session count consistent.
- **Statistics** of play time by period.

## How to use

1. Download `arcadia.exe` from the [releases page](https://github.com/angelo848/arcadia/releases).
2. Run it. Windows asks for administrator permission: Arcadia runs elevated so it can start and track games that also
   require elevation.
3. An icon appears in the system tray. That is the agent, which keeps counting time even with the window closed.
   "Sair" (Quit) in the tray menu closes everything.

Requirements: Windows 10 or 11, with WebView2 (already included in Windows 11).

The interface is in Brazilian Portuguese.

The library is **portable**: the database, images and logs live in the `data` folder next to the executable. To use
another folder, pass `--data-dir <folder>` or set the `ARCADIA_DATA_DIR` environment variable.

## How it works

One executable, two processes:

- `arcadia.exe --agent` stays in the tray. It launches games, tracks time, reads the shortcuts and is the only writer
  of the database.
- `arcadia.exe` is the interface. It asks the agent for everything over a local named pipe (JSON-RPC 2.0) and never
  influences play time.

Stack: Go 1.27, Wails v2 with React, TypeScript and Vite, SQLite via `modernc.org/sqlite` (no CGO).

More details in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md), in the recorded decisions in [docs/adr/](docs/adr/README.md)
and in the [roadmap](docs/ROADMAP.md). These documents are written in Portuguese.

## Development

Prerequisites: Go 1.27, Node.js and the [Wails v2](https://wails.io) CLI (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```bash
go build ./...
go test ./...
go vet ./...
```

The agent alone, with its data in a development folder:

```bash
go run ./cmd/arcadia --agent --data-dir ./.devdata
```

Interface in development mode (from `cmd/arcadia`):

```bash
wails dev
```

Build the executable into `cmd/arcadia/build/bin/arcadia.exe` (from `cmd/arcadia`):

```bash
wails build -ldflags "-X main.version=1.3.1"
```

Interface tests (from `frontend`):

```bash
npx vitest run
```

After changing the queries in `internal/store/queries/`, run `sqlc generate`.

### Layout

```
cmd/arcadia      executable (interface and agent)
cmd/fake-game    fake game used by the tracking tests
internal/        agent, domain, database, tracker, input, IPC, providers, importers
frontend/        React interface
scripts/         icon generation and spec checking
docs/            architecture, ADRs, roadmap and the spec of each phase
```

### Process

The project follows Spec-Driven Development ([ADR 0008](docs/adr/0008-spec-driven-development.md)): each phase has a
spec in `docs/specs/<phase>/` with numbered requirements, and each test names the requirement it covers. Code
conventions and the workflow are in [CLAUDE.md](CLAUDE.md).

The icons are original drawings, with no manufacturer logos. To regenerate them, see `scripts/gen-icons.mjs` and
`scripts/gen-icon.go`.
