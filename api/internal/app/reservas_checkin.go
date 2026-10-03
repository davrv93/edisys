package app

// Bloque H1 · check-in con QR. La reserva confirmada lleva una entrada (como la del cine): un token aleatorio en un QR.
// El conserje la escanea el día del evento y el sistema valida tres cosas: que esté confirmada, que estemos en la
// franja (con la tolerancia del área antes del inicio) y que la unidad SIGA al día ese día (reserva_unidad_habilitada).
// Para reservar hay que estar al día, y para entrar también. La base lo vuelve a comprobar (reserva_valida_checkin, 0028).

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	qrcode "github.com/skip2/go-qrcode"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// prefijoQR: el QR lleva «EDISYS-R:<token>» para distinguirlo de otros QR (Yape, recibos).
const prefijoQR = "EDISYS-R:"

var reCodigoReserva = regexp.MustCompile(`(?i)^R-\d+$`)

// motivoCheckin: por qué no se puede entrar (o un aviso).
type motivoCheckin struct {
	Codigo string `json:"codigo"`
	Texto  string `json:"texto"`
}

// ventanaCheckin: nil si «ahora» cae entre (inicio − tolerancia) y fin.
func ventanaCheckin(inicio, fin time.Time, tolMin int, ahora time.Time) *motivoCheckin {
	desde := inicio.Add(-time.Duration(tolMin) * time.Minute)
	if ahora.Before(desde) {
		il := inicio.In(P.Lima)
		falta := desde.Sub(ahora)
		texto := fmt.Sprintf("Aún no empieza: la reserva es el %s de %s a %s.", il.Format("02/01"), il.Format("15:04"), fin.In(P.Lima).Format("15:04"))
		if falta < 2*time.Hour {
			texto = fmt.Sprintf("Aún no empieza: podrá ingresar en %d min (desde las %s).", int(falta.Minutes())+1, desde.In(P.Lima).Format("15:04"))
		}
		return &motivoCheckin{"ANTES_DE_FRANJA", texto}
	}
	if !ahora.Before(fin) {
		return &motivoCheckin{"FRANJA_TERMINADA", "La franja reservada ya terminó (" + fin.In(P.Lima).Format("02/01 15:04") + ")."}
	}
	return nil
}

// normalizarCodigoQR: acepta el contenido del QR, el token suelto o el código R-0000 tecleado.
func normalizarCodigoQR(txt string) (token, codigo string) {
	t := strings.TrimSpace(txt)
	if i := strings.Index(strings.ToUpper(t), prefijoQR); i >= 0 {
		t = strings.TrimSpace(t[i+len(prefijoQR):])
	}
	if reCodigoReserva.MatchString(t) {
		return "", strings.ToUpper(t)
	}
	return strings.ToLower(t), ""
}

type reservaCheckin struct {
	ID, UnidadID, AreaID int64
	Codigo, Estado       string
	Inicio, Fin          time.Time
	Tolerancia           int
	CheckinValido        *bool
	CheckinEn            *time.Time
	Datos                map[string]any
}

const sqlReservaCheckin = `SELECT rv.id, rv.unidad_id, rc.area_id, rv.codigo, rv.estado, rv.inicio, rv.fin, ar.checkin_tolerancia_min, rv.checkin_valido, rv.checkin_en,
	u.codigo AS unidad, rc.nombre AS recurso, ar.nombre AS area, rv.titulo, rv.asistentes, rv.total_cts, rv.garantia_cts, rv.limpieza_cts,
	rv.checkin_motivo, rv.checkin_forzado_motivo,
	COALESCE((SELECT p.nombre FROM unidad_persona up JOIN persona p ON p.id=up.persona_id WHERE up.unidad_id=u.id AND up.hasta IS NULL ORDER BY (up.rol='propietario') DESC, up.id LIMIT 1), '') AS titular
	FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id JOIN area ar ON ar.id=rc.area_id JOIN unidad u ON u.id=rv.unidad_id
	WHERE rv.edificio_id=$1 AND `

