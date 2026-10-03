package app

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// G5 · Registro de paquetes: la portería recibe, avisa a la unidad por WhatsApp (bandeja; simulado
// si el edificio no tiene Evolution) y entrega con el nombre de quien recoge y su firma o foto.

const sqlPaquete = `SELECT p.id, p.unidad_id, u.codigo AS unidad, p.remitente, p.descripcion, p.foto_id, p.estado,
	p.recibido_en, rp.nombre AS recibido_por, p.aviso_en, p.aviso_mensaje_id, m.estado AS aviso_estado,
	p.entregado_a, p.entregado_en, p.entrega_firma_id, ep.nombre AS entregado_por, p.motivo_devolucion
	FROM paquete p JOIN unidad u ON u.id=p.unidad_id LEFT JOIN usuario rp ON rp.id=p.recibido_por
	LEFT JOIN usuario ep ON ep.id=p.entregado_por LEFT JOIN whatsapp_mensaje m ON m.id=p.aviso_mensaje_id`

func (s *Server) decorarPaquete(f map[string]any) {
	for _, c := range []string{"foto", "entrega_firma"} {
		if id, ok := f[c+"_id"].(int64); ok {
			f[c+"_url"] = s.Firma.URL(id)
		} else {
			f[c+"_url"] = nil
		}
	}
}

