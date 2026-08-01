//go:build production

package web

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var files embed.FS

func Assets() fs.FS {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return dist
}
