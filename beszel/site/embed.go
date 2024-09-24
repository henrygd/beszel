// Package site handles the Beszel frontend embedding.
package site

import (
	"embed"

	"github.com/labstack/echo/v5"
)

//go:embed all:dist
var assets embed.FS

var Dist = echo.MustSubFS(assets, "dist")