// listarPaquetes: GET /paquetes?estado=&unidad_id= (residentes: solo los de su unidad)
func (s *Server) listarPaquetes(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	v := r.URL.Query()
	cond := []string{"p.edificio_id=$1"}
	args := []any{e.ID}
	add := func(c string, a any) {
		args = append(args, a)
		cond = append(cond, strings.ReplaceAll(c, "?", fmt.Sprintf("$%d", len(args))))
	}
	if e.SoloLoSuyo() {
		add("p.unidad_id = ANY(?)", e.Unidades)
	}
	if x := v.Get("estado"); x != "" {
		add("p.estado = ANY(?)", strings.Split(x, ","))
	}
	if n, err := strconv.ParseInt(v.Get("unidad_id"), 10, 64); err == nil {
		add("p.unidad_id = ?", n)
	}
	filas, err := db.Filas(r.Context(), s.DB, sqlPaquete+` WHERE `+strings.Join(cond, " AND ")+`
		ORDER BY p.estado <> 'recibido', p.recibido_en DESC LIMIT 300`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	pendientes := 0
	for _, f := range filas {
		s.decorarPaquete(f)
		if f["estado"] == "recibido" {
			pendientes++
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "pendientes": pendientes})
}

// avisarPaquete encola el aviso a la unidad y lo deja anotado en el paquete. Devuelve el id del
// mensaje (0 si la unidad no tiene celular registrado).
func (s *Server) avisarPaquete(ctx context.Context, q db.Q, eid, paqueteID int64) int64 {
	var unidad int64
	var desc, remitente string
	if err := q.QueryRow(ctx, `SELECT unidad_id, descripcion, remitente FROM paquete WHERE id=$1`, paqueteID).Scan(&unidad, &desc, &remitente); err != nil {
		return 0
	}
	nombre, cel, codigo := contactoUnidad(ctx, q, unidad)
	de := ""
	if remitente != "" {
		de = " de " + remitente
	}
	texto := fmt.Sprintf("Hola %s, llegó un paquete%s para el Dpto %s (%s). Puedes recogerlo en portería.", primerNombre(nombre), de, codigo, desc)
	mid := s.avisoSistema(ctx, q, eid, &unidad, cel, texto)
	if mid > 0 {
		_, _ = q.Exec(ctx, `UPDATE paquete SET aviso_mensaje_id=$2, aviso_en=now() WHERE id=$1`, paqueteID, mid)
	}
	return mid
}

// recibirPaquete: POST /paquetes (multipart {unidad_id, remitente, descripcion, foto?} o JSON).
func (s *Server) recibirPaquete(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	var in struct {
		UnidadID    int64  `json:"unidad_id"`
		Remitente   string `json:"remitente"`
		Descripcion string `json:"descripcion"`
	}
	var fotos []Subido
	if esMultipart(r) {
		var err error
		if fotos, err = archivosDeForm(r, "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
		in.UnidadID, _ = strconv.ParseInt(campo(r, "unidad_id"), 10, 64)
		in.Remitente, in.Descripcion = campo(r, "remitente"), campo(r, "descripcion")
	} else if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa el paquete.")
	in.Descripcion = strings.TrimSpace(in.Descripcion)
	if in.Descripcion == "" {
		ev.Campo("descripcion", "Describe el paquete (p. ej. «caja mediana Saga»).")
	}
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM unidad WHERE id=$1 AND edificio_id=$2`, in.UnidadID, e.ID).Scan(&ok)
	if !ok {
		ev.Campo("unidad_id", "Elige la unidad destinataria.")
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
	var fotoID *int64
	if len(fotos) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, fotos[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		fotoID = &id
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO paquete (edificio_id, unidad_id, remitente, descripcion, foto_id, recibido_por)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, e.ID, in.UnidadID, strings.TrimSpace(in.Remitente), in.Descripcion, fotoID, se.UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	mid := s.avisarPaquete(ctx, tx, e.ID, id)
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "estado": "recibido", "avisado": mid > 0, "aviso_mensaje_id": mid})
}

// entregarPaquete: POST /paquetes/{id}/entregar (multipart {entregado_a, firma|foto}) — sin firma o
// foto no hay entrega (también es regla en la base). Un paquete se entrega una sola vez.
func (s *Server) entregarPaquete(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	id := idURL(r, "id")
	firmas, err := archivosDeForm(r, "firma", "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	quien := campo(r, "entregado_a")
	ev := P.Validacion("Revisa la entrega.")
	if quien == "" {
		ev.Campo("entregado_a", "Escribe quién recoge el paquete.")
	}
	if len(firmas) == 0 {
		ev.Campo("firma", "Toma la firma o una foto de quien recoge.")
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
	var estado string
	if err := tx.QueryRow(ctx, `SELECT estado FROM paquete WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).Scan(&estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("ese paquete"))
		return
	}
	if estado != "recibido" {
		P.Fallo(w, r, P.Conflicto("PAQUETE_YA_"+strings.ToUpper(estado), "Ese paquete ya fue "+estado+"."))
		return
	}
	fid, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, firmas[0])
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE paquete SET estado='entregado', entregado_a=$2, entrega_firma_id=$3, entregado_por=$4, entregado_en=now() WHERE id=$1`,
		id, quien, fid, se.UsuarioID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "entregado"})
}

// devolverPaquete: POST /paquetes/{id}/devolver {motivo} — vuelve al courier sin entregarse.
func (s *Server) devolverPaquete(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Motivo) == "" {
		P.Fallo(w, r, P.Validacion("Indica por qué se devuelve.").Campo("motivo", "Obligatorio."))
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE paquete SET estado='devuelto', motivo_devolucion=$3 WHERE id=$1 AND edificio_id=$2 AND estado='recibido'`,
		id, e.ID, strings.TrimSpace(in.Motivo))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.Conflicto("PAQUETE_NO_PENDIENTE", "Ese paquete no está pendiente en portería."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "devuelto"})
}

// reavisarPaquete: POST /paquetes/{id}/reavisar — repite el aviso a la unidad.
func (s *Server) reavisarPaquete(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var estado string
	if err := s.DB.QueryRow(r.Context(), `SELECT estado FROM paquete WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("ese paquete"))
		return
	}
	if estado != "recibido" {
		P.Fallo(w, r, P.Conflicto("PAQUETE_NO_PENDIENTE", "Ese paquete ya no está en portería."))
		return
	}
	mid := s.avisarPaquete(r.Context(), s.DB, e.ID, id)
	if mid == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "UNIDAD_SIN_CELULAR", "La unidad no tiene un celular registrado para avisarle."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "aviso_mensaje_id": mid})
}
