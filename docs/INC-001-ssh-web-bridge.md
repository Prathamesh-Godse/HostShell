# INC-001: SSH↔Web Bridge Architecture

## 1. Trigger

hostshell's whole premise is one Go binary delivered two ways —
`ssh yourdomain.com` and `https://yourdomain.com` showing the same TUI.
Stage 1 only had the local Bubble Tea program; there was no actual
network delivery path yet. This stage builds both.

## 2. Before State

`cmd/hostshell/main.go` ran `tea.NewProgram(tui.New())` directly against
whatever terminal invoked it — fine for `go run` during development, but
nothing was listening on a network port. No SSH server, no web bridge.

## 3. Change Made

Two delivery paths, both wrapping the same `internal/tui.New()` model
so behavior can't diverge between them:

- **SSH**: `internal/server` runs a persistent [Wish](https://github.com/charmbracelet/wish)
  server. Each connecting session gets `bm.Middleware(teaHandler)`,
  which constructs a fresh `tui.New()` per session — sessions never
  share state. `activeterm.Middleware()` rejects non-interactive
  sessions (e.g. `ssh host ls`) instead of hanging them, since the TUI
  only makes sense with an attached pty. `main.go` dispatches into this
  mode when invoked as `hostshell serve`.

- **Web**: no code change at all — this was the interesting realization.
  [ttyd](https://github.com/tsl0922/ttyd) execs a binary with a pty
  attached and streams it to a browser via xterm.js, which is *exactly*
  what happens when a person runs `hostshell` in a real terminal. The
  plain local-run path in `main.go` already is the web-delivery path;
  ttyd just needs to be told to wrap the compiled binary.

Deployment plumbing added under `deploy/systemd/` (one unit for the SSH
server, one for ttyd) and `nginx/hostshell.conf` (reverse-proxies ttyd's
websocket to the public domain over TLS, since ttyd itself only binds
to localhost).

## 4. After State

- `hostshell serve` starts the SSH server (host/port/host-key path all
  overridable via env vars, no rebuild needed to relocate any of them).
  Verified working end-to-end: `ssh -p 23231 localhost` connects and
  renders the same menu as a local run.
- `hostshell` (no args) behaves exactly as before — this is what ttyd
  wraps for the browser path. Verified working via `ttyd -p 7681 -W
  ./hostshell` — `http://localhost:7681` renders and navigates
  identically to the SSH session.
- Reverse proxy config and systemd units are written and reviewed, not
  yet deployed to a live box — that's the next real-world test.

## 5. Lessons / What Broke

- The instinct going in was "web delivery needs its own code path
  parallel to SSH." It doesn't — ttyd's entire job is making a normal
  terminal program look like a network service, so the *existing*
  `go run`-style entrypoint was already the correct web target. The
  actual gap was purely on the SSH side, which genuinely does need a
  persistent listener since SSH sessions don't come with ttyd's "spawn
  a fresh process per pty" model built in.
- `activeterm.Middleware()` matters more than it looks — without it, a
  non-interactive SSH invocation (script, health check, `ssh host true`)
  would attach to a Bubble Tea program with no terminal to render into
  and just hang instead of failing cleanly.
- Misread ttyd's `--once` flag on the first pass: it doesn't mean "fresh
  process per client," it means "serve exactly one connection ever,
  then shut the whole server down" — meant for one-shot terminal
  sharing, not a public site. Quitting hostshell (`q`) inside that one
  allowed connection killed ttyd itself, and the browser's "Press ⏎ to
  Reconnect" had nothing left to reach. Fixed by dropping `--once`
  entirely — ttyd's actual default already spawns a fresh process per
  browser connection while staying up indefinitely, which was the
  behavior wanted the whole time.
- Testing from a fresh zip extraction each time (rather than one
  persistent local folder) meant the SSH host key regenerated on every
  run, since its path was relative to the working directory — every
  reconnect tripped `REMOTE HOST IDENTIFICATION HAS CHANGED`. Fixed by
  pointing `HOSTSHELL_SSH_HOST_KEY` at a stable path outside any
  per-extraction folder (`~/.hostshell/host_key`), so the server's
  identity now persists across reruns and rebuilds.
