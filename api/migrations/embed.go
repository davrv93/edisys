// Package migrations embebe los SQL numerados que aplica `edisys migrate`.
package migrations

import "embed"

// FS contiene los archivos NNNN_nombre.sql.
//
//go:embed *.sql
var FS embed.FS
