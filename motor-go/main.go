// EDISYS · motor conversacional en Go: puerto de motor/app.py con el mismo
// contrato HTTP (CONTRATO.md) y los mismos archivos de índice.
//
// Variables de entorno (con los valores que app.py tenía fijos):
//
//	PUERTO=8080  LLAMA_URL=http://llama:8080  INDICE=/motor/indice
//	E5_RUTA=/models/e5-model_quantized.onnx  E5_TOK=/models/e5-tokenizer.json
//	ORT_LIB=/usr/local/lib/libonnxruntime.so  E5_HILOS=0 (0 = los de onnxruntime)
//	PRECARGAR=1 (carga e5 al arrancar; Python lo cargaba en la 1.ª petición)
//
// `motor -salud` consulta /v1/salud en 127.0.0.1 (HEALTHCHECK de la imagen).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"edisys/motor-go/internal/e5"
	"edisys/motor-go/internal/motor"
)

func env(k, defecto string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return defecto
}

func main() {
	salud := flag.Bool("salud", false, "comprueba /v1/salud y sale (0 = sano)")
	flag.Parse()
	puerto := env("PUERTO", "8080")
	if *salud {
		c := &http.Client{Timeout: 4 * time.Second}
		resp, err := c.Get("http://127.0.0.1:" + puerto + "/v1/salud")
		if err != nil || resp.StatusCode != 200 {
			os.Exit(1)
		}
		os.Exit(0)
	}
	log.SetFlags(log.LstdFlags | log.LUTC)

	hilos, _ := strconv.Atoi(env("E5_HILOS", "0"))
	cargar := func() (motor.Embebedor, error) {
		t0 := time.Now()
		e, err := e5.Nuevo(env("E5_RUTA", "/models/e5-model_quantized.onnx"),
			env("E5_TOK", "/models/e5-tokenizer.json"), env("ORT_LIB", ""), hilos)
		if err != nil {
			log.Printf("e5: no cargó: %v", err)
			return nil, err
		}
		log.Printf("e5 cargado en %v", time.Since(t0).Round(time.Millisecond))
		return e, nil
	}
	m := motor.Nuevo(env("INDICE", "/motor/indice"), env("LLAMA_URL", "http://llama:8080"), cargar)
	if env("PRECARGAR", "1") == "1" {
		go func() { _ = m.Precargar() }()
	}

	srv := &http.Server{
		Addr:              ":" + puerto,
		Handler:           m.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("EDISYS · motor (Go) %s en :%s", motor.Version, puerto)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
