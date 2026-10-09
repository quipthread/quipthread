// Command gencatalog writes plans/catalog_gen.go from packages/plans/catalog.json.
// Run it through go generate in backend/plans.
package main

import (
	"log"
	"os"

	"github.com/quipthread/quipthread/plans/internal/catalogfile"
)

func main() {
	source, err := os.ReadFile(catalogfile.SourcePath)
	if err != nil {
		log.Fatal(err)
	}
	out, err := catalogfile.Render(source)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("catalog_gen.go", out, 0o644); err != nil { //nolint:gosec // generated source file, world-readable like the rest of the repo
		log.Fatal(err)
	}
}
