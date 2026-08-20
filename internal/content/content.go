// Package content is the ONLY place section text lives. Everything under
// data/ is a plain text file you can hand-edit directly — no Go syntax,
// no need to touch internal/tui at all.
//
// Every file under data/ — about.txt, contact.txt, projects.txt, and
// the servercodex/ and writeups/ directory trees — is read through the
// same rule: by default from the copy embedded in the binary at build
// time (so `go run` works with zero setup), but if HOSTSHELL_CONTENT_DIR
// is set, from that real directory on disk instead. That's what makes
// the "content" branch auto-deploy possible — a GitHub Actions workflow
// (see .github/workflows/deploy-content.yml) syncs that whole directory
// onto a live server, and every one of these functions picks the change
// up on its next call — no rebuild, no restart, since none of them
// cache anything between calls.
//
// ServerCodex and Writeups are additionally real directory trees rather
// than single delimited files — one file per entry, discovered by
// walking whatever exists. Add, remove, or rename a file and it just
// shows up, no other file to edit and no code change either.
//
// File formats (see data/ for the real files):
//
//	about.txt                            — man-page style: repeated blocks
//	                                        separated by a line containing
//	                                        only "===", each block's first
//	                                        line an ALL-CAPS section header
//	                                        (NAME, SYNOPSIS, DESCRIPTION,
//	                                        EXPERIENCE, SKILLS, SEE ALSO,
//	                                        AUTHOR — any headers work,
//	                                        these just match convention),
//	                                        remaining lines the body. In
//	                                        SKILLS, each body line is
//	                                        treated as "Category: items"
//	                                        and rendered as its own
//	                                        sub-list; everywhere else,
//	                                        body lines join into one
//	                                        wrapped paragraph.
//	contact.txt                          — one block of plain text, used as-is.
//	projects.txt                         — repeated blocks separated by a
//	                                        line containing only "===":
//	                                          <name>
//	                                          <status>
//	                                          <repo URL, or a blank line
//	                                           if the project has none>
//	                                          <tagline — one line>
//	                                          <tech stack, comma-separated>
//	                                          <license, or "not yet
//	                                           chosen" if undecided>
//	                                          <desc, can wrap multiple lines>
//
//	servercodex/**  and  writeups/{evergreen,signal,incidents}/**
//	                                      — one file per entry. Each file:
//	                                          Title: <the title>
//	                                          Date: <YYYY-MM, writeups/incidents only>
//	                                          (blank line)
//	                                          <body, can wrap multiple lines>
//	                                        Date is optional and only
//	                                        meaningful for writeups.
//	                                        Ordering within a directory
//	                                        comes from filename sort, so
//	                                        prefix files with a number:
//	                                        "01-...", "02-...". Put a
//	                                        servercodex file inside a
//	                                        subdirectory (e.g.
//	                                        servercodex/networking/) to
//	                                        group it under that folder in
//	                                        the tree view.
package content

import (
	"embed"
	"io/fs"
	"os"
	"sort"
	"strings"
)

//go:embed data
var files embed.FS

// Project is one entry on the Projects screen.
type Project struct {
	Name    string
	Status  string
	RepoURL string // empty if the project has no public repo yet
	Tagline string // short one-line description, shown next to the name
	Stack   string // comma-separated tech stack
	License string
	Desc    string
}

// Writeup is one entry in a Writeups tab (Evergreen/Signal/Incidents).
type Writeup struct {
	Title string
	Date  string // "YYYY-MM", or "" if undated
	Desc  string
}

// Chapter is one entry in ServerCodex's chapter tree.
type Chapter struct {
	Num   int
	Dir   string // "" for a root-level chapter, else the subdirectory it's in
	Title string
	Body  string
}

func mustRead(path string) string {
	b, err := fs.ReadFile(contentRoot(), path)
	if err != nil {
		// Missing under HOSTSHELL_CONTENT_DIR means a bad sync; missing
		// from the embedded copy means the binary itself is broken —
		// either way, fail loud and early rather than silently
		// rendering a blank screen.
		panic("content: missing file " + path + ": " + err.Error())
	}
	return strings.TrimRight(string(b), "\n")
}

// contentRoot is the filesystem every content file is read from.
// HOSTSHELL_CONTENT_DIR on disk if set, otherwise the copy embedded in
// the binary at build time — see the package doc above.
func contentRoot() fs.FS {
	if dir := os.Getenv("HOSTSHELL_CONTENT_DIR"); dir != "" {
		return os.DirFS(dir)
	}
	sub, err := fs.Sub(files, "data")
	if err != nil {
		panic("content: bad embedded data root: " + err.Error())
	}
	return sub
}

// AboutSection is one section of the About screen, rendered like a man
// page section: an ALL-CAPS header (NAME, SYNOPSIS, DESCRIPTION, ...)
// followed by indented body lines.
type AboutSection struct {
	Header string
	Body   []string // raw lines, kept separate — not joined into one
	// paragraph, so callers can tell "one item per line" (SKILLS) apart
	// from ordinary prose that should wrap as a paragraph.
}

