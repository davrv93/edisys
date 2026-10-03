package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// Encuestas (bloque J1): la administración arma la encuesta en borrador, la abre, los
// residentes y la junta responden una sola vez, y los resultados se agregan por opción.
// Reglas: solo se edita en borrador; solo se responde abierta y antes de cierra_en;
// el propietario ve resultados recién al cierre (antes, solo quien tiene encuestas.resultados).

type preguntaEncuesta struct {
	Texto       string   `json:"texto"`
	Tipo        string   `json:"tipo"` // unica | multiple | texto
	Obligatoria *bool    `json:"obligatoria"`
	Opciones    []string `json:"opciones"`
}

type datosEncuesta struct {
	Titulo      string             `json:"titulo"`
	Descripcion string             `json:"descripcion"`
	Anonima     *bool              `json:"anonima"`
	CierraEn    string             `json:"cierra_en"` // AAAA-MM-DD (cierra al final del día en Lima) o RFC 3339; vacío = sin fecha
	Preguntas   []preguntaEncuesta `json:"preguntas"`
}

// validar normaliza y revisa la encuesta; devuelve la fecha de cierre (nil si no hay).
func (in *datosEncuesta) validar() (*time.Time, error) {
	ev := P.Validacion("Revisa la encuesta.")
	in.Titulo = strings.TrimSpace(in.Titulo)
	in.Descripcion = strings.TrimSpace(in.Descripcion)
	if in.Titulo == "" {
		ev.Campo("titulo", "Escribe el título.")
	}
	var cierra *time.Time
	if c := strings.TrimSpace(in.CierraEn); c != "" {
		if t, err := time.ParseInLocation("2006-01-02", c, P.Lima); err == nil {
			t = t.Add(24*time.Hour - time.Second)
			cierra = &t
		} else if t, err := time.Parse(time.RFC3339, c); err == nil {
			cierra = &t
		} else {
			ev.Campo("cierra_en", "Usa el formato AAAA-MM-DD.")
		}
	}
	if len(in.Preguntas) == 0 {
		ev.Campo("preguntas", "Agrega al menos una pregunta.")
	}
	for i := range in.Preguntas {
		p := &in.Preguntas[i]
		p.Texto = strings.TrimSpace(p.Texto)
		if p.Texto == "" {
			ev.Campo("preguntas", "Cada pregunta necesita su texto.")
		}
		switch p.Tipo {
		case "unica", "multiple":
			ops := make([]string, 0, len(p.Opciones))
			vistas := map[string]bool{}
			for _, o := range p.Opciones {
				o = strings.TrimSpace(o)
				if o == "" || vistas[strings.ToLower(o)] {
					continue
				}
				vistas[strings.ToLower(o)] = true
				ops = append(ops, o)
			}
			p.Opciones = ops
			if len(ops) < 2 {
				ev.Campo("preguntas", "Las preguntas de opción necesitan al menos dos opciones distintas.")
			}
		case "texto":
			p.Opciones = nil
		default:
			ev.Campo("preguntas", "Tipo de pregunta inválido (unica, multiple o texto).")
		}
	}
	if len(ev.Campos) > 0 {
		return nil, ev
	}
	return cierra, nil
}

// guardarPreguntas inserta preguntas y opciones de una encuesta (dentro de la transacción).
func guardarPreguntas(ctx context.Context, tx pgx.Tx, encuestaID int64, ps []preguntaEncuesta) error {
	for i, p := range ps {
		oblig := p.Obligatoria == nil || *p.Obligatoria
		var pid int64
		if err := tx.QueryRow(ctx, `INSERT INTO encuesta_pregunta (encuesta_id, orden, texto, tipo, obligatoria)
			VALUES ($1,$2,$3,$4,$5) RETURNING id`, encuestaID, i+1, p.Texto, p.Tipo, oblig).Scan(&pid); err != nil {
			return err
		}
		for j, o := range p.Opciones {
			if _, err := tx.Exec(ctx, `INSERT INTO encuesta_opcion (pregunta_id, orden, texto) VALUES ($1,$2,$3)`, pid, j+1, o); err != nil {
				return err
			}
		}
	}
	return nil
}

