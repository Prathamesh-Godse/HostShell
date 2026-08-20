// Package tui contains the Bubble Tea model: the single source of truth
// for the app's UI and state. main.go just wires this up and runs it —
// it does not know anything about menus, cursors, or keypresses.
//
// Section text itself does not live here — see internal/content, which
// is plain-text files you can hand-edit without touching this package.
//
// Visual language, deliberately Unix/BSD-native rather than decorative:
//   - Headers are bracket tags, e.g. "[hostshell]" — the same convention
//     boot logs and systemd status output use for a labeled line.
//   - Selection is reverse video (foreground/background swapped), the
//     way `less`, `top`, and BSD's ncurses installers highlight a row —
//     not a colored cursor character.
//   - Color is reserved for status, and only ever one basic ANSI green:
//     active/ongoing state (● bullets, [ OK ] boot lines), nothing else.
//     A well-behaved terminal tool doesn't fight the user's color theme
//     any more than it has to.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/prathamesh-godse/hostshell/internal/content"
)

// view identifies which screen is on-screen. bootView plays once at
// startup, then hands off to menuView; every content screen is one
// level deep off the menu and returns to it on "back".
type view int

const (
	bootView view = iota
	menuView
	aboutView
	projectsView
	writeupsView
	codexView
	contactView
)

// writeupsTab identifies which of the three Writeups tabs is active.
type writeupsTab int

const (
	evergreenTab writeupsTab = iota
	signalTab
	incidentsTab
)

// Styling: reverse-video selection, bold for structure, green reserved
// for status only. No hue anywhere else — see the package doc above.
var (
	green      = lipgloss.Color("2") // basic ANSI green (index 2), not 256-color — works on any terminal's own palette
	greenStyle = lipgloss.NewStyle().Foreground(green)
	boldStyle  = lipgloss.NewStyle().Bold(true)
	hintStyle  = lipgloss.NewStyle().Faint(true)
	revStyle   = lipgloss.NewStyle().Reverse(true)
)

// frameOverhead is how many lines a content screen spends on chrome
// (header + blank + blank + hint) around the scrollable body. Kept as
// one constant so scroll-clamping math and rendering math can't drift.
const frameOverhead = 4

// Model is exported (capital M) so cmd/hostshell/main.go can construct it.
type Model struct {
	view    view
	choices []string
	cursor  int

	// boot sequence state — only meaningful while view == bootView.
	bootIndex int

	// writeups sub-state — only meaningful while view == writeupsView.
	wTab writeupsTab

	// list/detail sub-state — used by projectsView, writeupsView, and
	// codexView, which all browse as a titles list first, then a full
	// entry. about/contact are single bodies and never touch these.
	inDetail   bool
	itemCursor int
	listScroll int

	// scroll is the first visible line of whichever single scrollable
	// body is on screen right now: about/contact directly, or the
	// selected entry's detail view for the list-mode screens.
	scroll int

	// terminal size, kept current via tea.WindowSizeMsg so scrolling
	// math knows how many lines actually fit on screen.
	width  int
	height int
}

// New builds the starting state. Keeping construction here (rather than
// building a bare struct{} in main.go) means main.go never needs to know
// what fields Model has — only tui.New() does.
func New() Model {
	return Model{
		view: bootView,
		choices: []string{
			"About", "Projects", "Writeups", "ServerCodex", "Contact",
		},
	}
}

