package app

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// G3 · Registro de visitas e identificación QR. El residente (o la portería) autoriza la visita y
// el API genera un código aleatorio; el QR solo lleva ese código, así que no se puede falsificar ni
// sirve en otro edificio. El conserje lo lee y el API decide; cada lectura queda en «acceso».

// prefijoQRVisita marca el contenido del QR; al validar se acepta con o sin prefijo.
const prefijoQRVisita = "EDISYS:V:"

// Motivos de rechazo al validar una visita.
const (
	RechazoNoExiste   = "NO_EXISTE"
	RechazoAnulada    = "ANULADA"
	RechazoFinalizada = "FINALIZADA"
	RechazoYaDentro   = "YA_DENTRO"
	RechazoAunNo      = "AUN_NO_VIGENTE"
	RechazoVencida    = "VENCIDA"
	RechazoSinUsos    = "SIN_USOS"
)

var textoRechazo = map[string]string{
	RechazoNoExiste:   "El código no corresponde a ninguna visita de este edificio.",
	RechazoAnulada:    "La visita fue anulada por quien la autorizó.",
	RechazoFinalizada: "La visita ya terminó.",
	RechazoYaDentro:   "El visitante figura dentro; registra su salida antes de volver a ingresar.",
	RechazoAunNo:      "La autorización todavía no empieza.",
	RechazoVencida:    "La autorización ya venció.",
	RechazoSinUsos:    "La autorización ya se usó todas las veces permitidas.",
}

// MotivoRechazoVisita dice por qué no puede entrar una visita ("" = puede). Regla pura, sin base.
func MotivoRechazoVisita(estado string, desde, hasta, ahora time.Time, usos, usosMax int) string {
	switch estado {
	case "anulada":
		return RechazoAnulada
	case "finalizada":
		return RechazoFinalizada
	case "en_curso":
		return RechazoYaDentro
	}
	if ahora.Before(desde) {
		return RechazoAunNo
	}
	if !ahora.Before(hasta) {
		return RechazoVencida
	}
	if usos >= usosMax {
		return RechazoSinUsos
	}
	return ""
}

// NormalizarCodigoVisita quita el prefijo del QR, espacios y guiones, y pasa a mayúsculas.
func NormalizarCodigoVisita(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, prefijoQRVisita)
	return strings.NewReplacer(" ", "", "-", "").Replace(s)
}

// nuevoCodigoVisita: 80 bits aleatorios en base32 (16 caracteres).
func nuevoCodigoVisita() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

const sqlVisita = `SELECT v.id, v.unidad_id, u.codigo AS unidad, v.visitante, v.documento, v.vehiculo_placa, v.motivo,
	v.valido_desde, v.valido_hasta, v.usos, v.usos_max, v.estado, v.codigo_qr, v.creado_en, au.nombre AS autorizado_por,
	(SELECT max(a.creado_en) FROM acceso a WHERE a.visita_id=v.id AND a.tipo='entrada' AND a.resultado='permitido') AS ultima_entrada
	FROM visita v JOIN unidad u ON u.id=v.unidad_id LEFT JOIN usuario au ON au.id=v.autorizado_por`

