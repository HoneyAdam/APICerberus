package apicerberus

import (
	"embed"
	"io/fs"
)

var (
	// webDistFS embeds the built React dashboard/portal assets.
	//
	//go:embed web/dist/*
	webDistFS embed.FS
)

// EmbeddedDashboardFS returns the embedded admin dashboard filesystem rooted at web/dist.
func EmbeddedDashboardFS() (fs.FS, error) {
	return fs.Sub(webDistFS, "web/dist")
}

// EmbeddedPortalFS returns the embedded portal filesystem rooted at web/dist.
func EmbeddedPortalFS() (fs.FS, error) {
	return fs.Sub(webDistFS, "web/dist")
}