// --- boot sequence -----------------------------------------------------
//
// Two phases, mirroring how a real Linux boot actually reads: a fast
// flood of kernel-style lines (25ms apart — dense, no [ OK ] tags),
// slowing down for a shorter run of systemd-style service starts
// (150ms apart, green [ OK ]). Purely presentational — content lives
// here as Go slices rather than internal/content, since it's UI
// texture, not something the person is expected to hand-edit.
var bootKernelLines = []string{
	"hostshell 1.0 boot",
	"Command line: ./hostshell",
	"BIOS-provided physical RAM map",
	"x86/fpu: Supporting XSAVE feature",
	"tsc: Detected 3200.000 MHz processor",
	"tsc: Calibrating delay loop",
	"Calibrating delay loop... done",
	"smpboot: Allowing 8 CPUs, 0 hotplug CPUs",
	"smpboot: CPU0: bubbletea runtime core",
	"NUMA: Initialized distance table",
	"PID hash table entries: 4096",
	"Memory: embedded content 128K available",
	"SLUB: HWalign=64, Order=0-3, MinObjects=0",
	"clocksource: refined-jiffies: mask 0xffffffff",
	"rcu: Hierarchical RCU implementation",
	"NR_IRQS: 4352, nr_irqs: 512",
	"pci 0000:00:00.0: [hostshell] content controller",
	"pci 0000:00:01.0: [hostshell] tty multiplexer",
	"pci 0000:00:02.0: [hostshell] render pipeline",
	"pci 0000:00:03.0: [hostshell] ansi color engine",
	"pci 0000:00:04.0: [hostshell] keyboard input bridge",
	"pci 0000:00:05.0: [hostshell] scroll region unit",
	"pci 0000:00:06.0: [hostshell] alt-screen buffer",
	"pci 0000:00:07.0: [hostshell] lipgloss style bus",
	"ACPI: Core revision 20230628",
	"ACPI: bus type PNP registered",
	"PCI: Using configuration type 1 for base access",
	"vgaarb: loaded",
	"SCSI subsystem initialized",
	"usbcore: registered new interface driver usbfs",
	"usbcore: registered new interface driver hub",
	"usbcore: registered new device driver usb",
	"pty: 256 Unix98 ptys configured",
	"tty ptmx: partial deps",
	"loop: module loaded",
	"content: registered block device about",
	"content: registered block device projects",
	"content: registered block device writeups",
	"content: registered block device servercodex",
	"content: registered block device contact",
	"device-mapper: uevent: version 1.0.3",
	"cryptd: max_cpu_qlen set to 1000",
	"AVX2 version of gcm_enc/dec engaged",
	"Btrfs loaded",
	"ext4 filesystem support enabled",
	"devtmpfs: initialized",
	"clocksource: jiffies: mask 0xffffffff max_cycles",
	"futex hash table entries: 2048",
	"NET: Registered PF_UNIX/PF_LOCAL protocol family",
	"workingset: timestamp_bits=58 max_order=15",
	"random: crng init done",
	"Key type asymmetric registered",
	"async_tx: api initialized",
	"Block layer SCSI generic (bsg) driver version 0.4",
	"io scheduler mq-deadline registered",
	"input: hostshell keyboard as /devices/virtual/input/kbd0",
	"content: loaded data/about.txt",
	"content: loaded data/contact.txt",
	"content: loaded data/projects.txt",
	"content: loaded data/servercodex/01-base-os-install-and-initial-hardening.md",
	"content: loaded data/servercodex/02-nginx-install-and-directory-layout.md",
	"content: loaded data/servercodex/03-mariadb-install-and-secure-defaults.md",
	"content: loaded data/servercodex/04-php-fpm-pool-configuration-and-isolation.md",
	"content: loaded data/servercodex/05-ufw-and-fail2ban-baseline-rules.md",
	"content: loaded data/servercodex/06-ssl-tls-with-lets-encrypt.md",
	"content: loaded data/servercodex/07-ninjafirewall-waf-integration.md",
	"content: loaded data/servercodex/networking/01-example-networking-chapter.md",
	"content: loaded data/writeups/evergreen/01-what-fail2ban-actually-does.md",
	"content: loaded data/writeups/signal/01-why-most-homelab-guides-over-engineer-networking.md",
	"content: loaded data/writeups/incidents/2026-08-inc-001-ssh-web-bridge.md",
	"ACPI: Interpreter enabled",
	"ACPI: (supports S0 S3 S4 S5)",
	"cgroup: Disabling memory control group subsystem",
	"PM: Registering ACPI NVS region",
	"clk: Disabling unused clocks",
	"ansi256: colour ramp loaded",
	"lipgloss: style engine ready",
	"vt: bubbletea console driver registered",
	"tsc: Refined TSC clocksource calibration",
	"clocksource: Switched to clocksource tsc",
	"VFS: Disk quotas dquot_6.6.0",
	"FS-Cache: Loaded",
	"NET: Registered PF_INET protocol family",
	"TCP established hash table entries: 8192",
	"TCP bind hash table entries: 8192",
	"TCP: Hash tables configured",
	"UDP hash table entries: 512",
	"NET: Registered PF_INET6 protocol family",
	"sched_clock: Marking stable",
	"registered taskstats version 1",
	"Loading compiled-in X.509 certificates",
	"zbud: loaded",
	"Key type dns_resolver registered",
	"IPI shorthand broadcast: enabled",
	"sched_clock: Marking stable, no expected sleeps",
	"clocksource: acpi_pm unstable, using tsc",
	"alt-screen: entering full-screen buffer",
	"altscreen: buffer engaged",
	"input: keyboard driver ready",
	"nvme: probing device 0000:00:10.0",
	"nvme: driver loaded successfully",
	"ahci: probing device 0000:00:11.0",
	"ahci: driver loaded successfully",
	"e1000e: probing device 0000:00:12.0",
	"e1000e: driver loaded successfully",
	"xhci_hcd: probing device 0000:00:13.0",
	"xhci_hcd: driver loaded successfully",
	"snd_hda_intel: probing device 0000:00:14.0",
	"snd_hda_intel: driver loaded successfully",
	"i915: probing device 0000:00:15.0",
	"i915: driver loaded successfully",
	"thermal: probing device 0000:00:16.0",
	"thermal: driver loaded successfully",
	"cpufreq: probing device 0000:00:17.0",
	"cpufreq: driver loaded successfully",
	"watchdog: probing device 0000:00:18.0",
	"watchdog: driver loaded successfully",
	"rng-core: probing device 0000:00:19.0",
	"rng-core: driver loaded successfully",
}

