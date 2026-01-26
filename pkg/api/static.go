package api

import (
	"embed"
	"io/fs"
	"net/http"
)

// StaticFS will be set by the binary that embeds the static files
// This allows different binaries (validator, supervisor) to provide their own static files
var StaticFS embed.FS

// setupStaticRoutes configures static file serving for the web UI
func (s *Server) setupStaticRoutes() {
	// Check if static FS is available (it may not be set for validator-only mode)
	// Extract the web/static subdirectory from the embedded filesystem
	staticFS, err := fs.Sub(StaticFS, "web/static")
	if err != nil {
		// No static files available, skip serving
		return
	}

	// Serve static files at root path
	// This is registered last, so API routes take precedence
	fileServer := http.FileServer(http.FS(staticFS))
	s.router.PathPrefix("/").Handler(fileServer)
}

// SetStaticFS allows setting the static filesystem from external packages
func SetStaticFS(fs embed.FS) {
	StaticFS = fs
}

