# AGENTS.md — Go (Gio) GUI app rules

This repository is a Go GUI application built with the Gio UI library, with UX/component patterns inspired by the Chapar project (<https://github.com/chapar-rest/chapar>).

## How this repo uses OpenCode

- OpenCode should treat this file as the shared, repo-root rule set and keep it committed.
- If extra specialist agents are needed (UI, refactor, tests), add per-project agent definitions as markdown files under `.opencode/agent/` (and keep them small and single-purpose).
- If OpenCode needs to (re)generate this file, prefer the `/init` command and then refine the output to match the rules below.

## Prime directive: reuse Chapar patterns, use LSP to find golang code objects

The fastest path to “good Gio UI” in this repo is to reuse **proven patterns** from Chapar instead of inventing new ones.

When asked to build UI/components:

- First, search Chapar for an existing equivalent in local directory `./tmp/chapar` (widgets, theme, navigation, lists, editors, dialogs) and mirror the structure.
- Start by reading Chapar’s app entry/event loop and layout wiring (e.g., `main.go` in Chapar) to copy the overall flow and state ownership model.
- Prefer “copy the pattern, rename the domain” rather than “recreate the component from scratch”.

If there is no close match in Chapar:

- Build the smallest working version using Gio primitives, then iterate visually.
- Do not over-engineer abstractions on the first pass.

## Working style (required)

### Output expectations per task

Each change should include:

- A short plan (3–6 bullets) before editing.
- Minimal diffs (avoid drive-by refactors).
- Code that formats, builds, and runs.
- A quick “how to verify” section (commands + what to click/see).

### Keep iterations tight

- Implement in small, reviewable steps (one UI screen or one widget at a time).
- Prefer “make it work” → “make it pretty” → “make it reusable” (in that order).

### Ask clarifying questions when needed

If any of these are unclear, ask before coding:

- Target platform(s): Linux/Windows/macOS.
- Desired navigation model: tabs, sidebar, split panes.
- Data model + persistence approach.
- Whether visual parity with Chapar is required or only “similar quality”.

## Gio-specific rules (don’t fight the framework)

- Treat Gio as immediate-mode: UI is a pure function of the current state each frame; persistent widget state must be stored explicitly in structs. (Keep state out of layout closures unless it’s truly ephemeral.)
- Keep the UI thread responsive: any network I/O, parsing, file I/O, or heavy computation must not block the event loop (use goroutines + channels, then invalidate/refresh).
- Centralize application state in a small number of structs; pass pointers down to screens/components that own their widget state.

### Use Gio’s component toolkit intentionally

- `gioui.org/x/component` is useful, but it has no stable API; pin versions and avoid relying on internal/unexported behavior.
- Prefer simple composition (layout + material widgets) over complex custom draw ops unless necessary.

## Theme and styling

Goal: a cohesive theme system similar to Chapar (spacing, typography, colors, dark/light).

Rules:

- Theme decisions must be centralized (one theme package or module).
- Components accept a theme object (or style struct) rather than hardcoding colors.
- Keep spacing constants (padding, gap, corner radius) in one place.
- Do not introduce a new styling system unless required; mimic Chapar’s approach.

## Command cookbook (must follow)

Before finishing any task: 2. Build: do NOT build 3. Sanity checks (if present in repo):

- `go vet ./...`

## UI quality bar (what “close to Chapar” means)

- Consistent spacing and alignment across screens.
- Scrollable areas behave correctly (no clipped content, stable scroll state).
- Keyboard focus works for text inputs; tab/enter handling is deliberate.
- Long operations show progress/spinner and can be cancelled when feasible.
- Empty/error/loading states exist and look intentional (not raw errors).

## Persistence, privacy, and safety

Chapar’s product positioning emphasizes local-only operation and avoiding sending data to servers; keep the same default posture unless the user explicitly requests otherwise.

Rules:

- Never log secrets (tokens, API keys) to stdout.
- Prefer local storage; if adding telemetry/analytics, ask first.
- If implementing “import from Postman” or similar, keep it offline and deterministic (no background uploads).

## “Don’t do this” list

- Don't go outside of the workspace directory, use local `./tmp` for scratchpad and temporary tests
- Don't use heredoc to create/add files.
- Don’t invent a brand-new widget library when Chapar already has a pattern to copy.
- Don’t introduce heavy dependencies for state management, DI, or styling unless required.
- Don’t refactor unrelated parts of the codebase while implementing UI.
- Don’t ship unfinished UI (no placeholder labels/colors) unless explicitly requested for scaffolding.

## When stuck (explicit escalation path)

If implementation diverges from Chapar quality:

1. Locate the closest equivalent in Chapar and mirror the pattern more literally.
2. Reduce scope to a minimal working UI (one screen, one widget).
3. Ask a question: “Which aspect should match Chapar first: layout, theme, navigation, or interaction?”