func (s *Server) buscarReservaCheckin(ctx context.Context, q db.Q, eid int64, txt string, bloquear bool) (*reservaCheckin, error) {
	token, codigo := normalizarCodigoQR(txt)
	if token == "" && codigo == "" {
		return nil, P.Validacion("Escanea el QR o escribe el código de la reserva.").Campo("codigo", "Obligatorio.")
	}
	cond, arg := "rv.qr_token=$2", token
	if codigo != "" {
		cond, arg = "rv.codigo=$2", codigo
	}
	sql := sqlReservaCheckin + cond
	if bloquear {
		sql += " FOR UPDATE OF rv"
	}
	filas, err := db.Filas(ctx, q, sql, eid, arg)
	if err != nil {
		return nil, err
	}
	if len(filas) == 0 {
		return nil, P.NoEncontrado("esa reserva (el QR no es de este edificio o no existe)")
	}
	f := filas[0]
	rc := &reservaCheckin{ID: f["id"].(int64), UnidadID: f["unidad_id"].(int64), AreaID: f["area_id"].(int64), Codigo: f["codigo"].(string),
		Estado: f["estado"].(string), Inicio: f["inicio"].(time.Time), Fin: f["fin"].(time.Time), Datos: f}
	if v, ok := f["checkin_tolerancia_min"].(int32); ok {
		rc.Tolerancia = int(v)
	}
	if v, ok := f["checkin_valido"].(bool); ok {
		rc.CheckinValido = &v
	}
	if v, ok := f["checkin_en"].(time.Time); ok {
		rc.CheckinEn = &v
	}
	return rc, nil
}

// evaluarCheckin: lista de motivos que impiden entrar (vacía = puede entrar), la deuda y si solo la deuda lo impide.
func (s *Server) evaluarCheckin(ctx context.Context, q db.Q, rc *reservaCheckin, ahora time.Time) ([]motivoCheckin, int64) {
	motivos := []motivoCheckin{}
	if rc.CheckinValido != nil && *rc.CheckinValido {
		motivos = append(motivos, motivoCheckin{"YA_INGRESO", "Esta reserva ya registró su ingreso a las " + rc.CheckinEn.In(P.Lima).Format("15:04") + "."})
	}
	switch rc.Estado {
	case "confirmada":
	case "pendiente_pago":
		motivos = append(motivos, motivoCheckin{"PAGO_PENDIENTE", "La reserva aún no está pagada: no hay entrada válida."})
	default:
		motivos = append(motivos, motivoCheckin{"RESERVA_NO_ACTIVA", "La reserva está " + strings.ReplaceAll(rc.Estado, "_", " ") + "."})
	}
	if m := ventanaCheckin(rc.Inicio, rc.Fin, rc.Tolerancia, ahora); m != nil {
		motivos = append(motivos, *m)
	}
	var deuda int64
	_ = q.QueryRow(ctx, `SELECT deuda_vencida_cts($1)`, rc.UnidadID).Scan(&deuda)
	if deuda > 0 && !s.unidadHabilitada(ctx, q, rc.UnidadID, rc.AreaID) {
		motivos = append(motivos, motivoCheckin{"MOROSO", "La unidad tiene una deuda vencida de " + P.Soles(deuda) + ". Para entrar hay que estar al día."})
	}
	return motivos, deuda
}

// soloMoroso: el único impedimento es la deuda (lo único que el administrador puede forzar).
func soloMoroso(ms []motivoCheckin) bool {
	return len(ms) == 1 && ms[0].Codigo == "MOROSO"
}

