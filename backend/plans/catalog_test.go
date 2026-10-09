package plans

import (
	"bytes"
	"os"
	"testing"

	"github.com/quipthread/quipthread/plans/internal/catalogfile"
)

// catalog_gen.go must match packages/plans/catalog.json; run go generate in
// backend/plans after editing the catalog.
func TestCatalogMatchesSource(t *testing.T) {
	source, err := os.ReadFile(catalogfile.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := catalogfile.Render(source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("catalog_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("catalog_gen.go is out of date with packages/plans/catalog.json; run go generate ./plans")
	}
}
