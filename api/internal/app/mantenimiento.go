package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	M "edisys/api/internal/mantenimiento"
	P "edisys/api/internal/plataforma"
)

var categorias = map[string]bool{"gasfiteria": true, "electricidad": true, "ascensores": true, "bombas": true, "limpieza": true, "seguridad": true,
	"areas_comunes": true, "estructura": true, "jardineria": true, "otros": true}

const sqlIncidencia = `SELECT i.id, i.codigo, i.titulo, i.descripcion, i.ubicacion, i.categoria, i.criticidad, i.estado, i.origen,
	i.monto_presupuesto_cts, i.costo_real_cts, i.proveedor, i.revision_junta, i.motivo, i.creado_en, i.actualizado_en, i.terminado_en,
	i.unidad_id, u.codigo AS unidad, i.reportado_por, rp.nombre AS reportado_por_nombre, i.responsable_id, rs.nombre AS responsable_nombre,
	(SELECT count(*) FROM voto v WHERE v.incidencia_id=i.id AND v.voto='aprueba') AS votos_a_favor,
	(SELECT count(*) FROM voto v WHERE v.incidencia_id=i.id AND v.voto='rechaza') AS votos_en_contra,
	(SELECT count(*) FROM incidencia_evidencia ev WHERE ev.incidencia_id=i.id) AS evidencias,
	(SELECT ev.archivo_id FROM incidencia_evidencia ev WHERE ev.incidencia_id=i.id AND ev.tipo='reporte' ORDER BY ev.id LIMIT 1) AS foto_id
	FROM incidencia i LEFT JOIN unidad u ON u.id=i.unidad_id LEFT JOIN usuario rp ON rp.id=i.reportado_por LEFT JOIN usuario rs ON rs.id=i.responsable_id`

// listarIncidencias: GET /mantenimiento/incidencias (y /edificios/{eid}/incidencias, /trabajos)
// filtros: estado (lista con comas), criticidad, categoria, responsable_id, q, desde, hasta, mes.
func (s *Server) listarIncidencias(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	where, args := condIncidencias(e, r.URL.Query(), ses(r).UsuarioID)
	ctx := r.Context()
	filas, err := db.Filas(ctx, s.DB, sqlIncidencia+` WHERE `+where+`
		ORDER BY CASE i.criticidad WHEN 'critica' THEN 0 WHEN 'media' THEN 1 WHEN 'baja' THEN 2 ELSE 3 END, i.numero DESC LIMIT 500`, args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var umbral int64
	var miembros int
	var modo string
	_ = s.DB.QueryRow(ctx, `SELECT umbral_aprobacion_cts, modo_aprobacion, (SELECT count(*) FROM junta_miembro WHERE edificio_id=$1 AND activo) FROM edificio WHERE id=$1`, e.ID).Scan(&umbral, &modo, &miembros)
	conteos := map[string]int{}
	for _, est := range M.Estados {
		conteos[est] = 0
	}
	for _, f := range filas {
		conteos[f["estado"].(string)]++
		decorarIncidencia(s, f, umbral, modo, miembros)
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "conteos": conteos, "columnas": M.Estados})
}

func decorarIncidencia(s *Server, f map[string]any, umbral int64, modo string, miembros int) {
	if id, ok := f["foto_id"].(int64); ok {
		f["foto_url"] = s.Firma.URL(id)
	} else {
		f["foto_url"] = nil
	}
	delete(f, "foto_id")
	monto, _ := f["monto_presupuesto_cts"].(int64)
	requiere := monto > umbral
	f["requiere_junta"] = requiere
	_, nec := M.Votacion(modo, miembros, 0, 0, "")
	f["votos_necesarios"] = nec
	f["transiciones"] = M.Transiciones[f["estado"].(string)]
}

// tablero: GET /trabajos?estado=&criticidad=&mes= (mismo listado con conteos por estado).
func (s *Server) tablero(w http.ResponseWriter, r *http.Request) { s.listarIncidencias(w, r) }