// AboutSections returns the About screen's content, parsed from
// data/about.txt. Blocks are separated by "===" like projects.txt; each
// block's first line is the section header, everything after is body.
func AboutSections() []AboutSection {
	blocks := splitBlocks(mustRead("about.txt"))
	out := make([]AboutSection, 0, len(blocks))

	for _, b := range blocks {
		lines := strings.Split(b, "\n")
		if len(lines) == 0 {
			continue
		}
		header := strings.TrimSpace(lines[0])
		body := make([]string, 0, len(lines)-1)
		for _, l := range lines[1:] {
			l = strings.TrimSpace(l)
			if l != "" {
				body = append(body, l)
			}
		}
		out = append(out, AboutSection{Header: header, Body: body})
	}
	return out
}

// Contact returns the Contact screen's text.
func Contact() string { return mustRead("contact.txt") }

// Projects returns the Projects list, parsed from data/projects.txt.
func Projects() []Project {
	blocks := splitBlocks(mustRead("projects.txt"))
	out := make([]Project, 0, len(blocks))

	for _, b := range blocks {
		lines := strings.SplitN(b, "\n", 7)
		if len(lines) < 7 {
			continue // malformed block — skip rather than crash the UI
		}
		out = append(out, Project{
			Name:    strings.TrimSpace(lines[0]),
			Status:  strings.TrimSpace(lines[1]),
			RepoURL: strings.TrimSpace(lines[2]),
			Tagline: strings.TrimSpace(lines[3]),
			Stack:   strings.TrimSpace(lines[4]),
			License: strings.TrimSpace(lines[5]),
			Desc:    joinDescLines(lines[6]),
		})
	}
	return out
}

// fileHeader is the parsed "Key: value" header block at the top of a
// servercodex/writeups file, plus everything after the first blank line
// as the body.
type fileHeader struct {
	fields map[string]string
	body   string
}

func parseHeaderFile(raw string) fileHeader {
	lines := strings.Split(raw, "\n")
	fields := map[string]string{}

	i := 0
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			break
		}
		idx := strings.Index(line, ":")
		if idx == -1 {
			break // not a header line — treat everything from here as body
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		fields[key] = val
	}

	body := joinDescLines(strings.Join(lines[i:], "\n"))
	return fileHeader{fields: fields, body: body}
}

// walkedFile is one file found under a content directory, kept with its
// path so results can be sorted into a stable, filename-driven order
// before the path itself is discarded.
type walkedFile struct {
	path   string
	header fileHeader
}

// walkContentDir walks root/dir (either the embedded copy or a real
// on-disk override — see contentRoot) and returns every file found,
// sorted by path. A missing directory just yields no results rather
// than an error, so a category with nothing in it yet doesn't break
// the screen.
func walkContentDir(dir string) []walkedFile {
	root := contentRoot()
	var out []walkedFile

	_ = fs.WalkDir(root, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		raw, rerr := fs.ReadFile(root, path)
		if rerr != nil {
			return nil
		}
		out = append(out, walkedFile{path: path, header: parseHeaderFile(string(raw))})
		return nil
	})

	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// Evergreen, Signal, and Incidents return the three Writeups tabs, each
// discovered by walking its own real directory under data/writeups/.
func Evergreen() []Writeup { return loadWriteups("writeups/evergreen") }
func Signal() []Writeup    { return loadWriteups("writeups/signal") }
func Incidents() []Writeup { return loadWriteups("writeups/incidents") }

func loadWriteups(dir string) []Writeup {
	files := walkContentDir(dir)
	out := make([]Writeup, 0, len(files))
	for _, f := range files {
		title := f.header.fields["Title"]
		if title == "" {
			title = f.path
		}
		out = append(out, Writeup{
			Title: title,
			Date:  f.header.fields["Date"],
			Desc:  f.header.body,
		})
	}
	return out
}

// Codex returns the ServerCodex chapter tree, discovered by walking
// data/servercodex/. A file directly under servercodex/ is a root-level
// chapter; a file under servercodex/<folder>/ belongs to that
// subdirectory in the tree view. Chapter numbers are assigned in
// discovery order (filename-sorted).
func Codex() []Chapter {
	files := walkContentDir("servercodex")
	out := make([]Chapter, 0, len(files))

	for i, f := range files {
		rel := strings.TrimPrefix(f.path, "servercodex/")
		dir := ""
		if idx := strings.LastIndex(rel, "/"); idx != -1 {
			dir = rel[:idx]
		}
		title := f.header.fields["Title"]
		if title == "" {
			title = rel
		}
		out = append(out, Chapter{Num: i + 1, Dir: dir, Title: title, Body: f.header.body})
	}
	return out
}

// splitBlocks splits on a line containing only "===" and drops any
// empty leading/trailing blocks (trailing blank lines in the file, etc).
// Still used by About/Projects, which stayed single-file formats.
func splitBlocks(raw string) []string {
	parts := strings.Split(raw, "\n===\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// joinDescLines takes the (possibly multi-line) remainder of a block and
// joins it into a single space-separated string — the TUI's own wrap()
// re-flows it to terminal width, so line breaks in the source file are
// just for readability when editing, not meaningful formatting.
func joinDescLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}
