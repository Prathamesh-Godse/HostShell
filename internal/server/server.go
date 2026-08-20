// Package server runs hostshell as a persistent SSH server via Wish,
// serving the same internal/tui model to every connecting session that
// cmd/hostshell/main.go runs locally for `go run` and for ttyd. SSH and
// web (via ttyd) end up showing an identical app because both paths
// construct the exact same tui.New() — this package's only job is
// wiring that model up to a network listener instead of a local pty.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	"github.com/prathamesh-godse/hostshell/internal/tui"
)

// Defaults are all overridable via environment variables so deployment
// (see deploy/systemd/hostshell-ssh.service) never needs a code change
// or rebuild to move host/port/key location.
const (
	defaultHost    = "0.0.0.0"
	defaultPort    = "23231" // unprivileged; production maps 22 -> this via port-forward or a reverse proxy that speaks SSH
	defaultHostKey = "hostshell_host_key"
)

// Serve starts the SSH server and blocks until interrupted (SIGINT/
// SIGTERM), then shuts down cleanly. Called from cmd/hostshell/main.go
// when invoked as `hostshell serve`.
func Serve() error {
	host := envOr("HOSTSHELL_SSH_HOST", defaultHost)
	port := envOr("HOSTSHELL_SSH_PORT", defaultPort)
	keyPath := envOr("HOSTSHELL_SSH_HOST_KEY", defaultHostKey)

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		// Wish generates and persists a host key here on first run if
		// one doesn't already exist — back this path up in production
		// so the server's identity (and client-side known_hosts trust)
		// survives redeploys.
		wish.WithHostKeyPath(keyPath),
		wish.WithMiddleware(
			bm.Middleware(teaHandler),
			// hostshell only makes sense interactively — reject
			// sessions with no attached pty (e.g. `ssh host ls`-style
			// non-interactive commands) instead of hanging them.
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
	if err != nil {
		return fmt.Errorf("creating ssh server: %w", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	log.Printf("hostshell SSH server listening on %s", net.JoinHostPort(host, port))
	errc := make(chan error, 1)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-done:
	}

	log.Println("stopping ssh server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.Shutdown(ctx)
}

// teaHandler builds a fresh Bubble Tea program per SSH session. Each
// connecting client gets their own independent tui.New() — sessions
// never share state with each other.
func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	_, _, active := s.Pty()
	if !active {
		// activeterm.Middleware() already filters non-pty sessions out
		// before this runs; this check is just defense in depth.
		return nil, nil
	}
	return tui.New(), []tea.ProgramOption{tea.WithAltScreen()}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