// incidenciaVisible carga una incidencia del edificio respetando el alcance del rol.
func (s *Server) incidenciaVisible(ctx context.Context, q db.Q, e *Edificio, uid, id int64) (map[string]any, error) {
	f, err := db.Fila(ctx, q, sqlIncidencia+` WHERE i.id=$1 AND i.edificio_id=$2`, id, e.ID)
	if err != nil {
		return nil, P.NoEncontrado("esa incidencia")
	}
	if e.Rol == "tecnico" {
		if rid, _ := f["responsable_id"].(int64); rid != uid {
			return nil, P.NoEncontrado("esa incidencia")
		}
	}
	if e.SoloLoSuyo() {
		rp, _ := f["reportado_por"].(int64)
		un, _ := f["unidad_id"].(int64)
		estado := f["estado"].(string)
		publico := estado == "presupuestado" || estado == "aprobado" || estado == "en_ejecucion" || estado == "terminado" || estado == "rechazado"
		if rp != uid && !e.EsSuya(un) && !publico {
			return nil, P.NoEncontrado("esa incidencia")
		}
	}
	return f, nil
}

// verTrabajo: detalle con línea de tiempo, evidencias, presupuesto, votos y lo que puedo hacer.
func (s *Server) verTrabajo(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "tid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	f, err := s.incidenciaVisible(ctx, s.DB, e, se.UsuarioID, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var umbral int64
	var modo string
	var miembros int
	var esMiembro, esPresidente bool
	_ = s.DB.QueryRow(ctx, `SELECT umbral_aprobacion_cts, modo_aprobacion, (SELECT count(*) FROM junta_miembro WHERE edificio_id=$1 AND activo),
		EXISTS (SELECT 1 FROM junta_miembro WHERE edificio_id=$1 AND usuario_id=$2 AND activo),
		EXISTS (SELECT 1 FROM junta_miembro WHERE edificio_id=$1 AND usuario_id=$2 AND activo AND presidente)
		FROM edificio WHERE id=$1`, e.ID, se.UsuarioID).Scan(&umbral, &modo, &miembros, &esMiembro, &esPresidente)
	decorarIncidencia(s, f, umbral, modo, miembros)
	tiempo, err := db.Filas(ctx, s.DB, `SELECT ev.id, ev.estado_desde, ev.estado_hasta, ev.nota, ev.creado_en, us.nombre AS usuario
		FROM incidencia_evento ev LEFT JOIN usuario us ON us.id=ev.usuario_id WHERE ev.incidencia_id=$1 ORDER BY ev.creado_en, ev.id`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	evid, err := db.Filas(ctx, s.DB, `SELECT ev.id, ev.tipo, ev.archivo_id, a.nombre, a.tipo_mime, ev.creado_en, us.nombre AS usuario
		FROM incidencia_evidencia ev JOIN archivo a ON a.id=ev.archivo_id LEFT JOIN usuario us ON us.id=ev.usuario_id WHERE ev.incidencia_id=$1 ORDER BY ev.id`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, x := range evid {
		x["url"] = s.Firma.URL(x["archivo_id"].(int64))
	}
	votos, err := db.Filas(ctx, s.DB, `SELECT v.voto, v.comentario, v.creado_en, us.nombre AS miembro, jm.presidente
		FROM voto v JOIN usuario us ON us.id=v.usuario_id LEFT JOIN junta_miembro jm ON jm.usuario_id=v.usuario_id AND jm.edificio_id=$2
		WHERE v.incidencia_id=$1 ORDER BY v.creado_en`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var yaVote bool
	_ = s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM voto WHERE incidencia_id=$1 AND usuario_id=$2)`, id, se.UsuarioID).Scan(&yaVote)
	aFavor, _ := f["votos_a_favor"].(int64)
	enContra, _ := f["votos_en_contra"].(int64)
	res, nec := M.Votacion(modo, miembros, int(aFavor), int(enContra), "")
	estado := f["estado"].(string)
	puedo := []string{}
	for _, h := range M.Transiciones[estado] {
		if e.Puede(M.PermisoPara(h)) {
			puedo = append(puedo, h)
		}
	}
	if estado == "presupuestado" && f["requiere_junta"] == true && esMiembro && !yaVote && e.Puede("trabajos.votar") {
		puedo = append(puedo, "votar")
	}
	f["linea_tiempo"] = tiempo
	f["evidencias"] = evid
	f["presupuestos"] = []map[string]any{}
	if m, ok := f["monto_presupuesto_cts"].(int64); ok {
		p := map[string]any{"proveedor": f["proveedor"], "monto_cts": m, "elegido": true, "pdf_url": nil}
		var pdfID *int64
		_ = s.DB.QueryRow(ctx, `SELECT presupuesto_archivo_id FROM incidencia WHERE id=$1`, id).Scan(&pdfID)
		p["pdf_url"] = s.url(pdfID)
		f["presupuestos"] = []map[string]any{p}
	}
	f["votos"] = votos
	f["votacion"] = map[string]any{"modo": modo, "miembros": miembros, "necesarios": nec, "a_favor": aFavor, "en_contra": enContra,
		"resultado": res, "umbral_cts": umbral, "texto": fmt.Sprintf("%d de %d votos", aFavor, nec)}
	f["puedo"] = puedo
	f["ya_vote"] = yaVote
	P.JSON(w, http.StatusOK, f)
}

// crearIncidencia registra un reporte nuevo (app, chatbot o admin) y su primer evento.
func (s *Server) crearIncidencia(ctx context.Context, tx pgx.Tx, eid int64, uid *int64, unidad *int64, titulo, desc, ubic, categoria, origen string) (int64, string, error) {
	if _, err := tx.Exec(ctx, `SELECT id FROM edificio WHERE id=$1 FOR UPDATE`, eid); err != nil {
		return 0, "", err
	}
	var num int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(numero),0)+1 FROM incidencia WHERE edificio_id=$1`, eid).Scan(&num); err != nil {
		return 0, "", err
	}
	codigo := fmt.Sprintf("INC-%03d", num)
	if titulo == "" {
		titulo = desc
		if len([]rune(titulo)) > 60 {
			titulo = string([]rune(titulo)[:57]) + "…"
		}
	}
	if !categorias[categoria] {
		categoria = "otros"
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO incidencia (edificio_id, numero, codigo, titulo, descripcion, ubicacion, categoria, origen, reportado_por, unidad_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, eid, num, codigo, titulo, desc, ubic, categoria, origen, uid, unidad).Scan(&id); err != nil {
		return 0, "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO incidencia_evento (incidencia_id, estado_desde, estado_hasta, usuario_id, nota) VALUES ($1,NULL,'reportado',$2,$3)`,
		id, uid, "Reportado desde "+origen); err != nil {
		return 0, "", err
	}
	return id, codigo, nil
}

// reportarIncidencia: POST /incidencias (multipart {descripcion, ubicacion, titulo?, categoria?, unidad_id?, fotos[]}) → 201 {id, codigo}.
func (s *Server) reportarIncidencia(w http.ResponseWriter, r *http.Request) {
	fotos, err := archivosDeForm(r, "fotos", "fotos[]", "foto")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if !esMultipart(r) || len(fotos) == 0 {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "FOTO_OBLIGATORIA", "Adjunta al menos una foto del problema.").Campo("fotos", "Obligatoria."))
		return
	}
	desc, ubic := campo(r, "descripcion"), campo(r, "ubicacion")
	if len(desc) < 3 {
		P.Fallo(w, r, P.Validacion("Describe brevemente el problema.").Campo("descripcion", "Obligatoria."))
		return
	}
	e := edf(r)
	se := ses(r)
	ctx := r.Context()
	var unidad *int64
	if v, err := strconv.ParseInt(campo(r, "unidad_id"), 10, 64); err == nil {
		unidad = &v
	} else if len(e.Unidades) > 0 {
		unidad = &e.Unidades[0]
	}
	if unidad != nil && e.SoloLoSuyo() && !e.EsSuya(*unidad) {
		P.Fallo(w, r, P.NoEncontrado("la unidad"))
		return
	}
	if e.Rol == "inquilino" && unidad != nil {
		var puede bool
		_ = s.DB.QueryRow(ctx, `SELECT COALESCE((permisos_inquilino->>'reportar')::boolean, false) FROM unidad WHERE id=$1`, *unidad).Scan(&puede)
		if !puede {
			P.Fallo(w, r, P.Prohibido("INQUILINO_SIN_PERMISO", "El propietario no habilitó los reportes para el inquilino."))
			return
		}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	id, codigo, err := s.crearIncidencia(ctx, tx, e.ID, &se.UsuarioID, unidad, campo(r, "titulo"), desc, ubic, campo(r, "categoria"), "app")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range fotos {
		aid, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, f)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if _, err := tx.Exec(ctx, `INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id) VALUES ($1,$2,'reporte',$3)`, id, aid, se.UsuarioID); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "codigo": codigo, "estado": "reportado"})
}

