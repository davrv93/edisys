package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // decodificadores del logo
	_ "image/png"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	_ "golang.org/x/image/webp"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Marca blanca (bloque I2): cada administradora pone su nombre, lema, colores y logo. La app, el login,
// el correo y el recibo en PDF leen la misma marca; los tokens de color se calculan aquí y el front solo los aplica.

// Marca de una administradora (administradora.branding + slug).
type Marca struct {
	Nombre          string `json:"nombre"`
	Lema            string `json:"lema"`
	ColorPrimario   string `json:"color_primario"`
	ColorFondoLogin string `json:"color_fondo_login"`
	LogoArchivoID   int64  `json:"logo_archivo_id,omitempty"`
	Slug            string `json:"-"`
	AdmID           int64  `json:"-"`
}

const (
	marcaNombre     = "EDISYS"
	marcaPrimario   = "#155E75" // cyan-800, el acento de tokens.css
	marcaFondoLogin = "#0F172A" // slate-900, el fondo del login
	// ContrasteMinimo: WCAG AA para texto normal. Los botones llevan texto blanco sobre el color primario.
	ContrasteMinimo = 4.5
	maxLogo         = 2 << 20
)

var (
	reHex  = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	reSlug = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)
)

// conDefectos completa lo que la administradora no configuró con la marca EDISYS.
func (m Marca) conDefectos() Marca {
	if strings.TrimSpace(m.Nombre) == "" {
		m.Nombre = marcaNombre
	}
	if !reHex.MatchString(m.ColorPrimario) {
		m.ColorPrimario = marcaPrimario
	}
	if !reHex.MatchString(m.ColorFondoLogin) {
		m.ColorFondoLogin = marcaFondoLogin
	}
	m.ColorPrimario = strings.ToUpper(m.ColorPrimario)
	m.ColorFondoLogin = strings.ToUpper(m.ColorFondoLogin)
	return m
}

// ---------- color ----------

type rgb [3]float64 // 0–255

func hexARGB(h string) (rgb, bool) {
	if !reHex.MatchString(h) {
		return rgb{}, false
	}
	v, _ := strconv.ParseUint(h[1:], 16, 32)
	return rgb{float64(v >> 16 & 0xFF), float64(v >> 8 & 0xFF), float64(v & 0xFF)}, true
}

func (c rgb) hex() string {
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(c[0])), int(math.Round(c[1])), int(math.Round(c[2])))
}

// mezclar lleva c hacia otro color en la proporción t (0 = c, 1 = otro).
func (c rgb) mezclar(otro rgb, t float64) rgb {
	return rgb{c[0] + (otro[0]-c[0])*t, c[1] + (otro[1]-c[1])*t, c[2] + (otro[2]-c[2])*t}
}

// luminancia relativa (WCAG 2.x).
func (c rgb) luminancia() float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

