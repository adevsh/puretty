# puretty

A single Go binary that serves a browser-based terminal into the machine it runs on. One shell session shared across all connected browser tabs. Works on mobile via a special-key toolbar.

## Quick start

```bash
make build && ./puretty
# Open http://127.0.0.1:7000/
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-addr` | `127.0.0.1:7000` | Listen address |
| `-shell` | `/bin/sh` | Shell to spawn |

## Build

```bash
make build                # local binary
make build-linux-amd64    # Linux amd64 cross-compile
make build-linux-arm64    # Linux arm64 cross-compile
make test                 # run tests
make vet                  # go vet
make fmt                  # gofmt
```

## How it works

```
Browser tab ──POST /input──→ Session ──→ PTY ──→ /bin/sh
Browser tab ←─GET /output── Session ←── PTY read loop
```

- **`POST /input`** — raw bytes sent to the PTY master
- **`GET /output?offset=N`** — long-polls a 1 MiB ring buffer, returns new data since the last offset (25s timeout)
- **`POST /resize`** — JSON `{"rows":N,"cols":N}`, calls `pty.Setsize`

One `embed.FS` binary bundles xterm.js (v6), addon-fit, and the PWA shell. No build step, no npm.

## Mobile / PWA

- `manifest.json` + service worker for Android install-to-home-screen
- iOS meta tags for Safari standalone mode
- On-screen toolbar with Ctrl (sticky modifier), Esc, Tab, and arrow keys
- `visualViewport`-driven resize on keyboard show/hide

## Architecture

```
session.go   — OutputBuffer ring buffer, Session (PTY lifecycle)
handlers.go  — HTTP handlers (input, output long-poll, resize)
main.go      — flag parsing, signal handling, graceful shutdown, embed.FS
web/         — xterm.js vendored, app.js (XHR poll loop), PWA assets
```

## Security

No TLS, no auth, no rate limiting. The security model is the `-addr` flag: bind to `127.0.0.1` (or a WireGuard/Tailscale interface) and nothing else.
