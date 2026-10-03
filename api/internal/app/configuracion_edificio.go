package app

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Configuración del edificio (bloque I5): activarlo o desactivarlo, el asistente de puesta en marcha
// (qué falta configurar, calculado de los datos) y el registro de cambios, que reusa la tabla auditoria.

// ---------- activo ----------

// exigirEdificioActivo: un edificio desactivado se consulta, pero no admite escrituras.
// La única escritura que pasa es la que lo reactiva (PUT …/configuracion/activo).
func (s *Server) exigirEdificioActivo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		e := edf(r)
		if e == nil || strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/configuracion/activo") {
			next.ServeHTTP(w, r)
			return
		}
		activo := true
		_ = s.DB.QueryRow(r.Context(), `SELECT activo FROM edificio WHERE id=$1`, e.ID).Scan(&activo)
		if !activo {
			P.Fallo(w, r, P.Conflicto("EDIFICIO_INACTIVO", "El edificio está desactivado: puedes consultar, pero no registrar cambios. Reactívalo en Configuración."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// verEstadoEdificio: GET /configuracion/estado
func (s *Server) verEstadoEdificio(w http.ResponseWriter, r *http.Request) {
	fila, err := db.Fila(r.Context(), s.DB, `SELECT id, nombre, activo, desactivado_en, desactivado_motivo FROM edificio WHERE id=$1`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, fila)
}

// cambiarActivoEdificio: PUT /configuracion/activo {activo, motivo}. Desactivar exige el motivo.
func (s *Server) cambiarActivoEdificio(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Activo *bool  `json:"activo"`
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Motivo = strings.TrimSpace(in.Motivo)
	if in.Activo == nil {
		P.Fallo(w, r, P.Validacion("Indica si el edificio queda activo.").Campo("activo", "Obligatorio."))
		return
	}
	if !*in.Activo && len([]rune(in.Motivo)) < 5 {
		P.Fallo(w, r, P.Validacion("Explica por qué se desactiva.").Campo("motivo", "Escribe el motivo (5 caracteres o más)."))
		return
	}
	ctx := r.Context()
	e := edf(r)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	antes, err := db.Fila(ctx, tx, `SELECT activo, desactivado_motivo FROM edificio WHERE id=$1 FOR UPDATE`, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if antes["activo"] == *in.Activo {
		P.Fallo(w, r, P.Conflicto("SIN_CAMBIO", map[bool]string{true: "El edificio ya está activo.", false: "El edificio ya está desactivado."}[*in.Activo]))
		return
	}
	if *in.Activo {
		_, err = tx.Exec(ctx, `UPDATE edificio SET activo=true, desactivado_en=NULL, desactivado_motivo='' WHERE id=$1`, e.ID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE edificio SET activo=false, desactivado_en=now(), desactivado_motivo=$2 WHERE id=$1`, e.ID, in.Motivo)
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	accion := map[bool]string{true: "activar", false: "desactivar"}[*in.Activo]
	s.auditarCambio(ctx, tx, r, "configuracion", accion, "edificio", e.ID, antes, map[string]any{"activo": *in.Activo, "desactivado_motivo": in.Motivo})
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.verEstadoEdificio(w, r)
}

// ---------- asistente ----------

// PasoAsistente: un punto de la puesta en marcha. Pagina es la clave de la pantalla del front donde se resuelve.
type PasoAsistente struct {
	Clave   string `json:"clave"`
	Titulo  string `json:"titulo"`
	Detalle string `json:"detalle"`
	Hecho   bool   `json:"hecho"`
	Pagina  string `json:"pagina"`
}

// asistenteConfiguracion: GET /configuracion/asistentes — qué falta para operar el edificio, leído de los datos.
func (s *Server) asistenteConfiguracion(w http.ResponseWriter, r *http.Request) {
	pasos, err := s.PasosAsistente(r.Context(), edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	hechos := 0
	for _, p := range pasos {
		if p.Hecho {
			hechos++
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"pasos": pasos, "hechos": hechos, "total": len(pasos), "porcentaje": hechos * 100 / max(1, len(pasos))})
}

// PasosAsistente calcula cada paso con una sola consulta.
func (s *Server) PasosAsistente(ctx context.Context, eid int64) ([]PasoAsistente, error) {
	var (
		direccion, distrito                                string
		unidades, conPropietario, cuentas, periodos, junta int64
		participacion                                      float64
		tieneMarca, tienePlantilla                         bool
	)
	err := s.DB.QueryRow(ctx, `SELECT e.direccion, e.distrito,
			(SELECT count(*) FROM unidad u WHERE u.edificio_id=e.id AND u.activo),
			(SELECT count(DISTINCT u.id) FROM unidad u JOIN unidad_persona up ON up.unidad_id=u.id AND up.rol='propietario' AND up.hasta IS NULL
				WHERE u.edificio_id=e.id AND u.activo),
			COALESCE((SELECT sum(u.participacion_pct) FROM unidad u WHERE u.edificio_id=e.id AND u.activo),0)::float8,
			(SELECT count(*) FROM cuenta_bancaria c WHERE c.edificio_id=e.id AND c.activo),
			(SELECT count(*) FROM periodo p WHERE p.edificio_id=e.id),
			(SELECT count(*) FROM junta_miembro j WHERE j.edificio_id=e.id AND j.activo),
			(a.branding ? 'nombre' OR a.branding ? 'logo_archivo_id'),
			EXISTS (SELECT 1 FROM plantilla_recibo pr WHERE pr.edificio_id=e.id)
		FROM edificio e JOIN administradora a ON a.id=e.administradora_id WHERE e.id=$1`, eid).
		Scan(&direccion, &distrito, &unidades, &conPropietario, &participacion, &cuentas, &periodos, &junta, &tieneMarca, &tienePlantilla)
	if err != nil {
		return nil, err
	}
	return armarPasos(direccion, distrito, unidades, conPropietario, participacion, cuentas, periodos, junta, tieneMarca, tienePlantilla), nil
}

// armarPasos es la regla pura: qué cuenta como hecho en cada paso.
func armarPasos(direccion, distrito string, unidades, conPropietario int64, participacion float64, cuentas, periodos, junta int64, tieneMarca, tienePlantilla bool) []PasoAsistente {
	n := func(v int64, uno, varios string) string {
		if v == 1 {
			return "1 " + uno
		}
		return strconv.FormatInt(v, 10) + " " + varios
	}
	partOK := unidades > 0 && participacion > 99.99 && participacion < 100.01
	return []PasoAsistente{
		{"ficha", "Datos del edificio", "Dirección y distrito para los recibos.", strings.TrimSpace(direccion) != "" && strings.TrimSpace(distrito) != "", "unidades"},
		{"unidades", "Unidades", n(unidades, "unidad activa", "unidades activas") + ".", unidades > 0, "unidades"},
		{"participacion", "Participaciones al 100 %", "Suman " + P.Pct(participacion, 2) + ".", partOK, "unidades"},
		{"propietarios", "Propietarios asignados", strconv.FormatInt(conPropietario, 10) + " de " + strconv.FormatInt(unidades, 10) + " unidades con propietario.", unidades > 0 && conPropietario >= unidades, "unidades"},
		{"cuentas", "Cuenta bancaria", n(cuentas, "cuenta activa", "cuentas activas") + " para recibir pagos.", cuentas > 0, "vouchers"},
		{"periodo", "Primer periodo", n(periodos, "periodo abierto", "periodos abiertos") + ".", periodos > 0, "recibos"},
		{"junta", "Junta de propietarios", n(junta, "miembro", "miembros") + " de la junta.", junta > 0, "roles"},
		{"marca", "Marca de la administradora", "Nombre, colores y logo en la app, el login y el correo.", tieneMarca, "marca"},
		{"plantilla", "Plantilla del recibo", "Color, logo y bloques del recibo en PDF.", tienePlantilla, "marca"},
	}
}

// ---------- registro de cambios ----------

// Cambio: un campo que cambió entre antes y después.
type Cambio struct {
	Campo   string `json:"campo"`
	Antes   any    `json:"antes"`
	Despues any    `json:"despues"`
}

// DiferenciasCambio compara antes y después (objetos JSON): solo los campos que cambiaron, en orden alfabético.
// Si alguno no es un objeto, se comparan enteros bajo el campo «valor».
func DiferenciasCambio(antes, despues any) []Cambio {
	a, okA := comoObjeto(antes)
	d, okD := comoObjeto(despues)
	if !okA || !okD {
		if reflect.DeepEqual(normalizarJSON(antes), normalizarJSON(despues)) {
			return []Cambio{}
		}
		return []Cambio{{"valor", antes, despues}}
	}
	claves := map[string]bool{}
	for k := range a {
		claves[k] = true
	}
	for k := range d {
		claves[k] = true
	}
	orden := make([]string, 0, len(claves))
	for k := range claves {
		orden = append(orden, k)
	}
	sort.Strings(orden)
	out := []Cambio{}
	for _, k := range orden {
		if !reflect.DeepEqual(a[k], d[k]) {
			out = append(out, Cambio{k, a[k], d[k]})
		}
	}
	return out
}

// comoObjeto pasa cualquier valor por JSON y dice si es un objeto.
func comoObjeto(v any) (map[string]any, bool) {
	if v == nil {
		return map[string]any{}, true
	}
	m, ok := normalizarJSON(v).(map[string]any)
	return m, ok
}

func normalizarJSON(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// modulosConfiguracion: lo que cuenta como «configuración» en el registro de cambios.
var modulosConfiguracion = []string{"configuracion", "edificio", "marca", "recibos", "roles", "facturacion", "whatsapp", "mantenimiento"}

// registroCambios: GET /configuracion/cambios?modulo=&pagina= — filas de auditoría con antes/después,
// con la lista de campos que cambiaron ya calculada.
func (s *Server) registroCambios(w http.ResponseWriter, r *http.Request) {
	pagina, por := paginacion(r)
	modulo := r.URL.Query().Get("modulo")
	mods := modulosConfiguracion
	if modulo != "" {
		mods = []string{modulo}
	}
	e := edf(r)
	ctx := r.Context()
	var total int64
	_ = s.DB.QueryRow(ctx, `SELECT count(*) FROM auditoria WHERE edificio_id=$1 AND modulo = ANY($2) AND (antes IS NOT NULL OR despues IS NOT NULL)`, e.ID, mods).Scan(&total)
	filas, err := db.Filas(ctx, s.DB, `SELECT a.id, a.modulo, a.accion, a.entidad, a.entidad_id, a.antes, a.despues, a.creado_en, COALESCE(us.nombre,'Sistema') AS usuario
		FROM auditoria a LEFT JOIN usuario us ON us.id=a.usuario_id
		WHERE a.edificio_id=$1 AND a.modulo = ANY($2) AND (a.antes IS NOT NULL OR a.despues IS NOT NULL)
		ORDER BY a.id DESC LIMIT $3 OFFSET $4`, e.ID, mods, por, (pagina-1)*por)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		f["cambios"] = DiferenciasCambio(f["antes"], f["despues"])
		delete(f, "antes")
		delete(f, "despues")
	}
	out := paginado(filas, total, pagina)
	out["modulos"] = modulosConfiguracion
	P.JSON(w, http.StatusOK, out)
}