// listarEncuestas: GET /encuestas. Quien no administra no ve los borradores.
func (s *Server) listarEncuestas(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	filas, err := db.Filas(r.Context(), s.DB, `SELECT en.id, en.titulo, en.descripcion, en.estado, en.anonima,
			to_char(en.cierra_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS cierra_en,
			(en.estado='abierta' AND (en.cierra_en IS NULL OR en.cierra_en > now())) AS acepta_respuestas,
			(SELECT count(*) FROM encuesta_pregunta p WHERE p.encuesta_id=en.id) AS preguntas,
			(SELECT count(*) FROM encuesta_respuesta x WHERE x.encuesta_id=en.id) AS respuestas,
			EXISTS(SELECT 1 FROM encuesta_respuesta x WHERE x.encuesta_id=en.id AND x.usuario_id=$3) AS respondida,
			to_char(en.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS fecha
		FROM encuesta en
		WHERE en.edificio_id=$1 AND ($2 OR en.estado <> 'borrador')
		ORDER BY (en.estado='abierta') DESC, en.creado_en DESC`, e.ID, e.Puede("encuestas.administrar"), ses(r).UsuarioID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// cargarEncuesta trae la cabecera y sus preguntas con opciones. El borrador solo lo ve quien administra.
func (s *Server) cargarEncuesta(ctx context.Context, e *Edificio, id, uid int64) (map[string]any, error) {
	enc, err := db.Fila(ctx, s.DB, `SELECT en.id, en.titulo, en.descripcion, en.estado, en.anonima,
			to_char(en.cierra_en AT TIME ZONE 'America/Lima','YYYY-MM-DD') AS cierra_en,
			(en.estado='abierta' AND (en.cierra_en IS NULL OR en.cierra_en > now())) AS acepta_respuestas,
			(SELECT count(*) FROM encuesta_respuesta x WHERE x.encuesta_id=en.id) AS respuestas,
			EXISTS(SELECT 1 FROM encuesta_respuesta x WHERE x.encuesta_id=en.id AND x.usuario_id=$3) AS respondida
		FROM encuesta en WHERE en.id=$1 AND en.edificio_id=$2`, id, e.ID, uid)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && enc["estado"] == "borrador" && !e.Puede("encuestas.administrar")) {
		return nil, P.NoEncontrado("la encuesta")
	}
	if err != nil {
		return nil, err
	}
	preguntas, err := db.Filas(ctx, s.DB, `SELECT p.id, p.orden, p.texto, p.tipo, p.obligatoria,
			COALESCE((SELECT json_agg(json_build_object('id', o.id, 'texto', o.texto) ORDER BY o.orden)
				FROM encuesta_opcion o WHERE o.pregunta_id=p.id), '[]'::json) AS opciones
		FROM encuesta_pregunta p WHERE p.encuesta_id=$1 ORDER BY p.orden`, id)
	if err != nil {
		return nil, err
	}
	enc["preguntas"] = preguntas
	return enc, nil
}

// verEncuesta: GET /encuestas/{id}
func (s *Server) verEncuesta(w http.ResponseWriter, r *http.Request) {
	enc, err := s.cargarEncuesta(r.Context(), edf(r), idURL(r, "id"), ses(r).UsuarioID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, enc)
}

// crearEncuesta: POST /encuestas → queda en borrador.
func (s *Server) crearEncuesta(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	var in datosEncuesta
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	cierra, err := in.validar()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	anon := in.Anonima == nil || *in.Anonima
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO encuesta (edificio_id, titulo, descripcion, anonima, cierra_en, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, e.ID, in.Titulo, in.Descripcion, anon, cierra, ses(r).UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := guardarPreguntas(ctx, tx, id, in.Preguntas); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "estado": "borrador"})
}

