package main

import "embed"

//go:embed all:static/frontend
var frontendFS embed.FS
