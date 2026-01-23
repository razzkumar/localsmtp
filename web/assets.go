package web

import (
	"embed"
	"io/fs"
)

//go:embed dist
var Assets embed.FS

func Dist() (fs.FS, error) {
	return fs.Sub(Assets, "dist")
}
