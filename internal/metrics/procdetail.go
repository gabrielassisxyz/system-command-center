package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// detailMaxLen is the length past which the generic fallback stops being an
// identifier. A command line that long is configuration shared by every
// instance of the program (a browser's ~20 startup flags), so it fills the
// column without telling one row from another and is dropped instead.
const detailMaxLen = 60

// interpreters are programs whose own name says nothing: what tells one
// instance from the next is the script it was handed.
var interpreters = map[string]bool{
	"node":    true,
	"python":  true,
	"python3": true,
	"deno":    true,
	"bun":     true,
	"ruby":    true,
	"perl":    true,
}

// ProcessDetail derives a short label that tells apart processes sharing a
// name — 27 "brave" rows or 12 "node" rows are otherwise indistinguishable.
// It applies the most specific rule that matches and falls back to the trimmed
// command line, so an unrecognised program degrades visibly rather than
// silently losing its label.
func ProcessDetail(name string, cmdline []string, cwd string) string {
	if d := chromiumDetail(cmdline); d != "" {
		return d
	}
	if d := postgresDetail(cmdline); d != "" {
		return d
	}
	if d := scriptDetail(name, cmdline); d != "" {
		return d
	}
	if base := usefulCwd(cwd); base != "" {
		return base
	}

	args := argsOnly(cmdline)
	if len([]rune(args)) > detailMaxLen {
		return ""
	}
	return args
}

// cmdlineTokens flattens a command line into individual arguments. Chromium
// writes its whole command line as one space-separated string with no NUL
// separators, so gopsutil hands back every argument inside a single element;
// splitting on whitespace normalises that against the usual NUL-separated form.
func cmdlineTokens(cmdline []string) []string {
	tokens := make([]string, 0, len(cmdline))
	for _, part := range cmdline {
		tokens = append(tokens, strings.Fields(part)...)
	}
	return tokens
}

// usefulCwd rejects working directories that say nothing about the process: a
// sandboxed Chromium child is chdir'd into /proc/<pid>/fdinfo, and a process
// sitting at the filesystem root or in the user's home names no project.
func usefulCwd(cwd string) string {
	if cwd == "" || cwd == "/" || strings.HasPrefix(cwd, "/proc/") {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && cwd == home {
		return ""
	}
	return baseName(cwd)
}

// chromiumDetail names a Chromium child process by its role. A renderer's
// --renderer-client-id is an internal counter with no mapping to a tab title or
// extension name, so the role is as specific as /proc can get here.
func chromiumDetail(cmdline []string) string {
	var kind, subType string
	extension := false
	for _, arg := range cmdlineTokens(cmdline) {
		switch {
		case strings.HasPrefix(arg, "--type="):
			kind = strings.TrimPrefix(arg, "--type=")
		case strings.HasPrefix(arg, "--utility-sub-type="):
			subType = strings.TrimPrefix(arg, "--utility-sub-type=")
		case arg == "--extension-process":
			extension = true
		}
	}
	if kind == "" {
		return ""
	}

	// Utility processes carry a named service, e.g. network.mojom.NetworkService.
	if subType != "" {
		if i := strings.LastIndex(subType, "."); i >= 0 {
			subType = subType[i+1:]
		}
		return splitCamel(subType)
	}

	switch {
	case kind == "renderer" && extension:
		return "extension"
	case kind == "gpu-process":
		return "GPU"
	}
	return kind
}

// postgresDetail uses PostgreSQL's own process-title rewriting: a backend
// advertises its role, database, client address and state in argv itself, so
// the label needs no guessing on our side.
func postgresDetail(cmdline []string) string {
	if len(cmdline) == 0 {
		return ""
	}
	const prefix = "postgres:"
	joined := strings.TrimSpace(strings.Join(cmdline, " "))
	if !strings.HasPrefix(joined, prefix) {
		return ""
	}
	// Collapse the padding postgres leaves in the rewritten title.
	return strings.Join(strings.Fields(strings.TrimPrefix(joined, prefix)), " ")
}

// scriptDetail names an interpreted process by its script — "nuq-worker.js"
// rather than a twelfth identical "node".
func scriptDetail(name string, cmdline []string) string {
	tokens := cmdlineTokens(cmdline)
	if !interpreters[strings.ToLower(name)] || len(tokens) < 2 {
		return ""
	}
	for _, arg := range tokens[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if base := baseName(arg); base != "" {
			return base
		}
	}
	return ""
}

func baseName(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" || p == "." {
		return ""
	}
	return filepath.Base(p)
}

func argsOnly(cmdline []string) string {
	tokens := cmdlineTokens(cmdline)
	if len(tokens) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(tokens[1:], " "))
}

// splitCamel turns "NetworkService" into "Network Service" so a service name
// reads as words in the table.
func splitCamel(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
