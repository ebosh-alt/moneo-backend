package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDomainContextsDoNotImportEachOther(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path")
	}

	internalDir := filepath.Dir(filepath.Dir(filename))
	domainDir := filepath.Join(internalDir, "domain")
	moduleDomainPath := "moneo/backend/internal/domain/"

	err := filepath.WalkDir(domainDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		importerContext, ok := domainContext(domainDir, path)
		if !ok || importerContext == "shared" {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if !strings.HasPrefix(importPath, moduleDomainPath) {
				continue
			}

			imported := strings.TrimPrefix(importPath, moduleDomainPath)
			importedContext := strings.Split(imported, "/")[0]
			if importedContext == "shared" || importedContext == importerContext {
				continue
			}

			t.Fatalf(
				"%s imports domain context %q from domain context %q; coordinate cross-context work in internal/app",
				path,
				importedContext,
				importerContext,
			)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func domainContext(domainDir, filePath string) (string, bool) {
	rel, err := filepath.Rel(domainDir, filePath)
	if err != nil {
		return "", false
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return "", false
	}

	return parts[0], true
}