// listarVisitas: GET /visitas?estado=&dia=AAAA-MM-DD (residentes: solo las de su unidad)
func (s *Server) listarVisitas(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	v := r.URL.Query()
	cond := []string{"v.edificio_id=$1"}
	args := []any{e.ID}
	add := func(c string, a any) {
		args = append(args, a)
		cond = append(cond, strings.ReplaceAll(c, "?", fmt.Sprintf("$%d", len(args))))
	}
	if e.SoloLoSuyo() {
		add("v.unidad_id = ANY(?)", e.Unidades)
	}
	if x := v.Get("estado"); x != "" {
		add("v.estado = ANY(?)", strings.Split(x, ","))
	}
	if t, err := time.ParseInLocation("2006-01-02", v.Get("dia"), P.Lima); err == nil {
		// Vigentes ese día: la ventana se cruza con el día.
		add("v.valido_desde < ?", t.AddDate(0, 0, 1))
		add("v.valido_hasta > ?", t)
	}
	filas, err := db.Filas(r.Context(), s.DB, sqlVisita+` WHERE `+strings.Join(cond, " AND ")+` ORDER BY v.valido_desde DESC, v.id DESC LIMIT 300`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearVisita: POST /visitas {unidad_id, visitante, documento, vehiculo_placa, motivo, valido_desde, valido_hasta,
// usos_max, ingresar_ahora}. Sin fechas: desde ahora hasta el fin del día. ingresar_ahora (solo portería)
// registra la entrada en el acto: es el visitante que llega sin autorización previa.
func (s *Server) crearVisita(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	var in struct {
		UnidadID      int64  `json:"unidad_id"`
		Visitante     string `json:"visitante"`
		Documento     string `json:"documento"`
		Placa         string `json:"vehiculo_placa"`
		Motivo        string `json:"motivo"`
		Desde         string `json:"valido_desde"`
		Hasta         string `json:"valido_hasta"`
		UsosMax       int    `json:"usos_max"`
		IngresarAhora bool   `json:"ingresar_ahora"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.UnidadID == 0 && e.SoloLoSuyo() && len(e.Unidades) > 0 {
		in.UnidadID = e.Unidades[0]
	}
	ev := P.Validacion("Revisa la visita.")
	in.Visitante = strings.TrimSpace(in.Visitante)
	if in.Visitante == "" {
		ev.Campo("visitante", "Escribe el nombre del visitante.")
	}
	if e.SoloLoSuyo() && !e.EsSuya(in.UnidadID) {
		ev.Campo("unidad_id", "Solo puedes autorizar visitas a tu unidad.")
	} else {
		var ok bool
		_ = s.DB.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, in.UnidadID, e.ID).Scan(&ok)
		if !ok {
			ev.Campo("unidad_id", "Elige una unidad del edificio.")
		}
	}
	ahora := time.Now()
	desde := ahora
	if in.Desde != "" {
		t, err := parseFecha(in.Desde)
		if err != nil {
			ev.Campo("valido_desde", "Fecha y hora inválidas.")
		} else {
			desde = t
		}
	}
	y, m, d := desde.In(P.Lima).Date()
	hasta := time.Date(y, m, d, 23, 59, 59, 0, P.Lima)
	if in.Hasta != "" {
		t, err := parseFecha(in.Hasta)
		if err != nil {
			ev.Campo("valido_hasta", "Fecha y hora inválidas.")
		} else {
			if len(in.Hasta) == len("2006-01-02") {
				t = t.Add(24*time.Hour - time.Second) // un día suelto vale hasta su último segundo
			}
			hasta = t
		}
	}
	if !hasta.After(desde) {
		ev.Campo("valido_hasta", "Debe ser posterior al inicio.")
	}
	if hasta.Sub(desde) > 90*24*time.Hour {
		ev.Campo("valido_hasta", "Una autorización dura como máximo 90 días.")
	}
	if in.UsosMax == 0 {
		in.UsosMax = 1
	}
	if in.UsosMax < 1 || in.UsosMax > 100 {
		ev.Campo("usos_max", "Entre 1 y 100 ingresos.")
	}
	if in.IngresarAhora && !e.Puede("visitas.validar") {
		ev.Campo("ingresar_ahora", "Solo la portería registra ingresos.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	codigo := nuevoCodigoVisita()
	estado, usos := "autorizada", 0
	if in.IngresarAhora {
		estado, usos = "en_curso", 1
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO visita (edificio_id, unidad_id, visitante, documento, vehiculo_placa, motivo, autorizado_por,
			valido_desde, valido_hasta, usos_max, usos, codigo_qr, estado)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		e.ID, in.UnidadID, in.Visitante, strings.TrimSpace(in.Documento), strings.ToUpper(strings.TrimSpace(in.Placa)), strings.TrimSpace(in.Motivo),
		se.UsuarioID, desde, hasta, in.UsosMax, usos, codigo, estado).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.IngresarAhora {
		if _, err := tx.Exec(ctx, `INSERT INTO acceso (edificio_id, visita_id, tipo, resultado, motivo, codigo_leido, registrado_por)
			VALUES ($1,$2,'entrada','permitido','Ingreso registrado en portería',$3,$4)`, e.ID, id, codigo, se.UsuarioID); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "codigo_qr": codigo, "contenido_qr": prefijoQRVisita + codigo, "estado": estado,
		"valido_desde": desde, "valido_hasta": hasta})
}

// visitaVisible carga una visita del edificio respetando el alcance del residente.
func (s *Server) visitaVisible(r *http.Request, id int64) (map[string]any, error) {
	e := edf(r)
	f, err := db.Fila(r.Context(), s.DB, sqlVisita+` WHERE v.id=$1 AND v.edificio_id=$2`, id, e.ID)
	if err != nil {
		return nil, P.NoEncontrado("esa visita")
	}
	if e.SoloLoSuyo() && !e.EsSuya(f["unidad_id"].(int64)) {
		return nil, P.NoEncontrado("esa visita")
	}
	return f, nil
}

// qrVisita: GET /visitas/{id}/qr → la matriz del QR (filas de «0»/«1») para pintarla en la pantalla.
// Se genera aquí mismo, sin servicios externos.
func (s *Server) qrVisita(w http.ResponseWriter, r *http.Request) {
	f, err := s.visitaVisible(r, idURL(r, "id"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	contenido := prefijoQRVisita + f["codigo_qr"].(string)
	q, err := qrcode.New(contenido, qrcode.Medium)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	q.DisableBorder = true
	bm := q.Bitmap()
	filas := make([]string, len(bm))
	for i, fila := range bm {
		var b strings.Builder
		for _, negro := range fila {
			if negro {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		filas[i] = b.String()
	}
	P.JSON(w, http.StatusOK, map[string]any{"visita": f, "contenido": contenido, "matriz": filas, "lado": len(filas)})
}

// anularVisita: POST /visitas/{id}/anular — solo mientras nadie haya entrado con ella.
func (s *Server) anularVisita(w http.ResponseWriter, r *http.Request) {
	id := idURL(r, "id")
	f, err := s.visitaVisible(r, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if f["estado"] != "autorizada" {
		P.Fallo(w, r, P.Conflicto("VISITA_NO_ANULABLE", "Solo se anula una visita autorizada que no está dentro."))
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE visita SET estado='anulada' WHERE id=$1 AND estado='autorizada'`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.Conflicto("VISITA_NO_ANULABLE", "La visita cambió mientras tanto; recarga."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "anulada"})
}

// validarVisita: POST /visitas/validar {codigo} — lectura del QR en portería. Responde 200 con
// {valido, motivo, mensaje, visita}; el rechazo también queda en la bitácora de accesos.
func (s *Server) validarVisita(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	var in struct {
		Codigo string `json:"codigo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	codigo := NormalizarCodigoVisita(in.Codigo)
	if codigo == "" {
		P.Fallo(w, r, P.Validacion("Escanea o escribe el código.").Campo("codigo", "Obligatorio."))
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id int64
	var estado string
	var desde, hasta time.Time
	var usos, usosMax int
	motivo := ""
	if err := tx.QueryRow(ctx, `SELECT id, estado, valido_desde, valido_hasta, usos, usos_max FROM visita
		WHERE codigo_qr=$1 AND edificio_id=$2 FOR UPDATE`, codigo, e.ID).Scan(&id, &estado, &desde, &hasta, &usos, &usosMax); err != nil {
		motivo = RechazoNoExiste
	} else {
		motivo = MotivoRechazoVisita(estado, desde, hasta, time.Now(), usos, usosMax)
	}
	var visitaID *int64
	if id > 0 {
		visitaID = &id
	}
	resultado := "permitido"
	if motivo != "" {
		resultado = "rechazado"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO acceso (edificio_id, visita_id, tipo, resultado, motivo, codigo_leido, registrado_por)
		VALUES ($1,$2,'entrada',$3,$4,$5,$6)`, e.ID, visitaID, resultado, motivo, codigo, se.UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if motivo == "" {
		if _, err := tx.Exec(ctx, `UPDATE visita SET usos=usos+1, estado='en_curso' WHERE id=$1`, id); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	out := map[string]any{"valido": motivo == "", "motivo": motivo, "mensaje": textoRechazo[motivo]}
	if motivo == "" {
		out["mensaje"] = "Puede ingresar."
	}
	if id > 0 {
		if f, err := db.Fila(ctx, s.DB, sqlVisita+` WHERE v.id=$1`, id); err == nil {
			delete(f, "codigo_qr")
			out["visita"] = f
		}
	}
	P.JSON(w, http.StatusOK, out)
}

// salidaVisita: POST /visitas/{id}/salida — registra la salida. Si le quedan ingresos y sigue vigente,
// la visita vuelve a «autorizada»; si no, termina.
func (s *Server) salidaVisita(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var estado string
	var hasta time.Time
	var usos, usosMax int
	if err := tx.QueryRow(ctx, `SELECT estado, valido_hasta, usos, usos_max FROM visita WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).
		Scan(&estado, &hasta, &usos, &usosMax); err != nil {
		P.Fallo(w, r, P.NoEncontrado("esa visita"))
		return
	}
	if estado != "en_curso" {
		P.Fallo(w, r, P.Conflicto("VISITA_NO_DENTRO", "Esa visita no figura dentro del edificio."))
		return
	}
	nuevo := "finalizada"
	if usos < usosMax && time.Now().Before(hasta) {
		nuevo = "autorizada"
	}
	if _, err := tx.Exec(ctx, `UPDATE visita SET estado=$2 WHERE id=$1`, id, nuevo); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO acceso (edificio_id, visita_id, tipo, resultado, registrado_por) VALUES ($1,$2,'salida','permitido',$3)`,
		e.ID, id, ses(r).UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": nuevo})
}

// listarAccesos: GET /accesos?limite= — bitácora de portería, lo último primero.
func (s *Server) listarAccesos(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	lim, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	if lim <= 0 || lim > 500 {
		lim = 100
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT a.id, a.tipo, a.resultado, a.motivo, a.codigo_leido, a.creado_en, a.visita_id,
			v.visitante, u.codigo AS unidad, us.nombre AS registrado_por
		FROM acceso a LEFT JOIN visita v ON v.id=a.visita_id LEFT JOIN unidad u ON u.id=v.unidad_id LEFT JOIN usuario us ON us.id=a.registrado_por
		WHERE a.edificio_id=$1 ORDER BY a.creado_en DESC, a.id DESC LIMIT $2`, e.ID, lim)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if m, _ := f["motivo"].(string); textoRechazo[m] != "" {
			f["motivo_texto"] = textoRechazo[m]
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}