func (s *Server) respuestaCheckin(e *Edificio, rc *reservaCheckin, motivos []motivoCheckin, deuda int64) map[string]any {
	d := rc.Datos
	return map[string]any{
		"valido": len(motivos) == 0, "motivos": motivos, "deuda_cts": deuda, "al_dia": deuda == 0,
		"puede_forzar": soloMoroso(motivos) && e.Puede("reservas.administrar"),
		"reserva": map[string]any{"id": rc.ID, "codigo": rc.Codigo, "estado": rc.Estado, "inicio": rc.Inicio.UTC(), "fin": rc.Fin.UTC(),
			"unidad": d["unidad"], "titular": d["titular"], "recurso": d["recurso"], "area": d["area"], "titulo": d["titulo"], "asistentes": d["asistentes"],
			"checkin_en": rc.CheckinEn, "checkin_valido": rc.CheckinValido},
	}
}

// verCheckin: GET /checkin?codigo= → vista previa del escaneo, sin registrar nada.
func (s *Server) verCheckin(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	rc, err := s.buscarReservaCheckin(r.Context(), s.DB, e.ID, r.URL.Query().Get("codigo"), false)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	motivos, deuda := s.evaluarCheckin(r.Context(), s.DB, rc, time.Now())
	P.JSON(w, http.StatusOK, s.respuestaCheckin(e, rc, motivos, deuda))
}

// registrarCheckin: POST /checkin {codigo, forzar_motivo?}. Registra el intento (válido o rechazado) y responde 200
// con «valido» y los motivos; 409 CHECKIN_REPETIDO si ya entró. Solo la deuda se puede forzar, y solo el administrador.
func (s *Server) registrarCheckin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Codigo       string `json:"codigo"`
		ForzarMotivo string `json:"forzar_motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	forzar := strings.TrimSpace(in.ForzarMotivo)
	if forzar != "" && !e.Puede("reservas.administrar") {
		P.Fallo(w, r, P.Prohibido("SIN_PERMISO", "Solo la administración puede autorizar el ingreso de una unidad con deuda."))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	rc, err := s.buscarReservaCheckin(ctx, tx, e.ID, in.Codigo, true)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if rc.CheckinValido != nil && *rc.CheckinValido {
		P.Fallo(w, r, P.Conflicto("CHECKIN_REPETIDO", "Esta reserva ya registró su ingreso a las "+rc.CheckinEn.In(P.Lima).Format("15:04")+".").Con("checkin_en", rc.CheckinEn))
		return
	}
	motivos, deuda := s.evaluarCheckin(ctx, tx, rc, time.Now())
	valido := len(motivos) == 0
	forzado := false
	if !valido && forzar != "" {
		if !soloMoroso(motivos) {
			P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "NO_FORZABLE", "Solo se autoriza el ingreso con deuda; lo demás no se puede saltar.").Con("motivos", motivos))
			return
		}
		valido, forzado = true, true
	}
	var forz, motivoTxt *string
	if forzado {
		forz = &forzar
	}
	if !valido {
		t := make([]string, len(motivos))
		for i, m := range motivos {
			t[i] = m.Texto
		}
		j := strings.Join(t, " ")
		motivoTxt = &j
	}
	uid := ses(r).UsuarioID
	if _, err := tx.Exec(ctx, `UPDATE reserva SET checkin_en=now(), checkin_por=$2, checkin_valido=$3, checkin_motivo=$4, checkin_forzado_motivo=$5 WHERE id=$1`,
		rc.ID, uid, valido, motivoTxt, forz); err != nil {
		P.Fallo(w, r, traducirCheckin(err))
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO reserva_checkin (reserva_id, usuario_id, valido, motivos, deuda_cts, forzado_motivo) VALUES ($1,$2,$3,$4,$5,$6)`,
		rc.ID, uid, valido, motivos, deuda, forz); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, traducirCheckin(err))
		return
	}
	if forzado {
		s.auditarCambio(ctx, s.DB, r, "reservas", "forzar_checkin", "reserva", rc.ID, map[string]any{"motivos": motivos, "deuda_cts": deuda}, map[string]any{"motivo": forzar})
	}
	ahora := time.Now()
	rc.CheckinEn = &ahora
	rc.CheckinValido = &valido
	res := s.respuestaCheckin(e, rc, motivos, deuda)
	res["valido"] = valido
	res["forzado"] = forzado
	res["puede_forzar"] = !valido && soloMoroso(motivos) && e.Puede("reservas.administrar")
	if valido {
		res["mensaje"] = "Ingreso registrado: " + rc.Codigo + "."
	}
	P.JSON(w, http.StatusOK, res)
}

