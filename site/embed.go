package site

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

func Assets() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}

func Icons() (fs.FS, error) {
	return fs.Sub(assets, "dist/icons")
}
