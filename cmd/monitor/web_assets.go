package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var embeddedWeb embed.FS

func embeddedHTTPFS() http.FileSystem {
	sub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}
