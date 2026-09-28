// Servidor HTTP del API (equivale a `edisys serve`).
package main

import (
	"os"

	"edisys/api/internal/arranque"
)

func main() { os.Exit(arranque.Main([]string{"serve"})) }