var bootServiceLines = []string{
	"Mounted about",
	"Mounted contact",
	"Mounted projects",
	"Started writeups index",
	"Started writeups: evergreen",
	"Started writeups: signal",
	"Started writeups: incidents",
	"Started servercodex tree",
	"Started servercodex: networking",
	"Reached target Local File Systems",
	"Started Content Discovery",
	"Started Journal Service",
	"Started Load Kernel Modules",
	"Started Remount Root and Kernel File Systems",
	"Started Apply Kernel Variables",
	"Started Create Static Device Nodes in /dev",
	"Started udev Coldplug all Devices",
	"Reached target Sysinit",
	"Started Update UTMP about System Boot/Shutdown",
	"Started SSH bridge (wish)",
	"Started Web bridge (ttyd)",
	"Started Network Manager",
	"Reached target Network",
	"Started D-Bus System Message Bus",
	"Started ANSI Colour Ramp Loader",
	"Started Lipgloss Style Engine",
	"Started Alt-Screen Buffer Manager",
	"Started Bubble Tea Runtime",
	"Reached target Multi-User System",
	"Started Getty on tty1",
	"Reached target Graphical Interface",
	"Started hostshell.service",
	"Reached target interactive",
}

const (
	bootKernelDelay  = 12 * time.Millisecond
	bootServiceDelay = 30 * time.Millisecond
	bootFinalPause   = 300 * time.Millisecond
)

// bootTickMsg advances the boot sequence by one line.
type bootTickMsg struct{}

// bootTick schedules the next line reveal at the right speed for
// whichever phase the given index falls into.
func bootTick(nextIndex int) tea.Cmd {
	total := len(bootKernelLines) + len(bootServiceLines)
	var delay time.Duration
	switch {
	case nextIndex < len(bootKernelLines):
		delay = bootKernelDelay
	case nextIndex < total:
		delay = bootServiceDelay
	default:
		delay = bootFinalPause
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return bootTickMsg{} })
}

func (m Model) Init() tea.Cmd {
	return bootTick(0)
}

// resetContentState clears every piece of sub-navigation state so a
// screen never opens showing leftover scroll/cursor/detail from the
// last time it was visited.
func (m *Model) resetContentState() {
	m.inDetail = false
	m.itemCursor = 0
	m.listScroll = 0
	m.scroll = 0
}

// hasList reports whether the current view browses as a titles list
// with drill-down detail (Projects, Writeups, ServerCodex) versus a
// single body (About, Contact).
func (m Model) hasList() bool {
	return m.view == projectsView || m.view == writeupsView || m.view == codexView
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case bootTickMsg:
		total := len(bootKernelLines) + len(bootServiceLines)
		if m.bootIndex < total {
			m.bootIndex++
			return m, bootTick(m.bootIndex)
		}
		m.view = menuView
		return m, nil

	case tea.KeyMsg:
		if m.view == bootView {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			// Any other key skips straight to the menu — the boot
			// sequence is texture, not a gate the person has to sit
			// through every time they run this.
			m.view = menuView
			return m, nil
		}

		switch msg.String() {

		case "ctrl+c":
			return m, tea.Quit

		case "q":
			// q quits from the menu, but inside a content screen it's
			// swallowed by "back" below so it doesn't feel like a
			// trapdoor while reading.
			if m.view == menuView {
				return m, tea.Quit
			}
			m.view = menuView
			m.resetContentState()
			return m, nil

		case "esc", "b":
			if m.hasList() && m.inDetail {
				// Back out of a single entry to its titles list, not
				// all the way to the main menu.
				m.inDetail = false
				m.scroll = 0
			} else if m.view != menuView {
				m.view = menuView
				m.resetContentState()
			}
			return m, nil

		case "up", "k":
			m.moveUp()

		case "down", "j":
			m.moveDown()

		case "left", "h":
			if m.view == writeupsView && !m.inDetail && m.wTab > evergreenTab {
				m.wTab--
				m.itemCursor = 0
				m.listScroll = 0
			}

		case "right", "l", "tab":
			if m.view == writeupsView && !m.inDetail && m.wTab < incidentsTab {
				m.wTab++
				m.itemCursor = 0
				m.listScroll = 0
			}

		case "enter":
			if m.view == menuView {
				switch m.cursor {
				case 0:
					m.view = aboutView
				case 1:
					m.view = projectsView
				case 2:
					m.view = writeupsView
				case 3:
					m.view = codexView
				case 4:
					m.view = contactView
				}
				m.resetContentState()
			} else if m.hasList() && !m.inDetail {
				m.inDetail = true
				m.scroll = 0
			}
		}
	}

	return m, nil
}

// moveUp handles every "up/k" case: menu cursor, list cursor, or body
// scroll — whichever applies to the current view/mode.
func (m *Model) moveUp() {
	switch {
	case m.view == menuView:
		if m.cursor > 0 {
			m.cursor--
		}
	case m.hasList() && !m.inDetail:
		if m.itemCursor > 0 {
			m.itemCursor--
			if m.itemCursor < m.listScroll {
				m.listScroll = m.itemCursor
			}
		}
	default:
		if m.scroll > 0 {
			m.scroll--
		}
	}
}

