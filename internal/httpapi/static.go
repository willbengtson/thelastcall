package httpapi

import (
	"path"
	"strings"
)

// cleanFSPath turns a URL path into an io/fs path.
func cleanFSPath(urlPath string) string {
	return strings.TrimPrefix(path.Clean("/"+urlPath), "/")
}

// isHashedAsset reports whether a path is a Vite content-hashed build output,
// which may be cached forever.
func isHashedAsset(urlPath string) bool {
	return strings.HasPrefix(cleanFSPath(urlPath), "assets/build/")
}
