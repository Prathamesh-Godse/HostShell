// hostshell — entrypoint. Deliberately thin: it just decides which mode
// to run in, then hands off to internal/tui or internal/server. Every
// delivery path (local dev, ttyd/web, SSH) ends up constructing the
// exact same tui.New() model, so behavior never diverges between them.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/prathamesh-godse/hostshell/internal/server"
	"github.com/prathamesh-godse/hostshell/internal/tui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := server.Serve(); err != nil {
			fmt.Println("Error running SSH server:", err)
			os.Exit(1)
		}
		return
	}

	// Plain local run: used for `go run ./cmd/hostshell` during
	// development, and this exact invocation is also what ttyd wraps to
	// deliver hostshell in the browser — ttyd just execs a binary with
	// a pty attached, identical to running it in a terminal by hand, so
	// no ttyd-specific code is needed here at all.
	//
	// Full-screen alternate buffer: without this, Bubble Tea's inline
	// renderer can drop lines (including blank ones) once content grows
	// taller than the terminal window.
	p := tea.NewProgram(tui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
