# NATS Desktop

> **Vibecoded** — Built with AI assistance, following [NATS CLI](https://github.com/nats-io/natscli) patterns

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)]()

**A cross-platform desktop GUI for [NATS](https://nats.io)** — the lightweight, high-performance messaging system for cloud-native applications, IoT messaging, and microservices architectures.

Manage servers, JetStream streams, KV stores, and more — with a visual interface that follows familiar NATS CLI patterns.

---

## Why NATS Desktop?

Official NATS tools are powerful but often require command-line expertise. Existing attempts at GUIs are either too basic or cumbersome to use (looking at you, nats-nui) or behind a paywall. NATS Desktop is free and open source. All known clients are listed on the official download page <https://nats.io/download/>

| Feature            | NATS CLI       | NATS Desktop                        |
| ------------------ | -------------- | ----------------------------------- |
| Learning curve     | Moderate       | Gentle                              |
| Visual feedback    | Text-based     | Real-time GUI                       |
| Message browsing   | Limited        | Full visual browser                 |
| Cluster monitoring | Commands       | Live dashboard                      |
| Configuration      | Context files  | GUI + file compatible               |
| Best for           | Scripts, CI/CD | Human daily operations, exploration |

**NATS CLI Compatibility**: This project reads/writes standard NATS context files (`~/.config/nats/context/`), so you can switch between CLI and GUI seamlessly.

Screenshots are in the [screenshots/](screenshots/) directory.

---

## Quick Start

### 1. Download the latest release

<https://github.com/nats-io/nats-desktop/releases>

### 2. Start a local NATS server

You can download and run the NATS server from <https://github.com/nats-io/nats-server/releases/latest> or use [mise-en-place](https://github.com/jdx/mise/releases/latest) in a clone of this repository:

```bash
## Install nats-server and nats cli
mise up
## Start nats-server with jetstream enabled in local ./tmp
mise run nats-server
```

### 3. Connect

1. Click **"New Connection"**
2. Enter URL: `nats://localhost:4222`
3. (Optional) Add authentication
4. **Connect** and start exploring!

### 4. Embedded NATS Server for Testing

**"Start Embedded"** to launch a local NATS server for testing and development. This is ideal for trying out features without needing an external server.

---

## Features

### Connection Management

- Manage multiple NATS server contexts
- **NATS CLI compatible** — reads/writes `~/.config/nats/context/*.json`
- Authentication methods:
  - Username/Password
  - Token
  - NKeys
  - Credentials files
- Import/export contexts as JSON
- Connection status monitoring

### Cluster Monitoring

- Real-time server list with status indicators
- Server ping with RTT visualization
- Server information modal with detailed metrics
- Cluster topology visualization
- Auto-refresh with configurable intervals

### Stream Management (JetStream)

- Full CRUD operations for streams
- Stream creation with templates (File, Memory storage)
- **Message browser**:
  - Live-tail mode
  - Message history with filters
  - JSON syntax highlighting
  - Hex dump view
- Stream statistics and configuration
- Operations: duplicate, purge, seal
- Stream relations visualization

### Consumer Management

- Consumer list view with status
- Consumer creation wizard
- Detail view with configuration
- Pause/resume controls (NATS 2.11+)
- Reset/seek functionality
- Delete with cascade warnings

### Key-Value Stores

- KV store list with statistics
- **Browser with tree view**
- Search and filter keys
- Value display (text, JSON)
- History viewer
- Watch mode (real-time updates)
- Key-value editor
- KV-Stream relations

### Object Stores

- Object store list
- File-browser UI
- Metadata display
- Sort and search
- Upload with progress bar
- Download with progress
- Create/edit stores

### Services

- Service list with health status
- Service detail view:
  - Statistics dashboard
  - Endpoint list
  - Instance health
- Performance charts
- Latency histogram
- Service invocation tool

### Pub/Sub Tools

**Publisher**:

- Subject autocomplete
- Payload editor
- Header editor
- Batch publish
- Publish from file
- Message templates: `{{.Count}}`, `{{.TimeStamp}}`, `{{.UUID}}`

**Subscriber**:

- Queue groups support
- Wildcard subscriptions
- Match replies
- Message acknowledgment
- Message history and re-send

**Request-Reply**:

- Interactive request/response
- Timeout configuration
- Response viewing

**Reply Service**:

- Auto-respond to requests
- Template responses with token substitution
- Command execution support

### Benchmarking

- **Pub Benchmark**: Message publishing performance
- **Sub Benchmark**: Message subscription performance
- **JetStream Benchmark**: Stream-based performance
- **Service Benchmark**: Service invocation performance
- **Latency Test**: RTT visualization with P50/P95/P99 percentiles
- Live execution with progress
- Final statistics summary

### Events & Monitoring

- Event stream viewer:
  - Lifecycle events
  - JetStream advisories
  - Metric events
  - Filter and search
- Health check dashboard:
  - Stream health monitoring
  - Consumer health monitoring
  - Threshold configuration
  - Alerts panel

### Backup & Restore

- Stream backup with progress tracking
- Restore from backup files
- Configurable block size
- Resume interrupted operations

### Schema Management

- Schema registry viewer
- Schema validation
- Schema versioning

### Account Info

- Account statistics dashboard
- Resource limits and usage
- Stream/KV relations graph
- Usage trends

### Preferences

- Theme selection (Light/Dark)
- Auto-refresh intervals
- Font size settings
- Preferences persistence

### Keyboard and mouse navigation

- Basic keyboard navigation, tab to switch focus, enter to activate, escape to close modals.
- Some table/lists have double click to open details, <kbd>del</kbd>/<kbd>shift+del</kbd> to delete items.

- <kbd>Ctrl+H</kbd> displays local list of shortcuts.

- <kbd>Ctrl+1..9</kbd> to switch to corresponding sidebar item.
- <kbd>Ctrl+Shift+1..7</kbd> to switch to corresponding sidebar item (continued).
- <kbd>Ctrl+,</kbd> to switch to Preferences.

### Logging

Implements a wrapper over [zerolog](https://github.com/rs/zerolog) library. Log is output to the console.

- Log level: `LOG_LEVEL=info`. For all values, see [https://github.com/rs/zerolog/blob/a0d61dc2c7439bdea7e1a9920713b1134367be58/globals.go#L47-L59]
- Log format: `LOG_FORMAT=console`. Other possible value: `json`

---

## Development

- **Go 1.25+** (for building from source)
- **NATS Server 2.10+** with JetStream enabled for full features
- **Platforms**: Linux, macOS, Windows

```bash
## Clone and build
git clone https://github.com/thedataflows/nats-desktop.git
cd nats-desktop
## Run
go run ./cmd/nats-desktop
## Build
go build -o nats-desktop ./cmd/nats-desktop
```

---

## Architecture

Built with:

- **UI Framework**: [Gio](https://gioui.org/) — immediate mode, native performance
- **NATS Go Library**: [nats.go](https://github.com/nats-io/nats.go)
- **Configuration**: `~/.config/nats-desktop.toml`

```tree
nats-desktop/
├── cmd/nats-desktop/       # Application entry point
├── internal/
│   ├── application/        # App state, managers, business logic
│   ├── nats/               # NATS client wrapper
│   ├── views/              # UI screens
│   ├── ui/components/      # Reusable widgets
│   ├── config/             # Configuration management
│   ├── models/             # Data models
├── assets/                 # Fonts, static assets
└── docs/                   # Documentation
```

---

## Testing

```bash
# Run all tests
go test -v ./...

# Run with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Tests use an embedded NATS server for isolation.

---

## About

### Vibecoded

This project was built with AI assistance, combining human design decisions with AI-powered implementation. It is not optimal, but it does work. I did not want to learn gioui, too low level.

It follows patterns from:

- [NATS CLI](https://github.com/nats-io/natscli) — command structure and context format
- [Chapar](https://github.com/chapar-rest/chapar) — UI component patterns

### Philosophy

- **Local-first**: No cloud dependencies, your data stays on your machine
- **CLI-compatible**: Works with existing NATS contexts and configurations
- **Lightweight**: Native Go performance, no heavy runtimes

---

## Contributing

Contributions welcome, just do not expect fast response. Most probably will not accept new feature requests, as this is pretty much feature complete and trying to follow NATS CLI patterns. However, bug fixes and documentation improvements are always appreciated.

---

## Support

- **Issues**: Report bugs on GitHub
- **NATS Docs**: <https://docs.nats.io>
- **NATS Community**: <https://nats.io/slack>

---

## License

[MIT License](LICENSE)
