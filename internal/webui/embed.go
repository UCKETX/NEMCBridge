// Package webui embeds the management console assets.
package webui

import "embed"

// Files contains only the static assets served by the console.
//go:embed *.html *.js *.css *.svg
var Files embed.FS
