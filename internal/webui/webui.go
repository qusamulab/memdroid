// Package webui serves the browser interface embedded in the memdroid binary.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var assets embed.FS

// Handler serves the UI without a separate frontend build or asset directory.
func Handler() http.Handler {
	static, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err) // The embedded directory is checked by the Go compiler.
	}
	return http.FileServer(http.FS(static))
}
