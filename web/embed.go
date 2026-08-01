//go:build !production

package web

import (
	"io/fs"
	"testing/fstest"
)

// Assets is intentionally empty in development and tests. Vite serves the UI separately.
func Assets() fs.FS { return fstest.MapFS{} }
