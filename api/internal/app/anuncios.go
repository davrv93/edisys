package app

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/correo"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/whatsapp"
)

// Anuncios y comunicados (bloque E2). Un anuncio nace en borrador; al publicarlo se ve en el portal y,
// según sus canales, se encola UNA vez por destinatario en la bandeja de correo, WhatsApp o Telegram.
// Cada mensaje queda en anuncio_envio: esa es la evidencia de envío, con el estado vivo de su bandeja.

var canalesAnuncio = map[string]bool{"portal": true, "correo": true, "whatsapp": true, "telegram": true}

// normalizarCanales deja los canales válidos, sin repetir y siempre con «portal».
func normalizarCanales(in []string) ([]string, []string) {
	vistos := map[string]bool{"portal": true}
	out := []string{"portal"}
	var malos []string
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" || vistos[c] {
			continue
		}
		if !canalesAnuncio[c] {
			malos = append(malos, c)
			continue
		}
		vistos[c] = true
		out = append(out, c)
	}
	return out, malos
}

// entradaAnuncio es lo que llega al crear o editar (JSON o multipart con «archivo»).
type entradaAnuncio struct {
	Titulo  string   `json:"titulo"`
	Cuerpo  string   `json:"cuerpo"`
	Fecha   string   `json:"fecha"`
	Canales []string `json:"canales"`
}

func leerAnuncio(r *http.Request) (entradaAnuncio, []Subido, error) {
	var in entradaAnuncio
	if esMultipart(r) {
		if err := leerMultipart(r); err != nil {
			return in, nil, err
		}
		in.Titulo, in.Cuerpo, in.Fecha = campo(r, "titulo"), campo(r, "cuerpo"), campo(r, "fecha")
		if c := campo(r, "canales"); c != "" {
			in.Canales = strings.Split(c, ",")
		}
		adj, err := archivosDeForm(r, "archivo")
		return in, adj, err
	}
	err := P.Leer(r, &in)
	return in, nil, err
}

// validarAnuncio limpia la entrada y devuelve los canales normalizados y la fecha.
func validarAnuncio(in *entradaAnuncio) ([]string, *time.Time, *P.Error) {
	in.Titulo, in.Cuerpo = strings.TrimSpace(in.Titulo), strings.TrimSpace(in.Cuerpo)
	ev := P.Validacion("Revisa el anuncio.")
	if in.Titulo == "" {
		ev.Campo("titulo", "Escribe el título.")
	} else if len([]rune(in.Titulo)) > 160 {
		ev.Campo("titulo", "Máximo 160 caracteres.")
	}
	if in.Cuerpo == "" {
		ev.Campo("cuerpo", "Escribe el contenido.")
	}
	canales, malos := normalizarCanales(in.Canales)
	if len(malos) > 0 {
		ev.Campo("canales", "Canal no válido: "+strings.Join(malos, ", ")+". Usa portal, correo, whatsapp o telegram.")
	}
	fecha := fechaOpc(in.Fecha, ev, "fecha")
	if len(ev.Campos) > 0 {
		return nil, nil, ev
	}
	return canales, fecha, nil
}

