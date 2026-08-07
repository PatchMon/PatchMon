package main

import "embed"

//go:embed static/frontend/dist
var frontendFS embed.FS