// Contraste entre dos colores #RRGGBB (1 a 21). Con un color inválido devuelve 0.
func Contraste(a, b string) float64 {
	ca, ok1 := hexARGB(a)
	cb, ok2 := hexARGB(b)
	if !ok1 || !ok2 {
		return 0
	}
	l1, l2 := ca.luminancia(), cb.luminancia()
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

var (
	blanco = rgb{255, 255, 255}
	negro  = rgb{0, 0, 0}
)

// Tokens devuelve las variables CSS del acento para el tema claro y el oscuro (mismos nombres que tokens.css).
func (m Marca) Tokens() map[string]map[string]string {
	m = m.conDefectos()
	p, _ := hexARGB(m.ColorPrimario)
	return map[string]map[string]string{
		"claro": {
			"--color-acento":        p.hex(),
			"--color-acento-hover":  p.mezclar(negro, 0.18).hex(),
			"--color-acento-suave":  p.mezclar(blanco, 0.94).hex(),
			"--color-acento-borde":  p.mezclar(blanco, 0.82).hex(),
			"--color-acento-oscuro": p.mezclar(blanco, 0.55).hex(),
			"--color-fondo-login":   m.ColorFondoLogin,
		},
		"oscuro": {
			"--color-acento":       p.mezclar(blanco, 0.55).hex(),
			"--color-acento-hover": p.mezclar(blanco, 0.7).hex(),
			"--color-acento-suave": p.mezclar(negro, 0.6).hex(),
			"--color-acento-borde": p.hex(),
		},
	}
}

// ValidarMarca revisa lo que escribe la administradora. Invariante: el color primario lleva texto blanco
// encima (botones, cabecera del recibo y del correo), así que exige contraste AA con el blanco.
func ValidarMarca(m Marca) *P.Error {
	ev := P.Validacion("Revisa la marca.")
	if n := len([]rune(strings.TrimSpace(m.Nombre))); n == 0 || n > 40 {
		ev.Campo("nombre", "Entre 1 y 40 caracteres.")
	}
	if len([]rune(m.Lema)) > 80 {
		ev.Campo("lema", "Máximo 80 caracteres.")
	}
	if !reHex.MatchString(m.ColorPrimario) {
		ev.Campo("color_primario", "Usa un color #RRGGBB.")
	} else if c := Contraste(m.ColorPrimario, "#FFFFFF"); c < ContrasteMinimo {
		ev.Campo("color_primario", fmt.Sprintf("Muy claro: el texto blanco encima no se lee (contraste %.1f:1, mínimo 4,5:1).", c))
	}
	if !reHex.MatchString(m.ColorFondoLogin) {
		ev.Campo("color_fondo_login", "Usa un color #RRGGBB.")
	}
	if m.Slug != "" && !reSlug.MatchString(m.Slug) {
		ev.Campo("slug", "Solo minúsculas, cifras y guiones (hasta 40).")
	}
	if len(ev.Campos) > 0 {
		return ev
	}
	return nil
}

// ---------- lectura ----------

// marcaDeAdministradora lee la marca guardada (sin completar con los valores por defecto).
func (s *Server) marcaDeAdministradora(ctx context.Context, q db.Q, admID int64) (Marca, error) {
	var crudo []byte
	var slug *string
	if err := q.QueryRow(ctx, `SELECT branding, slug FROM administradora WHERE id=$1`, admID).Scan(&crudo, &slug); err != nil {
		return Marca{}, err
	}
	var m Marca
	_ = json.Unmarshal(crudo, &m)
	if slug != nil {
		m.Slug = *slug
	}
	m.AdmID = admID
	return m, nil
}

// marcaCruda: la marca guardada de la administradora del edificio (vacía si no hay).
func (s *Server) marcaCruda(ctx context.Context, eid int64) Marca {
	adm, err := admDeEdificio(ctx, s.DB, eid)
	if err != nil {
		return Marca{}
	}
	m, err := s.marcaDeAdministradora(ctx, s.DB, adm)
	if err != nil {
		return Marca{AdmID: adm}
	}
	return m
}

// MarcaDeEdificio: la marca de la administradora del edificio, ya completada. Nunca falla: sin datos, EDISYS.
func (s *Server) MarcaDeEdificio(ctx context.Context, eid int64) Marca {
	return s.marcaCruda(ctx, eid).conDefectos()
}

// logoMarca trae y decodifica el logo (nil si no hay o no se puede leer: el recibo sale con el nombre).
func (s *Server) logoMarca(ctx context.Context, m Marca) image.Image {
	if m.LogoArchivoID == 0 || s.Almacen == nil {
		return nil
	}
	var clave string
	if err := s.DB.QueryRow(ctx, `SELECT clave FROM archivo WHERE id=$1`, m.LogoArchivoID).Scan(&clave); err != nil {
		return nil
	}
	datos, err := s.Almacen.Leer(ctx, clave)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil
	}
	return img
}

// publica: lo que ven la app (/yo) y el login. El logo va con URL firmada de 10 minutos.
func (s *Server) marcaPublica(m Marca) map[string]any {
	personalizada := m.Nombre != "" || m.ColorPrimario != "" || m.LogoArchivoID != 0
	c := m.conDefectos()
	var logo any
	if c.LogoArchivoID != 0 {
		logo = s.Firma.URL(c.LogoArchivoID)
	}
	return map[string]any{"nombre": c.Nombre, "lema": c.Lema, "slug": c.Slug, "color_primario": c.ColorPrimario,
		"color_fondo_login": c.ColorFondoLogin, "logo_url": logo, "tokens": c.Tokens(), "personalizada": personalizada}
}

// ---------- endpoints ----------

func admDeEdificio(ctx context.Context, q db.Q, eid int64) (int64, error) {
	var adm int64
	err := q.QueryRow(ctx, `SELECT administradora_id FROM edificio WHERE id=$1`, eid).Scan(&adm)
	return adm, err
}

