package main

import "embed"

//go:embed templates
var templateFS embed.FS

//go:embed static templates new
var staticFS embed.FS

//go:embed data
var dataFS embed.FS
