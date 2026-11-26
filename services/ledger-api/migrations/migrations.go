// Package migrations embeds the goose SQL files into the binary, so the
// image needs no files on disk (distroless, read-only root filesystem).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
