// Package ui provides the embedded UI static files for the Flokoa server.
//
// The dist/ directory should contain the built Nuxt output (from `nuxt generate`).
// During development, run `cd ui && pnpm build` then copy `.output/public/` to `operator/ui/dist/`.
// In CI, this is handled by the build pipeline.
package ui

import "embed"

// DistFS embeds the built UI static files.
// When the dist/ directory is empty (development), the server falls back to
// redirecting to the Swagger UI instead.
//
//go:embed dist/*
var DistFS embed.FS
