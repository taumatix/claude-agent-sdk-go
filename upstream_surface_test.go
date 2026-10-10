package surface_test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type surfaceRow struct {
	symbol, status, where string
}

func readSurface(t *testing.T) []surfaceRow {
	t.Helper()
	f, err := os.Open("docs/upstream-surface.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []surfaceRow
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			t.Fatalf("malformed line %q", line)
		}
		rows = append(rows, surfaceRow{parts[0], parts[1], parts[2]})
	}
	return rows
}

// goDeclared reports whether package directory dir declares the exported name.
func goDeclared(t *testing.T, dir, name string) bool {
	t.Helper()
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", dir, err)
	}
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil && d.Name.Name == name {
						return true
					}
				case *ast.GenDecl:
					for _, s := range d.Specs {
						switch s := s.(type) {
						case *ast.TypeSpec:
							if s.Name.Name == name {
								return true
							}
						case *ast.ValueSpec:
							for _, n := range s.Names {
								if n.Name == name {
									return true
								}
							}
						}
					}
				}
			}
		}
	}
	return false
}

// Every claim that a Go symbol covers an upstream name must name a symbol that exists, and
// every row must say why it is missing or skipped. The inventory cannot rot into a list of
// names that no longer resolve.
func TestUpstreamSurfaceInventoryIsHonest(t *testing.T) {
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, r := range readSurface(t) {
		if seen[r.symbol] {
			t.Errorf("%s listed twice", r.symbol)
		}
		seen[r.symbol] = true
		counts[r.status]++
		switch r.status {
		case "ported", "partial":
			ref, _, _ := strings.Cut(r.where, ":")
			i := strings.LastIndex(ref, ".")
			if i < 0 {
				t.Errorf("%s: %q is not pkg/dir.Name", r.symbol, r.where)
				continue
			}
			if !goDeclared(t, filepath.FromSlash(ref[:i]), ref[i+1:]) {
				t.Errorf("%s: %s is not declared", r.symbol, ref)
			}
			if r.status == "partial" && !strings.Contains(r.where, ":") {
				t.Errorf("%s: a partial row must say what is missing", r.symbol)
			}
		case "missing", "skipped":
			if strings.TrimSpace(r.where) == "" {
				t.Errorf("%s: %s with no reason", r.symbol, r.status)
			}
		default:
			t.Errorf("%s: unknown status %q", r.symbol, r.status)
		}
	}
	t.Logf("upstream names: %d ported, %d partial, %d missing, %d skipped",
		counts["ported"], counts["partial"], counts["missing"], counts["skipped"])
}

// UPSTREAM_PYTHON_SDK points at a checkout of anthropics/claude-agent-sdk-python. With it,
// the inventory must name exactly the symbols that checkout exports, so a new upstream
// export fails here until someone says what this SDK does about it.
func TestUpstreamSurfaceMatchesUpstream(t *testing.T) {
	dir := os.Getenv("UPSTREAM_PYTHON_SDK")
	if dir == "" {
		t.Skip("set UPSTREAM_PYTHON_SDK to a checkout of claude-agent-sdk-python")
	}
	src, err := os.ReadFile(filepath.Join(dir, "src", "claude_agent_sdk", "__init__.py"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)__all__\s*=\s*\[(.*?)\n\]`).FindSubmatch(src)
	if block == nil {
		t.Fatal("no __all__ list in upstream __init__.py")
	}
	upstream := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(block[1], -1) {
		upstream[string(m[1])] = true
	}
	if len(upstream) < 100 {
		t.Fatalf("parsed only %d upstream names; the parser, not upstream, is probably wrong", len(upstream))
	}
	mine := map[string]bool{}
	for _, r := range readSurface(t) {
		mine[r.symbol] = true
		if !upstream[r.symbol] {
			t.Errorf("%s is in the inventory but upstream no longer exports it", r.symbol)
		}
	}
	for s := range upstream {
		if !mine[s] {
			t.Errorf("upstream exports %s and docs/upstream-surface.txt does not account for it", s)
		}
	}
}