// editarEncuesta: PUT /encuestas/{id} → reemplaza cabecera y preguntas; solo en borrador.
func (s *Server) editarEncuesta(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in datosEncuesta
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	cierra, err := in.validar()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	anon := in.Anonima == nil || *in.Anonima
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var estado string
	if err := tx.QueryRow(ctx, `SELECT estado FROM encuesta WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, id, e.ID).Scan(&estado); err != nil {
		P.Fallo(w, r, P.NoEncontrado("la encuesta"))
		return
	}
	if estado != "borrador" {
		P.Fallo(w, r, P.Conflicto("ENCUESTA_NO_EDITABLE", "Solo se edita una encuesta en borrador; ya tiene participación abierta o cerrada."))
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE encuesta SET titulo=$1, descripcion=$2, anonima=$3, cierra_en=$4 WHERE id=$5`,
		in.Titulo, in.Descripcion, anon, cierra, id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM encuesta_pregunta WHERE encuesta_id=$1`, id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := guardarPreguntas(ctx, tx, id, in.Preguntas); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// borrarEncuesta: DELETE /encuestas/{id} → solo borradores (una encuesta con votos no se borra).
func (s *Server) borrarEncuesta(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `DELETE FROM encuesta WHERE id=$1 AND edificio_id=$2 AND estado='borrador'`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, P.Conflicto("ENCUESTA_NO_BORRABLE", "Solo se borran encuestas en borrador. Ciérrala en su lugar."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// abrirEncuesta: POST /encuestas/{id}/abrir (borrador → abierta).
func (s *Server) abrirEncuesta(w http.ResponseWriter, r *http.Request) {
	s.cambiarEstadoEncuesta(w, r, "borrador", "abierta", `abierta_en=now()`)
}

// cerrarEncuesta: POST /encuestas/{id}/cerrar (abierta → cerrada).
func (s *Server) cerrarEncuesta(w http.ResponseWriter, r *http.Request) {
	s.cambiarEstadoEncuesta(w, r, "abierta", "cerrada", `cerrada_en=now()`)
}

func (s *Server) cambiarEstadoEncuesta(w http.ResponseWriter, r *http.Request, desde, hacia, sello string) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `UPDATE encuesta SET estado=$1, `+sello+` WHERE id=$2 AND edificio_id=$3 AND estado=$4`, hacia, id, e.ID, desde)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		var existe bool
		_ = s.DB.QueryRow(r.Context(), `SELECT true FROM encuesta WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&existe)
		if !existe {
			P.Fallo(w, r, P.NoEncontrado("la encuesta"))
			return
		}
		P.Fallo(w, r, P.Conflicto("TRANSICION_INVALIDA", "La encuesta no está en "+desde+"."))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": hacia})
}

