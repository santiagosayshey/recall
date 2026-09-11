package boundaries

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// The analyzer is checked with hand-built passes rather than analysistest,
// which would need a second module on disk: what matters is which import
// paths are reported for which package.
func TestBoundaries(t *testing.T) {
	cases := []struct {
		pkg     string
		imports []string
		want    int
	}{
		{"internal/webhook", []string{"encoding/json"}, 0},
		{"internal/webhook", []string{"github.com/santiagosayshey/recall/internal/store"}, 1},
		{"internal/webhook", []string{"github.com/santiagosayshey/recall/internal/webhook/webhooktest"}, 0},
		{"internal/decide", []string{"github.com/santiagosayshey/recall/internal/store"}, 1},
		{"internal/decide", []string{"github.com/santiagosayshey/recall/internal/webhook"}, 1},
		{"internal/arr", []string{"github.com/santiagosayshey/recall/internal/store"}, 1},
		{"internal/store", []string{"github.com/santiagosayshey/recall/internal/webhook"}, 0},
		{"internal/store", []string{"github.com/santiagosayshey/recall/internal/decide"}, 0},
		{"internal/store", []string{"github.com/santiagosayshey/recall/internal/server"}, 1},
		{"internal/store", []string{"github.com/santiagosayshey/recall/internal/arr"}, 1},
		{"internal/server", []string{"github.com/santiagosayshey/recall/internal/store", "github.com/santiagosayshey/recall/internal/decide"}, 0},
		{"cmd/recall", []string{"github.com/santiagosayshey/recall/internal/server"}, 0},
	}
	for _, c := range cases {
		src := "package p\n"
		for _, imp := range c.imports {
			src += "import _ \"" + imp + "\"\n"
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "p.go", src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		var got int
		pass := &analysis.Pass{
			Analyzer: Analyzer,
			Fset:     fset,
			Files:    []*ast.File{f},
			Pkg:      types.NewPackage(module+c.pkg, "p"),
			Report:   func(analysis.Diagnostic) { got++ },
		}
		if _, err := run(pass); err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s importing %v: want %d reports, got %d", c.pkg, c.imports, c.want, got)
		}
	}
}
