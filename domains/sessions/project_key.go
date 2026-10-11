package sessions

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maxSanitizedLength = 200

// ProjectKeyForDirectory returns the project key the Claude Code CLI files a directory's
// sessions under, which is also the SessionKey.ProjectKey to use for them in a SessionStore
// (ports upstream's project_key_for_directory). An empty dir means the current directory.
// The path is made absolute and symlinks are resolved when the directory exists; unlike
// upstream the result is not Unicode-normalised to NFC, so a path with decomposed characters
// (macOS HFS+) can differ from the CLI's.
func ProjectKeyForDirectory(dir string) string {
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	return sanitizePath(dir)
}

// sanitizePath is the CLI's directory name for a project path: every character that is not an
// ASCII letter or digit becomes "-", and a name over 200 characters is cut there and given a
// hash of the whole path.
func sanitizePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s := b.String()
	if len(s) <= maxSanitizedLength {
		return s
	}
	return s[:maxSanitizedLength] + "-" + simpleHash(path)
}

// simpleHash is the 31-multiplier string hash the CLI's JavaScript uses for long project
// names, in base 36. It runs over UTF-16 code units, as charCodeAt does.
func simpleHash(s string) string {
	var h int32
	for _, u := range utf16.Encode([]rune(s)) {
		h = h<<5 - h + int32(u)
	}
	n := int64(h)
	if n < 0 {
		n = -n
	}
	return strconv.FormatInt(n, 36)
}

// legacySanitizePath is how this package named project directories before it matched the
// CLI (separators and ":" became "-", a leading "-" was dropped). Directories it created are
// still found under that name.
func legacySanitizePath(path string) string {
	s := strings.ReplaceAll(path, string(os.PathSeparator), "-")
	s = strings.ReplaceAll(s, ":", "-")
	return strings.TrimPrefix(s, "-")
}
