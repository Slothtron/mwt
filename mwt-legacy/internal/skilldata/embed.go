package skilldata

import "embed"

// FS holds the embedded mwt Agent skill (source of mwt skill sync).
//
//go:embed all:mwt
var FS embed.FS
