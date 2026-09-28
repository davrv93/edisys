// Comando edisys: serve (por defecto), migrate, seed [--pendientes=N] y salud.
package main

import (
	"os"

	"edisys/api/internal/arranque"
)

func main() { os.Exit(arranque.Main(os.Args[1:])) }
