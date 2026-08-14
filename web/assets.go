package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed src/*
var source embed.FS

func Handler() http.Handler {
	content, err := fs.Sub(source, "src")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(content))
}
