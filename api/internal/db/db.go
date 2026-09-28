// Package db abre el pool de PostgreSQL y aplica las migraciones numeradas.
package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"edisys/api/migrations"
)

// Q es lo común a *pgxpool.Pool y pgx.Tx.
type Q interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Abrir crea el pool (15 conexiones, §1.2) y espera a que la base responda.
func Abrir(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 15
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	var pool *pgxpool.Pool
	for intento := 1; intento <= 30; intento++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		slog.Info("esperando a PostgreSQL", "intento", intento, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("la base no responde: %w", err)
}

// Migrar aplica en orden los NNNN_*.sql que falten, cada uno en su transacción.
// Un candado consultivo evita que dos procesos migren a la vez.
func Migrar(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(424242)`); err != nil {
		return nil, err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(424242)`)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, aplicado_en timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	nombres, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(nombres)
	var aplicadas []string
	for _, n := range nombres {
		version := strings.TrimSuffix(n, ".sql")
		var existe bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&existe); err != nil {
			return aplicadas, err
		}
		if existe {
			continue
		}
		sql, err := migrations.FS.ReadFile(n)
		if err != nil {
			return aplicadas, err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return aplicadas, err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return aplicadas, fmt.Errorf("migración %s: %w", n, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			tx.Rollback(ctx)
			return aplicadas, err
		}
		if err := tx.Commit(ctx); err != nil {
			return aplicadas, fmt.Errorf("migración %s: %w", n, err)
		}
		aplicadas = append(aplicadas, version)
		slog.Info("migración aplicada", "version", version)
	}
	return aplicadas, nil
}

// Filas ejecuta una consulta y devuelve cada fila como mapa columna → valor (JSON directo).
// Nunca devuelve nil: una lista vacía sale como [].
func Filas(ctx context.Context, q Q, sql string, args ...any) ([]map[string]any, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

// Fila devuelve una sola fila como mapa (pgx.ErrNoRows si no hay).
func Fila(ctx context.Context, q Q, sql string, args ...any) (map[string]any, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToMap)
}