// moveDown is moveUp's counterpart.
func (m *Model) moveDown() {
	switch {
	case m.view == menuView:
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}
	case m.hasList() && !m.inDetail:
		items := m.listRows()
		if m.itemCursor < len(items)-1 {
			m.itemCursor++
			visible := m.bodyHeight()
			if m.itemCursor >= m.listScroll+visible {
				m.listScroll = m.itemCursor - visible + 1
			}
		}
	default:
		lines := m.bodyLines()
		max := maxScroll(len(lines), m.bodyHeight())
		if m.scroll < max {
			m.scroll++
		}
	}
}

func (m Model) View() string {
	if m.view == bootView {
		return m.bootScreen()
	}
	if m.view == menuView {
		return m.menuScreen()
	}
	if m.hasList() && !m.inDetail {
		return m.listScreen()
	}
	return m.scrollableScreen()
}

// bootScreen renders the lines revealed so far, kernel-style lines
// dimmed with a fake timestamp, service-style lines with a green
// [ OK ] tag — matching the two-phase pacing driven by bootTick.
func (m Model) bootScreen() string {
	var b strings.Builder

	for i := 0; i < m.bootIndex && i < len(bootKernelLines); i++ {
		b.WriteString(hintStyle.Render(fmt.Sprintf("[%9.6f]", float64(i)*0.001)))
		b.WriteString(" " + bootKernelLines[i] + "\n")
	}
	for i := len(bootKernelLines); i < m.bootIndex && i < len(bootKernelLines)+len(bootServiceLines); i++ {
		b.WriteString(greenStyle.Render("[ OK ]"))
		b.WriteString(" " + bootServiceLines[i-len(bootKernelLines)] + "\n")
	}
	b.WriteString("\n")

	footer := hintStyle.Render("press any key to skip")
	return m.anchorFooter(b.String(), footer) + "\n"
}

