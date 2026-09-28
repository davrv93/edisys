// Package auth: claves con bcrypt, JWT HS256, tokens opacos y límite de intentos.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// CostoBcrypt: §2.4.
const CostoBcrypt = 12

// Nombres de cookie (§2.4).
const (
	CookieAcceso  = "edisys_at"
	CookieRefresh = "edisys_rt"
	DuracionJWT   = 15 * time.Minute
	DuracionRT    = 30 * 24 * time.Hour
)

// HashClave genera el hash bcrypt.
func HashClave(clave string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(clave), CostoBcrypt)
	return string(h), err
}

// ClaveCorrecta compara en tiempo constante.
func ClaveCorrecta(hash, clave string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(clave)) == nil
}

// Claims del JWT: sub (usuario), adm (administradora), edf (edificios), ver (versión de permisos).
type Claims struct {
	Adm int64   `json:"adm"`
	Edf []int64 `json:"edf"`
	Ver int     `json:"ver"`
	jwt.RegisteredClaims
}

// FirmarJWT emite un token de acceso de 15 minutos.
func FirmarJWT(secreto string, usuarioID, adm int64, edificios []int64, ver int) (string, time.Time, error) {
	vence := time.Now().Add(DuracionJWT)
	c := Claims{Adm: adm, Edf: edificios, Ver: ver, RegisteredClaims: jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(usuarioID, 10),
		ExpiresAt: jwt.NewNumericDate(vence),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    "edisys",
	}}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secreto))
	return s, vence, err
}

// ErrToken: token inválido o vencido.
var ErrToken = errors.New("token inválido")

// LeerJWT valida firma, algoritmo y vencimiento.
func LeerJWT(secreto, token string) (int64, *Claims, error) {
	c := &Claims{}
	t, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrToken
		}
		return []byte(secreto), nil
	}, jwt.WithIssuer("edisys"), jwt.WithExpirationRequired())
	if err != nil || !t.Valid {
		return 0, nil, ErrToken
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0, nil, ErrToken
	}
	return id, c, nil
}

// TokenOpaco genera un token aleatorio (refresh, invitación) y su hash para guardar.
func TokenOpaco() (token, hash string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken: sha256 en hex.
func HashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

// Limitador cuenta fallos por clave en una ventana (5 fallos en 15 min → bloqueo).
type Limitador struct {
	mu      sync.Mutex
	max     int
	ventana time.Duration
	fallos  map[string][]time.Time
}

// NuevoLimitador crea un limitador.
func NuevoLimitador(max int, ventana time.Duration) *Limitador {
	return &Limitador{max: max, ventana: ventana, fallos: map[string][]time.Time{}}
}

func (l *Limitador) limpiar(k string, ahora time.Time) []time.Time {
	v := l.fallos[k][:0]
	for _, t := range l.fallos[k] {
		if ahora.Sub(t) < l.ventana {
			v = append(v, t)
		}
	}
	if len(v) == 0 {
		delete(l.fallos, k)
		return nil
	}
	l.fallos[k] = v
	return v
}

// Bloqueado dice si alguna clave superó el máximo y cuántos minutos faltan.
func (l *Limitador) Bloqueado(claves ...string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ahora := time.Now()
	for _, k := range claves {
		v := l.limpiar(k, ahora)
		if len(v) >= l.max {
			espera := l.ventana - ahora.Sub(v[0])
			return true, int(espera.Minutes()) + 1
		}
	}
	return false, 0
}

// Fallo registra un intento fallido.
func (l *Limitador) Fallo(claves ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range claves {
		l.fallos[k] = append(l.fallos[k], time.Now())
	}
}

// Limpiar borra los fallos tras un ingreso correcto.
func (l *Limitador) Limpiar(claves ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range claves {
		delete(l.fallos, k)
	}
}