// opcionesCambio: datos opcionales al mover una incidencia.
type opcionesCambio struct {
	Motivo        string `json:"motivo"`
	Nota          string `json:"nota"`
	Criticidad    string `json:"criticidad"`
	Categoria     string `json:"categoria"`
	MontoCts      *int64 `json:"monto_presupuesto_cts"`
	Proveedor     string `json:"proveedor"`
	Diagnostico   string `json:"diagnostico"`
	RubroID       *int64 `json:"rubro_id"`
	CostoRealCts  *int64 `json:"costo_real_cts"`
	ResponsableID *int64 `json:"responsable_id"`
	evidencias    []Subido
	tipoEvidencia string
	comprobante   *Subido
	presupuesto   *Subido
}

// Transicionar mueve una incidencia validando la máquina de estados, el permiso de cada paso,
// el umbral de aprobación y las evidencias de cierre. Al terminar, el costo real entra como egreso.
func (s *Server) Transicionar(ctx context.Context, e *Edificio, uid, id int64, hacia string, o opcionesCambio) (map[string]any, error) {
	if !M.EstadoValido(hacia) {
		return nil, P.Validacion("Estado desconocido: "+hacia).Campo("estado", "Uno de: "+strings.Join(M.Estados, ", "))
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	inc, err := s.incidenciaVisible(ctx, tx, e, uid, id)
	if err != nil {
		return nil, err
	}
	desde := inc["estado"].(string)
	if !M.PuedePasar(desde, hacia) {
		return nil, P.Conflicto("TRANSICION_INVALIDA", fmt.Sprintf("No se puede pasar de «%s» a «%s».", desde, hacia)).
			Con("desde", desde).Con("hacia", hacia).Con("permitidas", M.Transiciones[desde])
	}
	perm := M.PermisoPara(hacia)
	if !e.Puede(perm) {
		return nil, P.Prohibido("SIN_PERMISO", "Tu rol no puede pasar trabajos a «"+hacia+"».").Con("permiso", perm)
	}
	var umbral int64
	var modo string
	if err := tx.QueryRow(ctx, `SELECT umbral_aprobacion_cts, modo_aprobacion FROM edificio WHERE id=$1`, e.ID).Scan(&umbral, &modo); err != nil {
		return nil, err
	}
	sets := []string{"estado=$2"}
	args := []any{id, hacia}
	set := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	if o.Criticidad != "" {
		if o.Criticidad != "critica" && o.Criticidad != "media" && o.Criticidad != "baja" {
			return nil, P.Validacion("Criticidad inválida.").Campo("criticidad", "critica, media o baja.")
		}
		set("criticidad", o.Criticidad)
	}
	if o.Categoria != "" && categorias[o.Categoria] {
		set("categoria", o.Categoria)
	}
	if o.ResponsableID != nil {
		set("responsable_id", *o.ResponsableID)
	}
	monto, _ := inc["monto_presupuesto_cts"].(int64)
	switch hacia {
	case "validado":
		if o.Criticidad == "" && inc["criticidad"] == nil {
			return nil, P.Err(http.StatusUnprocessableEntity, "CRITICIDAD_OBLIGATORIA", "Indica la criticidad al validar (crítica, media o baja).").Campo("criticidad", "Obligatoria.")
		}
	case "descartado":
		if strings.TrimSpace(o.Motivo) == "" {
			return nil, P.Validacion("Explica por qué se descarta (p. ej. «igual a INC-012»).").Campo("motivo", "Obligatorio.")
		}
		set("motivo", o.Motivo)
	case "presupuestado":
		if o.MontoCts != nil {
			if *o.MontoCts <= 0 {
				return nil, P.Validacion("El presupuesto debe ser mayor que cero.").Campo("monto_presupuesto_cts", "Mayor que cero.")
			}
			monto = *o.MontoCts
			set("monto_presupuesto_cts", monto)
		}
		if monto <= 0 {
			return nil, P.Err(http.StatusUnprocessableEntity, "PRESUPUESTO_OBLIGATORIO", "Registra el monto del presupuesto.").Campo("monto_presupuesto_cts", "Obligatorio.")
		}
		if o.Proveedor != "" {
			set("proveedor", o.Proveedor)
		}
		if o.Diagnostico != "" {
			set("diagnostico", o.Diagnostico)
		}
		if o.RubroID != nil {
			set("rubro_id", *o.RubroID)
		}
		// Un presupuesto nuevo reinicia la votación.
		if _, err := tx.Exec(ctx, `DELETE FROM voto WHERE incidencia_id=$1`, id); err != nil {
			return nil, err
		}
	case "aprobado", "rechazado":
		if monto > umbral && !o.decisionJunta() {
			var esPresidente bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM junta_miembro WHERE edificio_id=$1 AND usuario_id=$2 AND activo AND presidente)`, e.ID, uid).Scan(&esPresidente)
			if !(modo == "presidente" && esPresidente) {
				return nil, P.Conflicto("REQUIERE_VOTO_JUNTA", fmt.Sprintf("El presupuesto (%s) pasa el umbral de %s: decide la junta con sus votos.", P.Soles(monto), P.Soles(umbral))).
					Con("umbral_cts", umbral).Con("monto_cts", monto)
			}
		}
		if hacia == "rechazado" && o.Motivo != "" {
			set("motivo", o.Motivo)
		}
	case "terminado":
		for _, ev := range o.evidencias {
			_ = ev
		}
		var cierres int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM incidencia_evidencia WHERE incidencia_id=$1 AND tipo IN ('cierre','comprobante')`, id).Scan(&cierres); err != nil {
			return nil, err
		}
		if cierres == 0 && len(o.evidencias) == 0 && o.comprobante == nil {
			return nil, P.Err(http.StatusUnprocessableEntity, "EVIDENCIA_CIERRE_OBLIGATORIA", "Sube al menos una foto del trabajo terminado.").Campo("evidencias", "Obligatoria.")
		}
	}
	// Evidencias y documentos que llegan con el cambio.
	tipoEv := o.tipoEvidencia
	if tipoEv == "" {
		tipoEv = "avance"
		if hacia == "terminado" {
			tipoEv = "cierre"
		}
	}
	for _, ev := range o.evidencias {
		aid, err := s.guardarArchivo(ctx, tx, e.ID, &uid, ev)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id) VALUES ($1,$2,$3,$4)`, id, aid, tipoEv, uid); err != nil {
			return nil, err
		}
	}
	var comprobante *int64
	if o.comprobante != nil {
		aid, err := s.guardarArchivo(ctx, tx, e.ID, &uid, *o.comprobante)
		if err != nil {
			return nil, err
		}
		comprobante = &aid
		set("comprobante_id", aid)
		_, _ = tx.Exec(ctx, `INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id) VALUES ($1,$2,'comprobante',$3)`, id, aid, uid)
	}
	if o.presupuesto != nil {
		aid, err := s.guardarArchivo(ctx, tx, e.ID, &uid, *o.presupuesto)
		if err != nil {
			return nil, err
		}
		set("presupuesto_archivo_id", aid)
		_, _ = tx.Exec(ctx, `INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id) VALUES ($1,$2,'presupuesto',$3)`, id, aid, uid)
	}
	var egresoID *int64
	if hacia == "terminado" {
		costo := monto
		if o.CostoRealCts != nil && *o.CostoRealCts > 0 {
			costo = *o.CostoRealCts
		}
		set("costo_real_cts", costo)
		if monto > 0 && (costo*10 > monto*11 || costo*10 < monto*9) {
			set("revision_junta", true)
		}
		if costo > 0 {
			if comprobante == nil {
				_ = tx.QueryRow(ctx, `SELECT comprobante_id FROM incidencia WHERE id=$1`, id).Scan(&comprobante)
			}
			var rubro int64
			if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT rubro_id FROM incidencia WHERE id=$1),
				(SELECT id FROM rubro WHERE edificio_id=$2 AND slug='correctivo'), (SELECT id FROM rubro WHERE edificio_id=$2 AND slug='mantenimiento'),
				(SELECT min(id) FROM rubro WHERE edificio_id=$2))`, id, e.ID).Scan(&rubro); err != nil {
				return nil, P.Err(http.StatusUnprocessableEntity, "SIN_RUBRO", "El edificio no tiene rubros para registrar el egreso.")
			}
			hoy := time.Now().In(P.Lima)
			tipoDoc := "foto"
			var eid int64
			if err := tx.QueryRow(ctx, `INSERT INTO egreso (edificio_id, periodo, rubro_id, descripcion, monto_cts, fecha, documento_id, tipo_documento, origen, incidencia_id, registrado_por)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'trabajo',$9,$10) RETURNING id`, e.ID, hoy.Format("2006-01"), rubro,
				fmt.Sprintf("%s · %s", inc["codigo"], inc["titulo"]), costo, hoy.Format("2006-01-02"), comprobante, tipoDoc, id, uid).Scan(&eid); err != nil {
				return nil, err
			}
			egresoID = &eid
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE incidencia SET `+strings.Join(sets, ", ")+` WHERE id=$1`, args...); err != nil {
		return nil, P.Traducir(err)
	}
	nota := strings.TrimSpace(o.Nota)
	if nota == "" {
		nota = strings.TrimSpace(o.Motivo)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO incidencia_evento (incidencia_id, estado_desde, estado_hasta, usuario_id, nota) VALUES ($1,$2,$3,$4,$5)`,
		id, desde, hacia, uid, nota); err != nil {
		return nil, err
	}
	// Cada cambio de estado avisa al que reportó (WhatsApp por la bandeja; en simulado solo se registra).
	var tel, nombre string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(u.telefono,''), (SELECT pe.celular FROM persona pe WHERE pe.usuario_id=u.id AND pe.celular<>'' LIMIT 1), ''), split_part(u.nombre,' ',1)
		FROM incidencia i JOIN usuario u ON u.id=i.reportado_por WHERE i.id=$1`, id).Scan(&tel, &nombre)
	if tel != "" {
		_, _ = s.encolar(ctx, tx, e.ID, nil, tel, "incidencia_actualizada", map[string]string{"nombre": nombre, "codigo": inc["codigo"].(string),
			"titulo": inc["titulo"].(string), "estado": etiquetaIncidencia(hacia)}, "sistema", &uid)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, P.Traducir(err)
	}
	go s.despacharPendientes(context.Background())
	out, err := db.Fila(ctx, s.DB, sqlIncidencia+` WHERE i.id=$1`, id)
	if err != nil {
		return nil, err
	}
	out["transiciones"] = M.Transiciones[hacia]
	out["egreso_id"] = egresoID
	delete(out, "foto_id")
	return out, nil
}

// decisionJunta marca que el cambio viene del resultado de la votación.
func (o opcionesCambio) decisionJunta() bool { return o.tipoEvidencia == "__junta__" }

func etiquetaIncidencia(e string) string {
	m := map[string]string{"reportado": "Reportado", "validado": "Validado por la administración", "presupuestado": "Con presupuesto, esperando aprobación",
		"aprobado": "Aprobado", "en_ejecucion": "En ejecución", "terminado": "Terminado", "rechazado": "No aprobado por la junta", "descartado": "Descartado"}
	return m[e]
}

// cambiarEstadoIncidencia: PATCH /mantenimiento/incidencias/{id}/estado {estado, …} (kanban).
func (s *Server) cambiarEstadoIncidencia(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "tid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Estado string `json:"estado"`
		opcionesCambio
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	out, err := s.Transicionar(r.Context(), edf(r), ses(r).UsuarioID, id, in.Estado, in.opcionesCambio)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, out)
}

// validarIncidencia: POST /incidencias/{id}/validar {accion: aceptar|descartar|unir, criticidad, unir_con?, motivo?}.
func (s *Server) validarIncidencia(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "tid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Accion  string `json:"accion"`
		UnirCon string `json:"unir_con"`
		opcionesCambio
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	hacia := "validado"
	switch in.Accion {
	case "aceptar", "":
	case "descartar":
		hacia = "descartado"
	case "unir":
		hacia = "descartado"
		if in.Motivo == "" {
			in.Motivo = "Igual a " + in.UnirCon
		}
	default:
		P.Fallo(w, r, P.Validacion("Acción inválida.").Campo("accion", "aceptar, descartar o unir."))
		return
	}
	out, err := s.Transicionar(r.Context(), edf(r), ses(r).UsuarioID, id, hacia, in.opcionesCambio)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, out)
}

// informeTrabajo: POST /trabajos/{id}/informe (multipart {diagnostico, proveedor, monto_cts, pdf, rubro_id}) → presupuestado.
func (s *Server) informeTrabajo(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "tid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var o opcionesCambio
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		o.Diagnostico, o.Proveedor, o.Nota = campo(r, "diagnostico"), campo(r, "proveedor"), campo(r, "nota")
		if v, err := strconv.ParseInt(campo(r, "monto_cts"), 10, 64); err == nil {
			o.MontoCts = &v
		}
		if v, err := strconv.ParseInt(campo(r, "rubro_id"), 10, 64); err == nil {
			o.RubroID = &v
		}
		pdfs, err := archivosDeForm(r, "pdf", "presupuesto")
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if len(pdfs) > 0 {
			o.presupuesto = &pdfs[0]
		}
	} else {
		var in struct {
			Diagnostico string `json:"diagnostico"`
			Proveedor   string `json:"proveedor"`
			MontoCts    *int64 `json:"monto_cts"`
			RubroID     *int64 `json:"rubro_id"`
		}
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
		o.Diagnostico, o.Proveedor, o.MontoCts, o.RubroID = in.Diagnostico, in.Proveedor, in.MontoCts, in.RubroID
	}
	if o.Nota == "" {
		o.Nota = o.Diagnostico
	}
	out, err := s.Transicionar(r.Context(), edf(r), ses(r).UsuarioID, id, "presupuestado", o)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, out)
}

// votarTrabajo: POST /trabajos/{id}/votos {voto: aprueba|rechaza, comentario} → 201 {resultado} · 409 YA_VOTASTE.
func (s *Server) votarTrabajo(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "tid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Voto       string `json:"voto"`
		Comentario string `json:"comentario"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Voto != "aprueba" && in.Voto != "rechaza" {
		P.Fallo(w, r, P.Validacion("Voto inválido.").Campo("voto", "aprueba o rechaza."))
		return
	}
	e := edf(r)
	uid := ses(r).UsuarioID
	ctx := r.Context()
	var estado, modo string
	var monto *int64
	var umbral int64
	var miembros int
	var esMiembro, esPresidente bool
	err = s.DB.QueryRow(ctx, `SELECT i.estado, i.monto_presupuesto_cts, e.umbral_aprobacion_cts, e.modo_aprobacion,
		(SELECT count(*) FROM junta_miembro WHERE edificio_id=e.id AND activo),
		EXISTS (SELECT 1 FROM junta_miembro WHERE edificio_id=e.id AND usuario_id=$3 AND activo),
		EXISTS (SELECT 1 FROM junta_miembro WHERE edificio_id=e.id AND usuario_id=$3 AND activo AND presidente)
		FROM incidencia i JOIN edificio e ON e.id=i.edificio_id WHERE i.id=$1 AND i.edificio_id=$2`, id, e.ID, uid).
		Scan(&estado, &monto, &umbral, &modo, &miembros, &esMiembro, &esPresidente)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("ese trabajo"))
		return
	}
	if !esMiembro {
		P.Fallo(w, r, P.Prohibido("NO_ES_MIEMBRO_JUNTA", "Solo votan los miembros activos de la junta."))
		return
	}
	if estado != "presupuestado" {
		P.Fallo(w, r, P.Conflicto("NO_EN_VOTACION", "Ese trabajo no está esperando aprobación (está "+estado+")."))
		return
	}
	if monto == nil || *monto <= umbral {
		P.Fallo(w, r, P.Conflicto("NO_REQUIERE_VOTO", "El presupuesto está dentro del umbral: lo aprueba la administración."))
		return
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO voto (incidencia_id, usuario_id, voto, comentario) VALUES ($1,$2,$3,$4)`, id, uid, in.Voto, in.Comentario); err != nil {
		P.Fallo(w, r, err)
		return
	}
	var aFavor, enContra int
	_ = s.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE voto='aprueba'), count(*) FILTER (WHERE voto='rechaza') FROM voto WHERE incidencia_id=$1`, id).Scan(&aFavor, &enContra)
	votoPres := ""
	if esPresidente {
		votoPres = in.Voto
	}
	res, nec := M.Votacion(modo, miembros, aFavor, enContra, votoPres)
	if res == "aprobado" || res == "rechazado" {
		o := opcionesCambio{tipoEvidencia: "__junta__", Nota: fmt.Sprintf("Decisión de la junta: %d a favor, %d en contra", aFavor, enContra)}
		if res == "rechazado" {
			o.Motivo = "Pendiente no aprobado por la junta"
		}
		if _, err := s.Transicionar(ctx, e, uid, id, res, o); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	P.JSON(w, http.StatusCreated, map[string]any{"resultado": res, "a_favor": aFavor, "en_contra": enContra, "necesarios": nec, "miembros": miembros,
		"texto": fmt.Sprintf("%d de %d votos", aFavor, nec)})
}