// responderEncuesta: POST /encuestas/{id}/respuestas {respuestas:[{pregunta_id, opcion_ids, texto}]}
func (s *Server) responderEncuesta(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var in struct {
		Respuestas []struct {
			PreguntaID int64   `json:"pregunta_id"`
			OpcionIDs  []int64 `json:"opcion_ids"`
			Texto      string  `json:"texto"`
		} `json:"respuestas"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	// FOR SHARE: un cierre simultáneo espera a que esta respuesta termine (o al revés).
	var estado string
	var abiertaAhora bool
	err = tx.QueryRow(ctx, `SELECT estado, (cierra_en IS NULL OR cierra_en > now()) FROM encuesta
		WHERE id=$1 AND edificio_id=$2 FOR SHARE`, id, e.ID).Scan(&estado, &abiertaAhora)
	if err != nil || estado == "borrador" {
		P.Fallo(w, r, P.NoEncontrado("la encuesta"))
		return
	}
	if estado != "abierta" || !abiertaAhora {
		P.Fallo(w, r, P.Conflicto("ENCUESTA_CERRADA", "La encuesta ya no recibe respuestas."))
		return
	}
	// Preguntas de la encuesta con su tipo y sus opciones válidas.
	type preg struct {
		tipo     string
		oblig    bool
		opciones map[int64]bool
	}
	pregs := map[int64]*preg{}
	filas, err := tx.Query(ctx, `SELECT p.id, p.tipo, p.obligatoria, o.id FROM encuesta_pregunta p
		LEFT JOIN encuesta_opcion o ON o.pregunta_id=p.id WHERE p.encuesta_id=$1`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for filas.Next() {
		var pid int64
		var tipo string
		var oblig bool
		var oid *int64
		if err := filas.Scan(&pid, &tipo, &oblig, &oid); err != nil {
			filas.Close()
			P.Fallo(w, r, err)
			return
		}
		if pregs[pid] == nil {
			pregs[pid] = &preg{tipo: tipo, oblig: oblig, opciones: map[int64]bool{}}
		}
		if oid != nil {
			pregs[pid].opciones[*oid] = true
		}
	}
	filas.Close()

	ev := P.Validacion("Revisa tus respuestas.")
	type item struct {
		pregunta int64
		opcion   *int64
		texto    string
	}
	var items []item
	respondidas := map[int64]bool{}
	for _, rp := range in.Respuestas {
		p := pregs[rp.PreguntaID]
		if p == nil {
			ev.Campo("respuestas", "Una de las respuestas no corresponde a esta encuesta.")
			continue
		}
		if respondidas[rp.PreguntaID] {
			ev.Campo("respuestas", "Una pregunta viene respondida dos veces.")
			continue
		}
		switch p.tipo {
		case "texto":
			t := strings.TrimSpace(rp.Texto)
			if t == "" {
				continue
			}
			if len([]rune(t)) > 2000 {
				ev.Campo("respuestas", "Las respuestas de texto tienen hasta 2000 caracteres.")
				continue
			}
			items = append(items, item{pregunta: rp.PreguntaID, texto: t})
		default:
			if len(rp.OpcionIDs) == 0 {
				continue
			}
			if p.tipo == "unica" && len(rp.OpcionIDs) > 1 {
				ev.Campo("respuestas", "Una pregunta de opción única admite una sola opción.")
				continue
			}
			vistas := map[int64]bool{}
			for _, oid := range rp.OpcionIDs {
				if !p.opciones[oid] {
					ev.Campo("respuestas", "Una opción no pertenece a su pregunta.")
					continue
				}
				if vistas[oid] {
					continue
				}
				vistas[oid] = true
				o := oid
				items = append(items, item{pregunta: rp.PreguntaID, opcion: &o})
			}
		}
		respondidas[rp.PreguntaID] = true
	}
	for pid, p := range pregs {
		if p.oblig && !respondidas[pid] {
			ev.Campo("respuestas", "Responde todas las preguntas obligatorias.")
			break
		}
	}
	if len(items) == 0 {
		ev.Campo("respuestas", "No hay ninguna respuesta.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}

	var unidad *int64
	if len(e.Unidades) > 0 {
		unidad = &e.Unidades[0]
	}
	var rid int64
	err = tx.QueryRow(ctx, `INSERT INTO encuesta_respuesta (encuesta_id, usuario_id, unidad_id) VALUES ($1,$2,$3) RETURNING id`,
		id, ses(r).UsuarioID, unidad).Scan(&rid)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("YA_RESPONDISTE", "Ya respondiste esta encuesta."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	for _, it := range items {
		if _, err := tx.Exec(ctx, `INSERT INTO encuesta_respuesta_item (respuesta_id, encuesta_id, pregunta_id, opcion_id, texto)
			VALUES ($1,$2,$3,$4,$5)`, rid, id, it.pregunta, it.opcion, it.texto); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": rid})
}

// resultadosEncuesta: GET /encuestas/{id}/resultados. Conteo por opción y porcentaje sobre
// quienes respondieron esa pregunta. Con encuesta anónima, los textos salen sin autor.
func (s *Server) resultadosEncuesta(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	enc, err := s.cargarEncuesta(ctx, e, id, ses(r).UsuarioID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if !e.Puede("encuestas.resultados") && enc["estado"] != "cerrada" {
		P.Fallo(w, r, P.Prohibido("RESULTADOS_AL_CIERRE", "Los resultados se publican cuando la encuesta cierra.").Con("permiso", "encuestas.resultados"))
		return
	}
	votos := map[int64]int64{}
	filas, err := s.DB.Query(ctx, `SELECT opcion_id, count(*) FROM encuesta_respuesta_item
		WHERE encuesta_id=$1 AND opcion_id IS NOT NULL GROUP BY opcion_id`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for filas.Next() {
		var oid, n int64
		if err := filas.Scan(&oid, &n); err != nil {
			filas.Close()
			P.Fallo(w, r, err)
			return
		}
		votos[oid] = n
	}
	filas.Close()
	porPregunta := map[int64]int64{}
	filas, err = s.DB.Query(ctx, `SELECT pregunta_id, count(DISTINCT respuesta_id) FROM encuesta_respuesta_item
		WHERE encuesta_id=$1 GROUP BY pregunta_id`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for filas.Next() {
		var pid, n int64
		if err := filas.Scan(&pid, &n); err != nil {
			filas.Close()
			P.Fallo(w, r, err)
			return
		}
		porPregunta[pid] = n
	}
	filas.Close()

	anonima, _ := enc["anonima"].(bool)
	salida := []map[string]any{}
	for _, p := range enc["preguntas"].([]map[string]any) {
		pid := p["id"].(int64)
		total := porPregunta[pid]
		res := map[string]any{"id": pid, "texto": p["texto"], "tipo": p["tipo"], "respondieron": total}
		if p["tipo"] == "texto" {
			textos, err := db.Filas(ctx, s.DB, `SELECT i.texto, CASE WHEN $2 THEN NULL ELSE u.nombre END AS autor
				FROM encuesta_respuesta_item i JOIN encuesta_respuesta x ON x.id=i.respuesta_id JOIN usuario u ON u.id=x.usuario_id
				WHERE i.pregunta_id=$1 AND i.opcion_id IS NULL ORDER BY x.creado_en`, pid, anonima)
			if err != nil {
				P.Fallo(w, r, err)
				return
			}
			res["textos"] = textos
		} else {
			ops := []map[string]any{}
			for _, o := range opcionesDe(p["opciones"]) {
				n := votos[o.ID]
				ops = append(ops, map[string]any{"id": o.ID, "texto": o.Texto, "votos": n, "porcentaje": porcentaje(n, total)})
			}
			res["opciones"] = ops
		}
		salida = append(salida, res)
	}
	P.JSON(w, http.StatusOK, map[string]any{
		"id": id, "titulo": enc["titulo"], "estado": enc["estado"], "anonima": anonima,
		"respuestas": enc["respuestas"], "preguntas": salida,
	})
}

type opcionEncuesta struct {
	ID    int64
	Texto string
}

// opcionesDe lee el json_agg de opciones (llega como []any de mapas con números float64).
func opcionesDe(v any) []opcionEncuesta {
	lista, _ := v.([]any)
	out := make([]opcionEncuesta, 0, len(lista))
	for _, x := range lista {
		m, _ := x.(map[string]any)
		id, _ := m["id"].(float64)
		t, _ := m["texto"].(string)
		out = append(out, opcionEncuesta{ID: int64(id), Texto: t})
	}
	return out
}

// porcentaje entero redondeado (half-up) de n sobre total; 0 si nadie respondió.
func porcentaje(n, total int64) int64 {
	if total == 0 {
		return 0
	}
	return (n*200 + total) / (total * 2)
}
