package webassets

import (
	"embed"
	"io/fs"
)

//go:generate go run ./cmd/syncweb

// embeddedFiles contains a generated copy of the repository root web/ directory.
//
//go:embed dist
var embeddedFiles embed.FS

func FS() (fs.FS, error) {
	return fs.Sub(embeddedFiles, "dist")
}
