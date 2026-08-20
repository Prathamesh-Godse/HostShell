# hostshell

A terminal-native portfolio — the same binary, served two ways:

```
ssh yourdomain.com
https://yourdomain.com
```

(Live links go here once Stage 1 deploys.)

## What this is

Not a website styled like a terminal — an actual Bubble Tea TUI, delivered
over both SSH (via Wish) and the browser (via ttyd + xterm.js). One Go
binary is the product; everything else is delivery plumbing.

## Build log

| Stage | Doc |
|---|---|
| 1. SSH↔web bridge (local scaffold) | [INC-001](docs/INC-001-ssh-web-bridge.md) |
| 2. SSH server + ttyd/nginx deployment | [INC-001](docs/INC-001-ssh-web-bridge.md) |

## Local dev

```bash
go run ./cmd/hostshell
```

## Running the SSH server

```bash
go run ./cmd/hostshell serve
# then, from another terminal:
ssh -p 23231 localhost
```

Host/port/host-key path are overridable via `HOSTSHELL_SSH_HOST`,
`HOSTSHELL_SSH_PORT`, `HOSTSHELL_SSH_HOST_KEY` — see
`internal/server/server.go`. Production systemd units and the nginx
reverse-proxy config for the web (ttyd) path are under `deploy/` and
`nginx/`.