// traducirCheckin: los RAISE de reserva_valida_checkin (0028) como errores del API.
func traducirCheckin(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "EDR01":
			return P.Conflicto("RESERVA_NO_CONFIRMADA", "La reserva no está confirmada.")
		case "EDR02":
			return P.Err(http.StatusUnprocessableEntity, "FUERA_DE_FRANJA", "El ingreso no cae en la franja reservada.")
		case "EDR03":
			return P.Prohibido("MOROSO", "La unidad tiene deuda vencida: para entrar hay que estar al día.")
		case "EDR04":
			return P.Conflicto("CHECKIN_REPETIDO", "Esta reserva ya registró su ingreso.")
		}
	}
	return P.Traducir(err)
}

// checkinHoy: GET /checkin/hoy → reservas del día (Lima) con su ingreso y si la unidad está habilitada hoy.
func (s *Server) checkinHoy(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	hoy := time.Now().In(P.Lima)
	if t, err := parseFecha(r.URL.Query().Get("dia")); err == nil {
		hoy = t
	}
	ini := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, P.Lima)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT rv.id, rv.codigo, rv.estado, rv.inicio, rv.fin, rv.titulo, rv.asistentes,
			u.codigo AS unidad, rc.nombre AS recurso, ar.nombre AS area,
			rv.checkin_en, rv.checkin_valido, rv.checkin_motivo, rv.checkin_forzado_motivo,
			NOT reserva_unidad_habilitada(rv.unidad_id, rc.area_id) AS moroso
		FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id JOIN area ar ON ar.id=rc.area_id JOIN unidad u ON u.id=rv.unidad_id
		WHERE rv.edificio_id=$1 AND rv.inicio >= $2 AND rv.inicio < $3 AND rv.estado IN ('confirmada','pendiente_pago','no_show')
		ORDER BY rv.inicio, rc.nombre`, e.ID, ini, ini.AddDate(0, 0, 1))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"dia": ini.Format("2006-01-02"), "datos": filas, "total": len(filas)})
}

// qrReserva: GET /reservas/{rid}/qr → la entrada de la reserva (contenido + PNG en data URI).
// El residente solo ve las de su unidad; la entrada existe solo si la reserva está confirmada.
func (s *Server) qrReserva(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	f, err := db.Fila(r.Context(), s.DB, `SELECT rv.id, rv.codigo, rv.estado, rv.unidad_id, rv.qr_token, rv.inicio, rv.fin, rv.titulo,
			rc.nombre AS recurso, ar.nombre AS area, u.codigo AS unidad, rv.checkin_en, rv.checkin_valido
		FROM reserva rv JOIN recurso rc ON rc.id=rv.recurso_id JOIN area ar ON ar.id=rc.area_id JOIN unidad u ON u.id=rv.unidad_id
		WHERE rv.id=$1 AND rv.edificio_id=$2`, rid, e.ID)
	if err != nil || (e.SoloLoSuyo() && !e.EsSuya(f["unidad_id"].(int64))) {
		P.Fallo(w, r, P.NoEncontrado("la reserva"))
		return
	}
	if f["estado"] != "confirmada" {
		P.Fallo(w, r, P.Conflicto("RESERVA_NO_CONFIRMADA", "La entrada con QR aparece cuando la reserva está confirmada (pagada o con cargo al recibo)."))
		return
	}
	contenido := prefijoQR + f["qr_token"].(string)
	png, err := qrcode.Encode(contenido, qrcode.Medium, 320)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	delete(f, "qr_token")
	f["contenido"] = contenido
	f["png"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	P.JSON(w, http.StatusOK, f)
}