// avanceTrabajo: POST /trabajos/{id}/avance (multipart {estado: en_ejecucion|terminado, nota, evidencias[], costo_real_cts?, comprobante?}).
func (s *Server) avanceTrabajo(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "tid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var o opcionesCambio
	var estado string
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			P.Fallo(w, r, err)
			return
		}
		estado, o.Nota = campo(r, "estado"), campo(r, "nota")
		if v, err := strconv.ParseInt(campo(r, "costo_real_cts"), 10, 64); err == nil {
			o.CostoRealCts = &v
		}
		if o.evidencias, err = archivosDeForm(r, "evidencias", "evidencias[]", "fotos", "foto"); err != nil {
			P.Fallo(w, r, err)
			return
		}
		comp, err := archivosDeForm(r, "comprobante")
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		if len(comp) > 0 {
			o.comprobante = &comp[0]
		}
	} else {
		var in struct {
			Estado       string `json:"estado"`
			Nota         string `json:"nota"`
			CostoRealCts *int64 `json:"costo_real_cts"`
		}
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
		estado, o.Nota, o.CostoRealCts = in.Estado, in.Nota, in.CostoRealCts
	}
	e := edf(r)
	uid := ses(r).UsuarioID
	ctx := r.Context()
	var actual string
	if err := s.DB.QueryRow(ctx, `SELECT estado FROM incidencia WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&actual); err != nil {
		P.Fallo(w, r, P.NoEncontrado("ese trabajo"))
		return
	}
	if estado == "" || estado == actual {
		// Solo avance con evidencias, sin cambio de estado.
		if len(o.evidencias) == 0 && o.Nota == "" {
			P.Fallo(w, r, P.Validacion("Agrega una nota o una foto de avance."))
			return
		}
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		defer tx.Rollback(ctx)
		for _, ev := range o.evidencias {
			aid, err := s.guardarArchivo(ctx, tx, e.ID, &uid, ev)
			if err != nil {
				P.Fallo(w, r, err)
				return
			}
			_, _ = tx.Exec(ctx, `INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id) VALUES ($1,$2,'avance',$3)`, id, aid, uid)
		}
		_, _ = tx.Exec(ctx, `INSERT INTO incidencia_evento (incidencia_id, estado_desde, estado_hasta, usuario_id, nota) VALUES ($1,$2,$2,$3,$4)`, id, actual, uid, o.Nota)
		if err := tx.Commit(ctx); err != nil {
			P.Fallo(w, r, err)
			return
		}
		P.JSON(w, http.StatusCreated, map[string]any{"id": id, "estado": actual, "evidencias": len(o.evidencias)})
		return
	}
	out, err := s.Transicionar(ctx, e, uid, id, estado, o)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, out)
}

func (s *Server) verReglasAprobacion(w http.ResponseWriter, r *http.Request) {
	f, err := db.Fila(r.Context(), s.DB, `SELECT modo_aprobacion AS modo, umbral_aprobacion_cts AS umbral_cts,
		(SELECT count(*) FROM junta_miembro WHERE edificio_id=$1 AND activo) AS miembros FROM edificio WHERE id=$1`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	_, nec := M.Votacion(f["modo"].(string), int(f["miembros"].(int64)), 0, 0, "")
	f["necesarios"] = nec
	P.JSON(w, http.StatusOK, f)
}

func (s *Server) editarReglasAprobacion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Modo      string `json:"modo"`
		UmbralCts *int64 `json:"umbral_cts"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Modo != "" && in.Modo != "presidente" && in.Modo != "mayoria" {
		P.Fallo(w, r, P.Validacion("Modo inválido.").Campo("modo", "presidente o mayoria."))
		return
	}
	if _, err := s.DB.Exec(r.Context(), `UPDATE edificio SET modo_aprobacion=COALESCE(NULLIF($2,''),modo_aprobacion), umbral_aprobacion_cts=COALESCE($3,umbral_aprobacion_cts) WHERE id=$1`,
		edf(r).ID, in.Modo, in.UmbralCts); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.verReglasAprobacion(w, r)
}

var _ = errors.New