// listarAnuncios: GET /anuncios?estado=&limite=. Quien administra ve todo con el resumen de envíos;
// el resto (portal) solo lo publicado.
func (s *Server) listarAnuncios(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	admin := e.Puede("anuncios.administrar")
	limite, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	if limite <= 0 || limite > 200 {
		limite = 100
	}
	filas, err := db.Filas(r.Context(), s.DB, `SELECT a.id, a.titulo, a.cuerpo, to_char(a.fecha,'YYYY-MM-DD') AS fecha, a.estado, a.canales, a.archivo_id,
			a.publicado_en, a.enviado_en, a.creado_en, us.nombre AS creado_por,
			(SELECT count(*) FROM anuncio_envio v WHERE v.anuncio_id=a.id) AS envios
		FROM anuncio a LEFT JOIN usuario us ON us.id=a.creado_por
		WHERE a.edificio_id=$1 AND ($2 OR a.estado='publicado') AND ($3='' OR a.estado=$3)
		ORDER BY (a.estado='borrador') DESC, a.fecha DESC, a.id DESC LIMIT $4`, e.ID, admin, r.URL.Query().Get("estado"), limite)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for _, f := range filas {
		if v, ok := f["archivo_id"].(int64); ok {
			f["archivo_url"] = s.Firma.URL(v)
		}
		if !admin {
			delete(f, "envios")
			delete(f, "creado_por")
		}
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearAnuncio: POST /anuncios (JSON o multipart con «archivo» opcional) → borrador.
func (s *Server) crearAnuncio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	in, adj, err := leerAnuncio(r)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	canales, fecha, ev := validarAnuncio(&in)
	if ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	se := ses(r)
	var archID *int64
	if len(adj) > 0 {
		id, err := s.guardarArchivo(ctx, tx, e.ID, &se.UsuarioID, adj[0])
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		archID = &id
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO anuncio (edificio_id, titulo, cuerpo, fecha, canales, archivo_id, creado_por)
		VALUES ($1,$2,$3,COALESCE($4::date,(now() AT TIME ZONE 'America/Lima')::date),$5,$6,$7) RETURNING id`,
		e.ID, in.Titulo, in.Cuerpo, fecha, canales, archID, se.UsuarioID).Scan(&id); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": id, "estado": "borrador", "canales": canales})
}

// editarAnuncio: PUT /anuncios/{id} {titulo, cuerpo, fecha, canales}. Solo en borrador: lo publicado ya se envió.
func (s *Server) editarAnuncio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	var in entradaAnuncio
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	canales, fecha, ev := validarAnuncio(&in)
	if ev != nil {
		P.Fallo(w, r, ev)
		return
	}
	ct, err := s.DB.Exec(r.Context(), `UPDATE anuncio SET titulo=$1, cuerpo=$2, fecha=COALESCE($3::date, fecha), canales=$4
		WHERE id=$5 AND edificio_id=$6 AND estado='borrador'`, in.Titulo, in.Cuerpo, fecha, canales, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, s.errorEstadoAnuncio(r.Context(), e.ID, id, "editar"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "canales": canales})
}

// borrarAnuncio: DELETE /anuncios/{id}. Solo un borrador; lo publicado se archiva (la evidencia se queda).
func (s *Server) borrarAnuncio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `DELETE FROM anuncio WHERE id=$1 AND edificio_id=$2 AND estado='borrador'`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, s.errorEstadoAnuncio(r.Context(), e.ID, id, "borrar"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id})
}

// archivarAnuncio: POST /anuncios/{id}/archivar. Sale del portal; sus envíos quedan como evidencia.
func (s *Server) archivarAnuncio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	id := idURL(r, "id")
	ct, err := s.DB.Exec(r.Context(), `UPDATE anuncio SET estado='archivado' WHERE id=$1 AND edificio_id=$2 AND estado='publicado'`, id, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if ct.RowsAffected() == 0 {
		P.Fallo(w, r, s.errorEstadoAnuncio(r.Context(), e.ID, id, "archivar"))
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "archivado"})
}

// errorEstadoAnuncio: 404 si no existe en el edificio; 409 si el estado no permite la acción.
func (s *Server) errorEstadoAnuncio(ctx context.Context, eid, id int64, accion string) error {
	var estado string
	if err := s.DB.QueryRow(ctx, `SELECT estado FROM anuncio WHERE id=$1 AND edificio_id=$2`, id, eid).Scan(&estado); err != nil {
		return P.NoEncontrado("el anuncio")
	}
	return P.Conflicto("ANUNCIO_"+strings.ToUpper(estado), "No se puede "+accion+" un anuncio «"+estado+"».").Con("estado", estado)
}

// destinatarioAnuncio: un vecino con su correo y celular (propietario o inquilino vigente).
type destinatarioAnuncio struct {
	unidad          int64
	codigo, nombre  string
	correo, celular string
}

func (s *Server) destinatariosAnuncio(ctx context.Context, q db.Q, eid int64) ([]destinatarioAnuncio, error) {
	filas, err := q.Query(ctx, `SELECT u.id, u.codigo, pe.nombre, COALESCE(pe.correo,''), COALESCE(pe.celular,'')
		FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id JOIN unidad u ON u.id=up.unidad_id
		WHERE u.edificio_id=$1 AND up.hasta IS NULL AND up.rol IN ('propietario','inquilino')
		ORDER BY u.codigo, (up.rol='propietario') DESC, pe.id`, eid)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []destinatarioAnuncio
	for filas.Next() {
		var d destinatarioAnuncio
		if err := filas.Scan(&d.unidad, &d.codigo, &d.nombre, &d.correo, &d.celular); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, filas.Err()
}

var plantillaAnuncio = template.Must(template.New("anuncio").Parse(`<!doctype html><html lang="es"><body style="margin:0;background:#F1F5F9;font-family:Arial,Helvetica,sans-serif;color:#0F172A">
<table width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:24px 12px">
<table width="560" cellpadding="0" cellspacing="0" style="background:#FFFFFF;border-radius:12px;overflow:hidden">
<tr><td style="background:#155E75;color:#FFFFFF;padding:18px 24px"><strong style="font-size:20px">EDISYS</strong><br><span style="font-size:13px">{{.Edificio}}</span></td></tr>
<tr><td style="padding:24px;font-size:15px;line-height:1.5">
<p>Hola, {{.Nombre}}:</p>
<h2 style="font-size:18px;margin:0 0 12px">{{.Titulo}}</h2>
<div style="white-space:pre-line">{{.Cuerpo}}</div>
<p style="margin-top:20px">Lo encuentras también en tu portal: <a href="{{.Enlace}}">{{.Enlace}}</a></p>
<p style="color:#64748B;font-size:12px">Comunicado de la administración enviado por EDISYS; no respondas a esta dirección.</p>
</td></tr></table></td></tr></table></body></html>`))

// textoAnuncio es el texto de WhatsApp y Telegram: título, cuerpo y enlace al portal.
func textoAnuncio(edificio, titulo, cuerpo, enlace string) string {
	return edificio + " · " + titulo + "\n\n" + cuerpo + "\n\nMás en tu portal: " + enlace
}

// publicarAnuncio: POST /anuncios/{id}/publicar. Pasa a publicado y encola los envíos de sus canales.
// El paso borrador→publicado es atómico: publicar dos veces no reenvía nada (409 la segunda).
func (s *Server) publicarAnuncio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	uid := ses(r).UsuarioID
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var titulo, cuerpo string
	var canales []string
	err = tx.QueryRow(ctx, `UPDATE anuncio SET estado='publicado', publicado_en=now()
		WHERE id=$1 AND edificio_id=$2 AND estado='borrador' RETURNING titulo, cuerpo, canales`, id, e.ID).Scan(&titulo, &cuerpo, &canales)
	if errors.Is(err, pgx.ErrNoRows) {
		P.Fallo(w, r, s.errorEstadoAnuncio(ctx, e.ID, id, "publicar"))
		return
	}
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	enlace := s.Cfg.URLPublica + "/app/anuncios/"
	ref := "anuncio:" + strconv.FormatInt(id, 10)
	envios := map[string]int{"correo": 0, "whatsapp": 0, "telegram": 0}
	sinCorreo, sinCelular := []string{}, []string{}
	quiere := map[string]bool{}
	for _, c := range canales {
		quiere[c] = true
	}
	// registrar deja la evidencia: una fila por destinatario y canal.
	registrar := func(canal, destino string, unidad *int64, col string, msgID int64) error {
		_, err := tx.Exec(ctx, `INSERT INTO anuncio_envio (anuncio_id, canal, destino, unidad_id, `+col+`) VALUES ($1,$2,$3,$4,$5)`, id, canal, destino, unidad, msgID)
		if err == nil {
			envios[canal]++
		}
		return err
	}

	if quiere["correo"] || quiere["whatsapp"] {
		ds, err := s.destinatariosAnuncio(ctx, tx, e.ID)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		correos, celulares := map[string]bool{}, map[string]bool{}
		faltaCorreo, faltaCel := map[string]bool{}, map[string]bool{}
		conCorreo, conCel := map[string]bool{}, map[string]bool{}
		for _, d := range ds {
			unidad := d.unidad
			if quiere["correo"] {
				c := strings.ToLower(strings.TrimSpace(d.correo))
				if !correo.ValidarDireccion(c) {
					faltaCorreo[d.codigo] = true
				} else {
					conCorreo[d.codigo] = true
					if !correos[c] {
						correos[c] = true
						var b bytes.Buffer
						_ = plantillaAnuncio.Execute(&b, map[string]string{"Edificio": e.Nombre, "Nombre": primerNombre(d.nombre), "Titulo": titulo, "Cuerpo": cuerpo, "Enlace": enlace})
						mid, err := s.encolarCorreo(ctx, tx, e.ID, &unidad, c, d.nombre, titulo+" · "+e.Nombre, b.String(),
							"Hola, "+primerNombre(d.nombre)+":\n\n"+titulo+"\n\n"+cuerpo+"\n\nMás en tu portal: "+enlace+"\n", "sistema", ref, nil, &uid)
						if err == nil {
							err = registrar("correo", c, &unidad, "correo_id", mid)
						}
						if err != nil {
							P.Fallo(w, r, err)
							return
						}
					}
				}
			}
			if quiere["whatsapp"] {
				tel := whatsapp.NormalizarTelefono(d.celular)
				if len(tel) < 9 {
					faltaCel[d.codigo] = true
				} else {
					conCel[d.codigo] = true
					if !celulares[tel] {
						celulares[tel] = true
						mid, err := s.encolar(ctx, tx, e.ID, &unidad, tel, "aviso_general", map[string]string{"mensaje": textoAnuncio(e.Nombre, titulo, cuerpo, enlace)}, "sistema", &uid)
						if err == nil {
							err = registrar("whatsapp", tel, &unidad, "whatsapp_id", mid)
						}
						if err != nil {
							P.Fallo(w, r, err)
							return
						}
					}
				}
			}
		}
		// Una unidad «sin correo» es la que no tiene a NADIE con correo válido.
		for c := range faltaCorreo {
			if !conCorreo[c] {
				sinCorreo = append(sinCorreo, c)
			}
		}
		for c := range faltaCel {
			if !conCel[c] {
				sinCelular = append(sinCelular, c)
			}
		}
	}
	sort.Strings(sinCorreo)
	sort.Strings(sinCelular)
	if quiere["telegram"] {
		ds, err := s.destinosTelegram(ctx, e.ID, nil)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		for _, d := range ds {
			did := d.id
			mid, err := s.encolarTelegram(ctx, tx, e.ID, &did, d.chat, textoAnuncio(e.Nombre, titulo, cuerpo, enlace), "anuncio", ref, &uid)
			if err == nil {
				err = registrar("telegram", d.chat, nil, "telegram_id", mid)
			}
			if err != nil {
				P.Fallo(w, r, err)
				return
			}
		}
	}
	if envios["correo"]+envios["whatsapp"]+envios["telegram"] > 0 {
		if _, err := tx.Exec(ctx, `UPDATE anuncio SET enviado_en=now() WHERE id=$1`, id); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	s.auditarCambio(ctx, tx, r, "anuncios", "publicar", "anuncio", id, nil, map[string]any{"estado": "publicado", "canales": canales, "envios": envios})
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	// Despacho inmediato (en simulado solo marca); lo que falle lo reintenta la bandeja.
	if envios["correo"] > 0 {
		s.despacharCorreos(ctx)
	}
	if envios["whatsapp"] > 0 {
		s.despacharPendientes(ctx)
	}
	if envios["telegram"] > 0 {
		s.despacharTelegram(ctx)
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": id, "estado": "publicado", "canales": canales, "envios": envios,
		"sin_correo": sinCorreo, "sin_celular": sinCelular, "resumen": s.resumenEnvios(ctx, id)})
}

// resumenEnvios cuenta los envíos de un anuncio por canal y estado vivo de su bandeja.
func (s *Server) resumenEnvios(ctx context.Context, id int64) []map[string]any {
	f, _ := db.Filas(ctx, s.DB, `SELECT v.canal, `+sqlEstadoEnvio+` AS estado, count(*) AS cantidad
		FROM anuncio_envio v `+sqlJoinEnvio+` WHERE v.anuncio_id=$1 GROUP BY 1,2 ORDER BY 1,2`, id)
	if f == nil {
		f = []map[string]any{}
	}
	return f
}

const sqlJoinEnvio = `LEFT JOIN correo_mensaje cm ON cm.id=v.correo_id LEFT JOIN whatsapp_mensaje wm ON wm.id=v.whatsapp_id
	LEFT JOIN telegram_mensaje tm ON tm.id=v.telegram_id`

// El estado se lee de la bandeja; si el mensaje se borró, queda «sin_registro».
const sqlEstadoEnvio = `COALESCE(cm.estado, wm.estado, tm.estado, 'sin_registro')`

// enviosAnuncio: GET /anuncios/{id}/envios → evidencia por destinatario con el estado vivo.
func (s *Server) enviosAnuncio(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	ctx := r.Context()
	id := idURL(r, "id")
	var ok bool
	_ = s.DB.QueryRow(ctx, `SELECT true FROM anuncio WHERE id=$1 AND edificio_id=$2`, id, e.ID).Scan(&ok)
	if !ok {
		P.Fallo(w, r, P.NoEncontrado("el anuncio"))
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT v.id, v.canal, v.destino, u.codigo AS unidad, `+sqlEstadoEnvio+` AS estado,
			COALESCE(cm.error, wm.error, tm.error, '') AS error, COALESCE(cm.procesado_en, wm.procesado_en, tm.procesado_en) AS procesado_en,
			COALESCE(v.correo_id, v.whatsapp_id, v.telegram_id) AS mensaje_id, v.creado_en
		FROM anuncio_envio v `+sqlJoinEnvio+` LEFT JOIN unidad u ON u.id=v.unidad_id
		WHERE v.anuncio_id=$1 ORDER BY v.canal, u.codigo NULLS LAST, v.id`, id)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1, "resumen": s.resumenEnvios(ctx, id)})
}
