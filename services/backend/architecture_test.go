package assethub_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCleanArchitectureDependencyDirection(t *testing.T) {
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		normalized := filepath.ToSlash(path)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			switch {
			case strings.HasPrefix(normalized, "internal/domain/"):
				if strings.Contains(strings.Split(p, "/")[0], ".") || strings.HasPrefix(p, "assethub/") {
					t.Errorf("domain %s imports %s", path, p)
				}
			case strings.HasPrefix(normalized, "internal/application/"):
				if strings.Contains(strings.Split(p, "/")[0], ".") || strings.HasPrefix(p, "assethub/") && !(strings.HasPrefix(p, "assethub/internal/domain") || strings.HasPrefix(p, "assethub/internal/application/")) {
					t.Errorf("application %s imports %s", path, p)
				}
			case strings.HasPrefix(normalized, "internal/adapters/inbound/"):
				if strings.Contains(p, "/adapters/outbound/") {
					t.Errorf("inbound %s imports outbound %s", path, p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
