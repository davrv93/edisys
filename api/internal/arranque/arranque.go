// Package arranque tiene los comandos del binario: serve, migrate, seed y salud.
package arranque

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"edisys/api/internal/app"
	"edisys/api/internal/archivo"
	"edisys/api/internal/config"
	"edisys/api/internal/db"
	"edisys/api/internal/seed"
)

// Main despacha el subcomando.
func Main(args []string) int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	cfg := config.Cargar()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch cmd {
	case "serve", "servir":
		err = servir(ctx, cfg)
	case "migrate", "migrar":
		err = migrar(ctx, cfg)
	case "seed", "sembrar":
		fs := flag.NewFlagSet("seed", flag.ContinueOnError)
		pend := fs.Int("pendientes", 0, "deja sin leer las últimas N lecturas de setiembre (demo en vivo)")
		if err = fs.Parse(args); err == nil {
			err = sembrar(ctx, cfg, seed.Opciones{LecturasPendientes: *pend})
		}
	case "preparar":
		err = preparar(ctx, cfg)
	case "salud", "health":
		err = salud(cfg)
	default:
		fmt.Fprintf(os.Stderr, "uso: edisys [serve|migrate|seed [--pendientes=N]|preparar|salud]\n")
		return 2
	}
	if err != nil {
		slog.Error("falló", "comando", cmd, "err", err)
		return 1
	}
	return 0
}

func almacen(ctx context.Context, cfg config.Config) (*archivo.S3, error) {
	s3, err := archivo.NuevoS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3Region, cfg.S3SSL)
	if err != nil {
		return nil, err
	}
	adminURL, token := os.Getenv("GARAGE_ADMIN_URL"), os.Getenv("GARAGE_ADMIN_TOKEN")
	var ultimo error
	for i := 0; i < 20; i++ {
		c, cancel := context.WithTimeout(ctx, 8*time.Second)
		if adminURL != "" {
			ultimo = archivo.PrepararGarage(c, adminURL, token, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket)
		}
		if ultimo == nil {
			ultimo = s3.AsegurarCubo(c)
		}
		cancel()
		if ultimo == nil {
			return s3, nil
		}
		slog.Info("esperando al S3", "intento", i+1, "err", ultimo)
		time.Sleep(2 * time.Second)
	}
	return s3, ultimo
}

func servir(ctx context.Context, cfg config.Config) error {
	if len(cfg.JWTSecret) < 32 {
		return errors.New("JWT_SECRET debe tener al menos 32 bytes")
	}
	pool, err := db.Abrir(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	s3, err := almacen(ctx, cfg)
	if err != nil {
		slog.Warn("S3 no disponible al arrancar; las subidas fallarán hasta que responda", "err", err)
	}
	srv, err := app.Nuevo(ctx, pool, s3, cfg)
	if err != nil {
		return err
	}
	if cfg.Tareas {
		go srv.Tareas(ctx)
	}
	h := &http.Server{Addr: ":" + cfg.Puerto, Handler: srv.Rutas(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.Shutdown(c)
	}()
	slog.Info("EDISYS API escuchando", "puerto", cfg.Puerto, "version", cfg.Version, "whatsapp", cfg.WhatsAppModo)
	if err := h.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func migrar(ctx context.Context, cfg config.Config) error {
	pool, err := db.Abrir(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	aplicadas, err := db.Migrar(ctx, pool)
	if err != nil {
		return err
	}
	slog.Info("migraciones al día", "aplicadas", len(aplicadas))
	return nil
}

func sembrar(ctx context.Context, cfg config.Config, op seed.Opciones) error {
	pool, err := db.Abrir(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if _, err := db.Migrar(ctx, pool); err != nil {
		return err
	}
	s3, err := almacen(ctx, cfg)
	if err != nil {
		return err
	}
	res, err := seed.Sembrar(ctx, pool, s3, op)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(res)
	slog.Info("semilla aplicada", "resumen", string(b))
	return nil
}

// preparar: migra y, según SEMBRAR, siembra el Edificio Demo.
// SEMBRAR=si_vacia (por defecto) solo siembra si no hay edificios; SEMBRAR=siempre borra y siembra; SEMBRAR=no nunca.
func preparar(ctx context.Context, cfg config.Config) error {
	if err := migrar(ctx, cfg); err != nil {
		return err
	}
	modo := os.Getenv("SEMBRAR")
	if modo == "" {
		modo = "si_vacia"
	}
	if modo == "no" {
		return nil
	}
	if modo == "si_vacia" {
		pool, err := db.Abrir(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}
		var n int
		err = pool.QueryRow(ctx, `SELECT count(*) FROM edificio`).Scan(&n)
		pool.Close()
		if err != nil {
			return err
		}
		if n > 0 {
			slog.Info("la base ya tiene datos: no se siembra (usa SEMBRAR=siempre o `make seed`)", "edificios", n)
			return nil
		}
	}
	pend := 0
	fmt.Sscanf(os.Getenv("SEMBRAR_PENDIENTES"), "%d", &pend)
	return sembrar(ctx, cfg, seed.Opciones{LecturasPendientes: pend})
}

// salud consulta /api/health (sirve de healthcheck en la imagen distroless, que no trae curl).
func salud(cfg config.Config) error {
	c := &http.Client{Timeout: 3 * time.Second}
	r, err := c.Get("http://127.0.0.1:" + cfg.Puerto + "/api/health")
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("health respondió %d", r.StatusCode)
	}
	return nil
}
