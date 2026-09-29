// Package config lee la configuración de variables de entorno.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config del API.
type Config struct {
	Puerto        string
	DatabaseURL   string
	JWTSecret     string
	CookieSecure  bool
	Version       string
	URLPublica    string // p. ej. http://localhost:4700 (para enlaces de invitación y recibos)
	S3Endpoint    string
	S3AccessKey   string
	S3SecretKey   string
	S3Bucket      string
	S3SSL         bool
	S3Region      string
	WhatsAppModo  string // simulado (por defecto) | evolution: interruptor maestro del servidor
	EvolutionURL  string
	EvolutionInst string
	EvolutionKey  string
	WebhookToken  string
	Tareas        bool // tareas programadas (liberar retenciones, reintentar outbox)
	// Correo: simulado (por defecto, no sale nada) | smtp. En local el SMTP es Mailpit.
	CorreoModo  string
	CorreoDe    string
	SMTPHost    string
	SMTPPuerto  string
	SMTPUsuario string
	SMTPClave   string // nunca se registra ni se devuelve
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	v, err := strconv.ParseBool(env(k, strconv.FormatBool(def)))
	if err != nil {
		return def
	}
	return v
}

// Cargar lee el entorno.
func Cargar() Config {
	return Config{
		Puerto:        env("PORT", "8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://edisys:edisys@localhost:4754/edisys?sslmode=disable"),
		JWTSecret:     env("JWT_SECRET", "cambia-este-secreto-de-desarrollo-de-32-bytes!"),
		CookieSecure:  envBool("COOKIE_SECURE", false),
		Version:       env("APP_VERSION", "dev"),
		URLPublica:    strings.TrimRight(env("URL_PUBLICA", "http://localhost:4700"), "/"),
		S3Endpoint:    env("S3_ENDPOINT", "localhost:4790"),
		S3AccessKey:   env("S3_ACCESS_KEY", "GK0e615b1c2d3e4f5a6b7c8d9e"),
		S3SecretKey:   env("S3_SECRET_KEY", "7d1f0c9b8a7e6d5c4b3a29181716151413121110a9b8c7d6e5f4a3b2c1d0e9f8"),
		S3Bucket:      env("S3_BUCKET", "edisys-privado"),
		S3SSL:         envBool("S3_SSL", false),
		S3Region:      env("S3_REGION", "garage"),
		WhatsAppModo:  env("WHATSAPP_MODO", "simulado"),
		EvolutionURL:  strings.TrimRight(env("EVOLUTION_URL", ""), "/"),
		EvolutionInst: env("EVOLUTION_INSTANCIA", ""),
		EvolutionKey:  env("EVOLUTION_APIKEY", ""),
		WebhookToken:  env("WHATSAPP_WEBHOOK_TOKEN", ""),
		Tareas:        envBool("TAREAS", true),
		CorreoModo:    env("CORREO_MODO", "simulado"),
		CorreoDe:      env("CORREO_DE", "EDISYS <no-responder@edisys.local>"),
		SMTPHost:      env("SMTP_HOST", ""),
		SMTPPuerto:    env("SMTP_PUERTO", "25"),
		SMTPUsuario:   env("SMTP_USUARIO", ""),
		SMTPClave:     env("SMTP_CLAVE", ""),
	}
}