func (m Model) menuScreen() string {
	var b strings.Builder

	b.WriteString(greenStyle.Render("[hostshell]"))
	b.WriteString("\n\n")
	b.WriteString("What do you want to see?\n\n")

	for i, choice := range m.choices {
		if m.cursor == i {
			b.WriteString(revStyle.Render(choice))
		} else {
			b.WriteString(choice)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	footer := hintStyle.Render("↑/↓ or j/k to move · enter to select · q to quit")
	return m.anchorFooter(b.String(), footer) + "\n"
}

// title returns the header text for whichever content view is active.
func (m Model) title() string {
	switch m.view {
	case aboutView:
		return "About"
	case projectsView:
		return "Projects"
	case writeupsView:
		return "Writeups"
	case codexView:
		return "ServerCodex"
	case contactView:
		return "Contact"
	}
	return ""
}

// listRow is one row in a list-mode screen: a label, an optional status
// string, and whether that status counts as "active" (gets the green
// bullet/text — everything else gets the dim hollow bullet).
type listRow struct {
	label  string
	status string
	active bool
}

// listRows returns the rows for whichever list-mode view is active.
func (m Model) listRows() []listRow {
	switch m.view {
	case projectsView:
		projects := content.Projects()
		out := make([]listRow, len(projects))
		for i, p := range projects {
			out[i] = listRow{label: p.Name, status: p.Status, active: isActiveStatus(p.Status)}
		}
		return out
	case writeupsView:
		items := m.currentWriteups()
		out := make([]listRow, len(items))
		for i, w := range items {
			out[i] = listRow{label: w.Title, status: w.Date}
		}
		return out
	case codexView:
		chapters := content.Codex()
		out := make([]listRow, len(chapters))
		for i, c := range chapters {
			out[i] = listRow{label: fmt.Sprintf("%2d.  %s", c.Num, c.Title)}
		}
		return out
	}
	return nil
}

// isActiveStatus decides whether a project's status text earns the
// green ● bullet — anything reading as "still being worked on" versus
// "done"/"pending", the same active/inactive split systemctl draws.
func isActiveStatus(status string) bool {
	s := strings.ToLower(status)
	return strings.Contains(s, "ongoing") || strings.Contains(s, "in progress")
}

func (m Model) currentWriteups() []content.Writeup {
	switch m.wTab {
	case evergreenTab:
		return content.Evergreen()
	case signalTab:
		return content.Signal()
	default:
		return content.Incidents()
	}
}

// selectedDetail renders the full text of whichever item is highlighted
// in the current list-mode view.
func (m Model) selectedDetail() string {
	switch m.view {
	case projectsView:
		projects := content.Projects()
		if m.itemCursor >= len(projects) {
			return ""
		}
		p := projects[m.itemCursor]
		return renderProjectStatus(p)
	case writeupsView:
		items := m.currentWriteups()
		if m.itemCursor >= len(items) {
			return ""
		}
		w := items[m.itemCursor]
		head := w.Title
		if w.Date != "" {
			head += "  " + hintStyle.Render(w.Date)
		}
		return fmt.Sprintf("%s\n\n%s", head, wrap(w.Desc, 70))
	case codexView:
		chapters := content.Codex()
		if m.itemCursor >= len(chapters) {
			return ""
		}
		c := chapters[m.itemCursor]
		return fmt.Sprintf("%s\n\n%s", c.Title, wrap(c.Body, 70))
	}
	return ""
}

// statusLine renders a bullet + status text, green/filled when active,
// dim/hollow otherwise — the systemctl active/inactive convention.
func statusLine(status string, active bool) string {
	if active {
		return greenStyle.Render("● " + status)
	}
	return hintStyle.Render("○ " + status)
}

// renderProjectStatus renders a project's detail view in the shape of
// `systemctl status <unit>` — bullet + name + tagline on the header
// line, right-aligned label/value fields below it, then the full
// description in place of systemd's trailing log lines (a project
// doesn't have log events; its description is the more honest analog).
func renderProjectStatus(p content.Project) string {
	active := isActiveStatus(p.Status)
	bullet := hintStyle.Render("○")
	if active {
		bullet = greenStyle.Render("●")
	}

	stateWord := "inactive"
	if active {
		stateWord = "active"
	}

	var b strings.Builder
	b.WriteString(bullet + " " + boldStyle.Render(p.Name))
	if p.Tagline != "" {
		b.WriteString(" - " + p.Tagline)
	}
	b.WriteString("\n")

	field := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(fmt.Sprintf("%9s: %s\n", label, value))
	}
	loaded := p.RepoURL
	if loaded == "" {
		loaded = "no public repo yet"
	}
	field("Loaded", loaded)
	field("Active", fmt.Sprintf("%s (%s)", stateWord, p.Status))
	field("Stack", p.Stack)
	field("License", p.License)

	b.WriteString("\n")
	b.WriteString(wrap(p.Desc, 70))

	return b.String()
}

// bodyLines returns the current view's full content, already split into
// lines, before any scroll-window slicing. Single-body screens (About,
// Contact) render whole; list-mode screens render the selected entry's
// detail once drilled in.
func (m Model) bodyLines() []string {
	var body string

	switch m.view {
	case aboutView:
		body = m.renderAboutManPage()
	case projectsView, writeupsView, codexView:
		body = m.selectedDetail()
	case contactView:
		body = content.Contact()
	}

	return strings.Split(body, "\n")
}

// bodyHeight is how many lines actually fit given the current terminal
// size. Falls back to a generous default before the first
// tea.WindowSizeMsg arrives (practically instant, but avoids a flash of
// an over-clamped view on the very first frame).
func (m Model) bodyHeight() int {
	if m.height <= 0 {
		return 1000
	}
	h := m.height - frameOverhead
	if h < 1 {
		h = 1
	}
	return h
}

func maxScroll(totalLines, visibleHeight int) int {
	max := totalLines - visibleHeight
	if max < 0 {
		max = 0
	}
	return max
}

// listScreen dispatches to whichever list-mode rendering the current
// view uses. Projects keeps the systemctl-style bullet list; Writeups
// and ServerCodex get dedicated, very different visual treatments (see
// writeupsVimScreen and codexTreeScreen) — different enough from each
// other and from Projects that folding them into one generic renderer
// stopped making sense.
func (m Model) listScreen() string {
	switch m.view {
	case writeupsView:
		return m.writeupsVimScreen()
	case codexView:
		return m.codexTreeScreen()
	}

	rows := m.listRows()
	visibleHeight := m.bodyHeight()
	end := m.listScroll + visibleHeight
	if end > len(rows) {
		end = len(rows)
	}

	var b strings.Builder
	b.WriteString(greenStyle.Render("[" + m.title() + "]"))
	b.WriteString("\n\n")

	if len(rows) == 0 {
		b.WriteString(hintStyle.Render("Nothing here yet."))
		b.WriteString("\n")
	} else {
		for i := m.listScroll; i < end; i++ {
			r := rows[i]
			if i == m.itemCursor {
				line := r.label
				if r.status != "" {
					line += "  " + r.status
				}
				b.WriteString(revStyle.Render(line))
			} else {
				b.WriteString(r.label)
				if r.status != "" {
					b.WriteString("  ")
					b.WriteString(statusLine(r.status, r.active))
				}
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	hint := "↑/↓ or j/k to move · enter to open · esc/b to go back · q to quit"

	return m.anchorFooter(b.String(), hintStyle.Render(hint)) + "\n"
}

// scrollableScreen renders the header, the scroll-clamped visible slice
// of the current body (About/Contact, or a drilled-into Projects/
// Writeups/ServerCodex entry), a scroll-position indicator when content
// overflows, and the footer hint.
func (m Model) scrollableScreen() string {
	lines := m.bodyLines()
	visibleHeight := m.bodyHeight()
	max := maxScroll(len(lines), visibleHeight)
	scroll := m.scroll
	if scroll > max {
		scroll = max
	}

	end := scroll + visibleHeight
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[scroll:end]

	var b strings.Builder
	if m.view != aboutView {
		// About's body already opens with its own man-page style
		// header line (see renderAboutManPage) — the generic bracket
		// tag would just duplicate it.
		b.WriteString(greenStyle.Render("[" + m.title() + "]"))
		b.WriteString("\n\n")
	}
	b.WriteString(strings.Join(visible, "\n"))
	b.WriteString("\n\n")

	hint := "esc/b to go back · q to quit"
	if m.hasList() && m.inDetail {
		hint = "esc/b to go back to the list · q to quit"
	}
	if max > 0 {
		hint = fmt.Sprintf("↑/↓ or j/k to scroll (%d/%d) · %s", scroll+1, max+1, hint)
	}

	return m.anchorFooter(b.String(), hintStyle.Render(hint)) + "\n"
}

// --- Writeups: rendered like Neovim's visual mode ----------------------
//
// Line-number gutter, a full-width reverse-video bar for the selected
// row (padded to the terminal edge, not just the text — visual-line
// selection in vim covers the whole width), and a two-part statusline
// at the bottom (mode + filename on the left, position on the right)
// styled after vim's default statusline/airline. Kept monochrome
// (bold for the mode label, dim for the rest) rather than copying vim's
// colored statusline verbatim, to stay consistent with the rest of the
// app's "reverse-video + bold + one green, nothing else" palette.

// writeupsChrome is the number of non-row lines writeupsVimScreen always
// prints, used to compute how many rows actually fit on screen.
const writeupsChrome = 5 // header + tab bar + blank + statusline + hint

func (m Model) writeupsVimScreen() string {
	items := m.currentWriteups()
	width := m.manPageWidth()
	visibleHeight := m.visibleRows(writeupsChrome)
	end := m.listScroll + visibleHeight
	if end > len(items) {
		end = len(items)
	}

	gutterWidth := len(fmt.Sprintf("%d", len(items)))
	if gutterWidth < 2 {
		gutterWidth = 2
	}

	var body strings.Builder
	body.WriteString(greenStyle.Render("[Writeups]"))
	body.WriteString("\n")
	body.WriteString(renderTabBar(m.wTab))
	body.WriteString("\n")

	if len(items) == 0 {
		body.WriteString(hintStyle.Render("Nothing here yet."))
		body.WriteString("\n")
	} else {
		for i := m.listScroll; i < end; i++ {
			w := items[i]
			label := w.Title
			if w.Date != "" {
				label += "  " + w.Date
			}
			lineNum := fmt.Sprintf("%*d ", gutterWidth, i+1)
			if i == m.itemCursor {
				body.WriteString(revStyle.Render(padToWidth(lineNum+label, width)))
			} else {
				body.WriteString(hintStyle.Render(lineNum) + label)
			}
			body.WriteString("\n")
		}
	}

	footer := "\n" + m.vimStatusLine(width, m.listScroll, visibleHeight, len(items)) + "\n" +
		hintStyle.Render("←/→ or h/l to switch tab · ↑/↓ or j/k to move · enter to open · esc/b to go back · q to quit")

	return m.anchorFooter(body.String(), footer) + "\n"
}

// writeupsFilename returns the real on-disk directory for the active
// tab — these genuinely are the directories under internal/content/
// data/writeups/, so the statusline isn't just decoration, it's
// accurate to what's actually being read.
func (m Model) writeupsFilename() string {
	switch m.wTab {
	case evergreenTab:
		return "writeups/evergreen/"
	case signalTab:
		return "writeups/signal/"
	default:
		return "writeups/incidents/"
	}
}

// vimStatusLine builds a two-part bar: "NORMAL  filename" on the left,
// "row:col  Top/Bot/NN%" on the right, spaced to fill width — the same
// shape as vim's default statusline. "NORMAL" is the one mode this app
// has (no insert/visual mode to switch into), shown for the reference
// rather than because it's functionally meaningful here.
func (m Model) vimStatusLine(width, scroll, visibleHeight, total int) string {
	left := boldStyle.Render("NORMAL") + "  " + hintStyle.Render(m.writeupsFilename())

	// Position word mirrors vim's ruler: based on scroll position
	// relative to the file, not the cursor line specifically. "All"
	// when everything fits on screen at once, "Top"/"Bot" at the
	// respective edges, a percentage in between.
	posWord := "All"
	switch {
	case total == 0:
		posWord = "Top"
	case total <= visibleHeight:
		posWord = "All"
	case scroll <= 0:
		posWord = "Top"
	case scroll+visibleHeight >= total:
		posWord = "Bot"
	default:
		posWord = fmt.Sprintf("%d%%%%", scroll*100/(total-visibleHeight))
	}

	// row:col — row is the highlighted item's position (changes as you
	// move up/down), col repurposes vim's column concept as "which
	// Writeups tab" (1=Evergreen, 2=Signal, 3=Incidents), since a list
	// of titles doesn't have real text columns to report.
	right := hintStyle.Render(fmt.Sprintf("%d:%d  %s", m.itemCursor+1, int(m.wTab)+1, posWord))

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// --- ServerCodex: rendered like `tree` output ---------------------------

// codexChrome is the number of non-row lines codexTreeScreen always
// prints, used to compute how many rows actually fit on screen.
const codexChrome = 6 // header + blank + root line + blank + count line + hint

func (m Model) codexTreeScreen() string {
	chapters := content.Codex()
	width := m.manPageWidth()
	visibleHeight := m.visibleRows(codexChrome)

	rows := buildCodexTreeRows(chapters)

	// Find which visual row the cursor (a chapter index) lands on, then
	// center the scroll window on it. Folders shift chapters to
	// different visual rows than their plain index, so the shared
	// moveUp/moveDown's edge-triggered listScroll (tracked in
	// chapter-index terms, shared with Projects/Writeups) can't be
	// reused directly here — centering on the cursor each render is
	// simpler and just as usable.
	cursorRow := 0
	for i, r := range rows {
		if r.chapterIdx == m.itemCursor {
			cursorRow = i
			break
		}
	}
	scroll := cursorRow - visibleHeight/2
	maxScrollRows := len(rows) - visibleHeight
	if maxScrollRows < 0 {
		maxScrollRows = 0
	}
	if scroll > maxScrollRows {
		scroll = maxScrollRows
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + visibleHeight
	if end > len(rows) {
		end = len(rows)
	}

	var b strings.Builder
	b.WriteString(greenStyle.Render("[ServerCodex]"))
	b.WriteString("\n\n")
	b.WriteString("servercodex/\n")

	if len(chapters) == 0 {
		b.WriteString(hintStyle.Render("Nothing here yet."))
		b.WriteString("\n")
	} else {
		for i := scroll; i < end; i++ {
			r := rows[i]
			if r.chapterIdx == m.itemCursor {
				b.WriteString(revStyle.Render(padToWidth(r.text, width)))
			} else {
				b.WriteString(r.text)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	footer := hintStyle.Render(fmt.Sprintf("%d chapters", len(chapters))) + "\n" +
		hintStyle.Render("↑/↓ or j/k to move · enter to open · esc/b to go back · q to quit")

	return m.anchorFooter(b.String(), footer) + "\n"
}

// codexTreeRow is one visual line of the ServerCodex tree: either a
// selectable chapter (chapterIdx is its index into content.Codex()) or
// a non-selectable directory header line (chapterIdx == -1).
type codexTreeRow struct {
	text       string
	chapterIdx int
}

// codexDirGroup collects every chapter that shares a directory prefix,
// in the order they appear in codex.txt.
type codexDirGroup struct {
	name    string
	indices []int
}

// buildCodexTreeRows flattens chapters into the exact lines `tree`
// would print: root-level chapters and directories interleaved in
// first-appearance order, each directory's chapters nested one level
// under it with the matching "│   "/"    " continuation prefix.
func buildCodexTreeRows(chapters []content.Chapter) []codexTreeRow {
	var order []any // int (root chapter index) or *codexDirGroup
	dirByName := map[string]*codexDirGroup{}

	for i, c := range chapters {
		if c.Dir == "" {
			order = append(order, i)
			continue
		}
		g, ok := dirByName[c.Dir]
		if !ok {
			g = &codexDirGroup{name: c.Dir}
			dirByName[c.Dir] = g
			order = append(order, g)
		}
		g.indices = append(g.indices, i)
	}

	var rows []codexTreeRow
	for oi, item := range order {
		isLastTop := oi == len(order)-1
		branch := "├── "
		if isLastTop {
			branch = "└── "
		}

		switch v := item.(type) {
		case int:
			filename := fmt.Sprintf("%02d-%s.md", chapters[v].Num, slugify(chapters[v].Title))
			rows = append(rows, codexTreeRow{text: branch + filename, chapterIdx: v})

		case *codexDirGroup:
			rows = append(rows, codexTreeRow{text: branch + v.name + "/", chapterIdx: -1})
			childPrefix := "│   "
			if isLastTop {
				childPrefix = "    "
			}
			for ci, idx := range v.indices {
				childBranch := "├── "
				if ci == len(v.indices)-1 {
					childBranch = "└── "
				}
				filename := fmt.Sprintf("%02d-%s.md", chapters[idx].Num, slugify(chapters[idx].Title))
				rows = append(rows, codexTreeRow{text: childPrefix + childBranch + filename, chapterIdx: idx})
			}
		}
	}
	return rows
}

// slugify turns a chapter title into a lowercase-hyphenated filename
// stem, e.g. "Base OS install and initial hardening" ->
// "base-os-install-and-initial-hardening" — matching how a real
// filesystem tree of markdown chapters would actually be named.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// padToWidth right-pads s with spaces to width, measuring visible width
// (not byte length) so multi-byte characters like em dashes don't throw
// off the padding — used to make the reverse-video selection bar span
// the full terminal width, matching vim's visual-line-mode highlight.
func padToWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// anchorFooter appends footer to body with exactly enough blank lines
// between them that footer's last line lands on the terminal's actual
// bottom row — every screen's hint/status bar should sit at the same
// physical spot on screen regardless of how much content is above it.
// Measures real line counts rather than a hand-counted constant, since
// hand-counted "chrome" estimates have drifted wrong before. body must
// end with "\n" after its last content line (as every builder here
// already does); footer must NOT have a trailing newline — the caller
// adds exactly one after this returns. Returns body+footer unpadded
// before real terminal size is known, rather than guessing.
func (m Model) anchorFooter(body, footer string) string {
	if m.height <= 0 {
		return body + footer
	}
	linesSoFar := strings.Count(body, "\n")
	footerLines := strings.Count(footer, "\n") + 1
	pad := m.height - linesSoFar - footerLines
	if pad < 0 {
		pad = 0
	}
	return body + strings.Repeat("\n", pad) + footer
}

// visibleRows is bodyHeight's per-screen-chrome counterpart: how many
// rows fit given the current terminal height minus `chrome` lines of
// fixed UI around them, rather than the generic frameOverhead used by
// bodyHeight for the simpler content screens.
func (m Model) visibleRows(chrome int) int {
	if m.height <= 0 {
		return 1000
	}
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	return h
}

func renderTabBar(active writeupsTab) string {
	labels := []string{"Evergreen", "Signal", "Incidents"}
	parts := make([]string, len(labels))

	for i, label := range labels {
		if writeupsTab(i) == active {
			parts[i] = boldStyle.Render(label)
		} else {
			parts[i] = hintStyle.Render(label)
		}
	}

	return strings.Join(parts, "   ")
}

// wrap does simple word-wrapping at width columns. Bubble Tea apps don't
// get free line-wrapping the way a browser does, so long descriptions
// need this or they'll run off narrow terminals.
func wrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}

	var b strings.Builder
	lineLen := 0

	for i, w := range words {
		if lineLen > 0 && lineLen+1+len(w) > width {
			b.WriteString("\n")
			lineLen = 0
		} else if i > 0 {
			b.WriteString(" ")
			lineLen++
		}
		b.WriteString(w)
		lineLen += len(w)
	}

	return b.String()
}

// manPageWidth returns the target line width for the man-page header
// and body: the actual terminal width, with a small margin so text
// doesn't touch the very edge. Real man pages fill whatever width the
// terminal actually is — no artificial cap — so this does too. Floored
// so narrow terminals still get something sane, and falls back to a
// fixed default before the first tea.WindowSizeMsg arrives.
func (m Model) manPageWidth() int {
	if m.width <= 0 {
		return 70
	}
	w := m.width - 2
	if w < 40 {
		w = 40
	}
	return w
}

// indentWrap wraps s to (width - indent) columns, then prefixes every
// resulting line with `indent` spaces — man pages indent body text
// under each section header rather than starting at column 0.
func indentWrap(s string, indent, width int) string {
	wrapped := wrap(s, width-indent)
	prefix := strings.Repeat(" ", indent)
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

// manPageHeader builds the classic "NAME(1)  ...  User Commands  ...
// NAME(1)" line real man pages open with — title left and right,
// "User Commands" centered between them, padded to width.
func manPageHeader(width int) string {
	left := "HOSTSHELL(1)"
	right := "HOSTSHELL(1)"
	mid := "User Commands"

	pad := width - len(left) - len(right) - len(mid)
	if pad < 2 {
		pad = 2
	}
	leftPad := pad / 2
	rightPad := pad - leftPad

	return boldStyle.Render(left) +
		strings.Repeat(" ", leftPad) + mid + strings.Repeat(" ", rightPad) +
		boldStyle.Render(right)
}

// renderAboutManPage renders content.AboutSections() as a man page:
// bold ALL-CAPS section headers, indented body text. SKILLS is treated
// specially — each "Category: items" line becomes its own bold
// sub-heading with the items indented further below it, the same shape
// real man pages use for an option list (flag, then its description
// indented on the next line).
func (m Model) renderAboutManPage() string {
	width := m.manPageWidth()

	var b strings.Builder
	b.WriteString(manPageHeader(width))
	b.WriteString("\n\n")

	sections := content.AboutSections()
	for i, s := range sections {
		b.WriteString(boldStyle.Render(s.Header))
		b.WriteString("\n")

		if s.Header == "SKILLS" {
			for _, line := range s.Body {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					b.WriteString(indentWrap(line, 5, width))
					b.WriteString("\n")
					continue
				}
				b.WriteString("     " + boldStyle.Render(strings.TrimSpace(parts[0])))
				b.WriteString("\n")
				b.WriteString(indentWrap(strings.TrimSpace(parts[1]), 9, width))
				b.WriteString("\n")
			}
		} else {
			b.WriteString(indentWrap(strings.Join(s.Body, " "), 5, width))
			b.WriteString("\n")
		}

		if i < len(sections)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
