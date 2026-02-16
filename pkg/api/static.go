package api

import (
	"net/http"
	"os"
	"path/filepath"
)

// setupStaticRoutes configures static file serving for the web UI
func (s *Server) setupStaticRoutes() {
	// Try to serve static files from web/static directory
	// This works when running from the project root
	staticDir := "web/static"

	// Check if the directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try alternative paths (when running from different directories)
		alternatives := []string{
			"../../web/static",
			"../web/static",
			"./web/static",
		}

		found := false
		for _, alt := range alternatives {
			if absPath, err := filepath.Abs(alt); err == nil {
				if _, err := os.Stat(absPath); err == nil {
					staticDir = absPath
					found = true
					break
				}
			}
		}

		if !found {
			// No static files available, skip serving
			return
		}
	}

	// Serve static files at root path
	// This is registered last, so API routes take precedence
	fileServer := http.FileServer(http.Dir(staticDir))
	s.router.PathPrefix("/").Handler(fileServer)
}