// verMarca: GET /marca — la marca de la administradora del edificio, con lo guardado y lo efectivo.
func (s *Server) verMarca(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adm, err := admDeEdificio(ctx, s.DB, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	m, err := s.marcaDeAdministradora(ctx, s.DB, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	out := s.marcaPublica(m)
	out["guardada"] = m
	out["contraste_minimo"] = ContrasteMinimo
	P.JSON(w, http.StatusOK, out)
}

// guardarMarca: PUT /marca {nombre, lema, slug, color_primario, color_fondo_login}. Conserva el logo.
func (s *Server) guardarMarca(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nombre          string `json:"nombre"`
		Lema            string `json:"lema"`
		Slug            string `json:"slug"`
		ColorPrimario   string `json:"color_primario"`
		ColorFondoLogin string `json:"color_fondo_login"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	adm, err := admDeEdificio(ctx, s.DB, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	antes, err := s.marcaDeAdministradora(ctx, s.DB, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	nueva := Marca{Nombre: strings.TrimSpace(in.Nombre), Lema: strings.TrimSpace(in.Lema), ColorPrimario: strings.ToUpper(strings.TrimSpace(in.ColorPrimario)),
		ColorFondoLogin: strings.ToUpper(strings.TrimSpace(in.ColorFondoLogin)), LogoArchivoID: antes.LogoArchivoID,
		Slug: strings.ToLower(strings.TrimSpace(in.Slug))}
	if ev := ValidarMarca(nueva); ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	var slug *string
	if nueva.Slug != "" {
		slug = &nueva.Slug
	}
	branding, _ := json.Marshal(nueva)
	if _, err := s.DB.Exec(ctx, `UPDATE administradora SET branding=$1, slug=$2 WHERE id=$3`, branding, slug, adm); err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("SLUG_OCUPADO", "Otra administradora ya usa ese identificador.").Campo("slug", "Elige otro."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "marca", "editar", "administradora", adm, conSlug(antes), conSlug(nueva))
	nueva.AdmID = adm
	P.JSON(w, http.StatusOK, s.marcaPublica(nueva))
}

// conSlug: la marca con el slug a la vista, para el registro de cambios.
func conSlug(m Marca) map[string]any {
	return map[string]any{"nombre": m.Nombre, "lema": m.Lema, "slug": m.Slug, "color_primario": m.ColorPrimario,
		"color_fondo_login": m.ColorFondoLogin, "logo_archivo_id": m.LogoArchivoID}
}

// subirLogo: POST /marca/logo (multipart «logo»): PNG, JPG o WebP de hasta 2 MB que se pueda decodificar.
func (s *Server) subirLogo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	e := edf(r)
	subidos, err := archivosDeForm(r, "logo")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if len(subidos) == 0 {
		P.Fallo(w, r, P.Validacion("Adjunta el logo.").Campo("logo", "Obligatorio."))
		return
	}
	a := subidos[0]
	if len(a.Datos) > maxLogo {
		P.Fallo(w, r, P.Err(http.StatusRequestEntityTooLarge, "ARCHIVO_GRANDE", "El logo pasa de 2 MB.").Campo("logo", "Máximo 2 MB."))
		return
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(a.Datos)); err != nil {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "TIPO_NO_PERMITIDO", "El logo debe ser PNG, JPG o WebP.").Campo("logo", "No es una imagen válida."))
		return
	}
	adm, err := admDeEdificio(ctx, s.DB, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	antes, err := s.marcaDeAdministradora(ctx, tx, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	id, err := s.guardarArchivo(ctx, tx, e.ID, &ses(r).UsuarioID, a)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE administradora SET branding = branding || jsonb_build_object('logo_archivo_id', $1::bigint) WHERE id=$2`, id, adm); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, tx, r, "marca", "logo", "administradora", adm, map[string]any{"logo_archivo_id": antes.LogoArchivoID}, map[string]any{"logo_archivo_id": id})
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"logo_archivo_id": id, "logo_url": s.Firma.URL(id)})
}

// quitarLogo: DELETE /marca/logo — vuelve al nombre en texto (el archivo queda en el almacén).
func (s *Server) quitarLogo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adm, err := admDeEdificio(ctx, s.DB, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	antes, _ := s.marcaDeAdministradora(ctx, s.DB, adm)
	if _, err := s.DB.Exec(ctx, `UPDATE administradora SET branding = branding - 'logo_archivo_id' WHERE id=$1`, adm); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "marca", "logo", "administradora", adm, map[string]any{"logo_archivo_id": antes.LogoArchivoID}, map[string]any{"logo_archivo_id": 0})
	P.JSON(w, http.StatusOK, map[string]any{"logo_archivo_id": nil})
}

// marcaPorSlug: GET /publico/marca/{slug} — sin sesión, para pintar el login con la marca.
// Solo expone lo que ya es público: nombre, lema, colores y el logo con URL firmada.
func (s *Server) marcaPorSlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(chi.URLParam(r, "slug"))
	if !reSlug.MatchString(slug) {
		P.Fallo(w, r, P.NoEncontrado("esa marca"))
		return
	}
	var adm int64
	if err := s.DB.QueryRow(r.Context(), `SELECT id FROM administradora WHERE slug=$1`, slug).Scan(&adm); err != nil {
		P.Fallo(w, r, P.NoEncontrado("esa marca"))
		return
	}
	m, err := s.marcaDeAdministradora(r.Context(), s.DB, adm)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	P.JSON(w, http.StatusOK, s.marcaPublica(m))
}
