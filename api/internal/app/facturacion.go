package app

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	qrcode "github.com/skip2/go-qrcode"

	"edisys/api/internal/db"
	"edisys/api/internal/pdf"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/sunat"
)

// Facturación electrónica SUNAT (bloque 5). Sin credenciales reales: el modo «simulado» firma con un
// certificado de prueba (o el .pfx cargado) y devuelve un CDR aceptado sin salir a la red; «beta» habla con el
// servicio de pruebas de SUNAT solo si hay usuario, clave y certificado; «produccion» está deshabilitado.

// URLBetaSUNAT: servicio de pruebas de SUNAT (billService).
const URLBetaSUNAT = "https://e-beta.sunat.gob.pe/ol-ti-itcpfegem-beta/billService"

type configFE struct {
	RUC, RazonSocial, Direccion, Ubigeo string
	SerieBoleta, SerieFactura, Modo     string
	OSEURL, OSEUsuario, OSEClave        string
	CertArchivo                         *int64
	CertClave                           string
	CertVence                           *time.Time
	Afectacion                          map[string]string
}

func (s *Server) configFacturacion(ctx context.Context, q db.Q, eid int64) (*configFE, error) {
	c := &configFE{Modo: "off", SerieBoleta: "B001", SerieFactura: "F001", Afectacion: map[string]string{}}
	var af []byte
	err := q.QueryRow(ctx, `SELECT ruc, razon_social, direccion, ubigeo, serie_boleta, serie_factura, modo, ose_url, ose_usuario, ose_clave,
			certificado_archivo_id, certificado_clave, certificado_vence, afectacion FROM facturacion_config WHERE edificio_id=$1`, eid).
		Scan(&c.RUC, &c.RazonSocial, &c.Direccion, &c.Ubigeo, &c.SerieBoleta, &c.SerieFactura, &c.Modo, &c.OSEURL, &c.OSEUsuario, &c.OSEClave,
			&c.CertArchivo, &c.CertClave, &c.CertVence, &af)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(af, &c.Afectacion)
	return c, nil
}

// verConfigFacturacion: GET /facturacion/config. El certificado y las claves nunca se devuelven.
func (s *Server) verConfigFacturacion(w http.ResponseWriter, r *http.Request) {
	c, err := s.configFacturacion(r.Context(), s.DB, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var vence any
	if c.CertVence != nil {
		vence = c.CertVence.Format("2006-01-02")
	}
	P.JSON(w, http.StatusOK, map[string]any{"ruc": c.RUC, "razon_social": c.RazonSocial, "direccion": c.Direccion, "ubigeo": c.Ubigeo,
		"serie_boleta": c.SerieBoleta, "serie_factura": c.SerieFactura, "modo": c.Modo, "ose_url": c.OSEURL, "ose_usuario": c.OSEUsuario,
		"tiene_ose_clave": c.OSEClave != "", "tiene_certificado": c.CertArchivo != nil, "certificado_vence": vence, "afectacion": c.Afectacion,
		"tiene_beta_servidor": s.Cfg.SUNATBetaUsuario != "" && s.Cfg.SUNATBetaClave != "",
		"modos": []string{"off", "simulado", "beta", "produccion"}, "produccion_habilitada": false,
		"aviso": "Producción está deshabilitada en esta entrega. «Simulado» no envía nada a SUNAT; «beta» usa el entorno de pruebas y necesita usuario, clave y certificado (los del edificio, o los de prueba del servidor)."})
}

var reSerie = map[string]*regexp.Regexp{"B": regexp.MustCompile(`^B[A-Z0-9]{3}$`), "F": regexp.MustCompile(`^F[A-Z0-9]{3}$`)}

// guardarConfigFacturacion: PUT /facturacion/config {ruc, razon_social, direccion, ubigeo, serie_boleta, serie_factura,
// modo, ose_url, ose_usuario, ose_clave?, afectacion?}.
func (s *Server) guardarConfigFacturacion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RUC          string            `json:"ruc"`
		RazonSocial  string            `json:"razon_social"`
		Direccion    string            `json:"direccion"`
		Ubigeo       string            `json:"ubigeo"`
		SerieBoleta  string            `json:"serie_boleta"`
		SerieFactura string            `json:"serie_factura"`
		Modo         string            `json:"modo"`
		OSEURL       string            `json:"ose_url"`
		OSEUsuario   string            `json:"ose_usuario"`
		OSEClave     *string           `json:"ose_clave"`
		Afectacion   map[string]string `json:"afectacion"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.SerieBoleta, in.SerieFactura = strings.ToUpper(strings.TrimSpace(in.SerieBoleta)), strings.ToUpper(strings.TrimSpace(in.SerieFactura))
	if in.SerieBoleta == "" {
		in.SerieBoleta = "B001"
	}
	if in.SerieFactura == "" {
		in.SerieFactura = "F001"
	}
	ev := P.Validacion("Revisa la configuración de facturación.")
	switch in.Modo {
	case "off", "simulado", "beta":
	case "produccion":
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "PRODUCCION_DESHABILITADA", "El modo producción está deshabilitado en esta entrega: usa simulado o beta.").Campo("modo", "No disponible."))
		return
	default:
		ev.Campo("modo", "off, simulado o beta.")
	}
	if in.Modo != "off" {
		if !sunat.RUCValido(strings.TrimSpace(in.RUC)) {
			ev.Campo("ruc", "RUC de 11 dígitos válido.")
		}
		if strings.TrimSpace(in.RazonSocial) == "" {
			ev.Campo("razon_social", "Obligatoria.")
		}
	}
	if !reSerie["B"].MatchString(in.SerieBoleta) {
		ev.Campo("serie_boleta", "B y 3 caracteres, p. ej. B001.")
	}
	if !reSerie["F"].MatchString(in.SerieFactura) {
		ev.Campo("serie_factura", "F y 3 caracteres, p. ej. F001.")
	}
	for k, v := range in.Afectacion {
		if v != sunat.Gravado && v != sunat.Exonerado && v != sunat.Inafecto {
			ev.Campo("afectacion."+k, "gravado, exonerado o inafecto.")
		}
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	e := edf(r)
	ctx := r.Context()
	actual, err := s.configFacturacion(ctx, s.DB, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	for k, v := range in.Afectacion {
		actual.Afectacion[k] = v
	}
	if len(actual.Afectacion) == 0 {
		actual.Afectacion = nil
	}
	af, _ := json.Marshal(actual.Afectacion)
	cambiaClave := in.OSEClave != nil && *in.OSEClave != ""
	clave := ""
	if cambiaClave {
		clave = *in.OSEClave
	}
	// La URL vacía en beta se guarda vacía a propósito: oseBeta la resuelve al enviar
	// (primero SUNAT_BETA_URL del servidor, si no, el beta de SUNAT).
	if _, err := s.DB.Exec(ctx, `INSERT INTO facturacion_config (edificio_id, ruc, razon_social, direccion, ubigeo, serie_boleta, serie_factura, modo, ose_url, ose_usuario, ose_clave, afectacion, actualizado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,COALESCE(NULLIF($12,'null')::jsonb, '{}'::jsonb),$13)
		ON CONFLICT (edificio_id) DO UPDATE SET ruc=EXCLUDED.ruc, razon_social=EXCLUDED.razon_social, direccion=EXCLUDED.direccion, ubigeo=EXCLUDED.ubigeo,
		  serie_boleta=EXCLUDED.serie_boleta, serie_factura=EXCLUDED.serie_factura, modo=EXCLUDED.modo, ose_url=EXCLUDED.ose_url, ose_usuario=EXCLUDED.ose_usuario,
		  ose_clave = CASE WHEN $14 THEN EXCLUDED.ose_clave ELSE facturacion_config.ose_clave END,
		  afectacion = CASE WHEN $12 = 'null' THEN facturacion_config.afectacion ELSE EXCLUDED.afectacion END,
		  actualizado_por=EXCLUDED.actualizado_por, actualizado_en=now()`,
		e.ID, strings.TrimSpace(in.RUC), strings.TrimSpace(in.RazonSocial), in.Direccion, in.Ubigeo, in.SerieBoleta, in.SerieFactura, in.Modo,
		strings.TrimRight(in.OSEURL, "/"), in.OSEUsuario, clave, string(af), ses(r).UsuarioID, cambiaClave); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "facturacion", "config", "facturacion_config", e.ID, nil,
		map[string]any{"modo": in.Modo, "ruc": in.RUC, "series": []string{in.SerieBoleta, in.SerieFactura}, "ose_clave_cambiada": cambiaClave})
	s.verConfigFacturacion(w, r)
}

// subirCertificado: POST /facturacion/certificado (multipart {certificado: .pfx/.p12, clave}). Se guarda en el
// cubo privado; ni el archivo ni la clave vuelven por el API.
func (s *Server) subirCertificado(w http.ResponseWriter, r *http.Request) {
	if err := leerMultipart(r); err != nil {
		P.Fallo(w, r, err)
		return
	}
	fh := firstFile(r, "certificado", "archivo")
	clave := campo(r, "clave")
	if fh == nil {
		P.Fallo(w, r, P.Validacion("Sube el certificado digital (.pfx o .p12).").Campo("certificado", "Obligatorio."))
		return
	}
	f, err := fh.Open()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	datos, err := io.ReadAll(io.LimitReader(f, 1<<20))
	f.Close()
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	cert, err := sunat.LeerPFX(datos, clave)
	if err != nil {
		P.Fallo(w, r, P.Err(http.StatusUnprocessableEntity, "CERTIFICADO_INVALIDO", "No pude abrir el certificado con esa clave.").Campo("clave", "Revisa la clave del certificado."))
		return
	}
	if s.Almacen == nil {
		P.Fallo(w, r, P.Err(http.StatusServiceUnavailable, "SIN_ALMACEN", "El almacén de archivos no está disponible."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	llave := fmt.Sprintf("e%d/certificados/%d.pfx", e.ID, time.Now().UnixNano())
	if err := s.Almacen.Subir(ctx, llave, datos, "application/x-pkcs12"); err != nil {
		P.Fallo(w, r, err)
		return
	}
	uid := ses(r).UsuarioID
	var aid int64
	if err := s.DB.QueryRow(ctx, `INSERT INTO archivo (edificio_id, clave, nombre, tipo_mime, tamano, subido_por) VALUES ($1,$2,'certificado.pfx','application/x-pkcs12',$3,$4) RETURNING id`,
		e.ID, llave, len(datos), uid).Scan(&aid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO facturacion_config (edificio_id, certificado_archivo_id, certificado_clave, certificado_vence, actualizado_por) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (edificio_id) DO UPDATE SET certificado_archivo_id=EXCLUDED.certificado_archivo_id, certificado_clave=EXCLUDED.certificado_clave,
		  certificado_vence=EXCLUDED.certificado_vence, actualizado_por=EXCLUDED.actualizado_por, actualizado_en=now()`, e.ID, aid, clave, cert.Cert.NotAfter, uid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "facturacion", "certificado", "facturacion_config", e.ID, nil, map[string]any{"vence": cert.Cert.NotAfter.Format("2006-01-02"), "titular": cert.Cert.Subject.CommonName})
	P.JSON(w, http.StatusCreated, map[string]any{"tiene_certificado": true, "certificado_vence": cert.Cert.NotAfter.Format("2006-01-02"), "titular": cert.Cert.Subject.CommonName})
}

// certificadoPara: el .pfx configurado, o el de prueba en simulado. Beta exige uno real.
func (s *Server) certificadoPara(ctx context.Context, c *configFE) (*sunat.Certificado, error) {
	if c.CertArchivo != nil && s.Almacen != nil {
		var llave string
		if err := s.DB.QueryRow(ctx, `SELECT clave FROM archivo WHERE id=$1`, *c.CertArchivo).Scan(&llave); err == nil {
			if datos, err := s.Almacen.Leer(ctx, llave); err == nil {
				if cert, err := sunat.LeerPFX(datos, c.CertClave); err == nil {
					return cert, nil
				}
			}
		}
		if c.Modo == "beta" {
			return nil, P.Err(http.StatusUnprocessableEntity, "CERTIFICADO_INVALIDO", "No pude leer el certificado configurado.")
		}
	}
	if c.Modo == "beta" {
		return nil, P.Err(http.StatusUnprocessableEntity, "SIN_CREDENCIALES_BETA", "Para el modo beta sube el certificado digital.")
	}
	return sunat.CertificadoPrueba()
}

// siguienteNumero reserva el correlativo de la serie dentro de la transacción (sin huecos).
func siguienteNumero(ctx context.Context, tx pgx.Tx, eid int64, serie string) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `INSERT INTO comprobante_serie (edificio_id, serie, ultimo) VALUES ($1,$2,1)
		ON CONFLICT (edificio_id, serie) DO UPDATE SET ultimo = comprobante_serie.ultimo + 1 RETURNING ultimo`, eid, serie).Scan(&n)
	return n, err
}

// oseBeta: cliente OSE del modo beta. Manda lo del edificio; si falta, lo del servidor
// (credenciales de prueba SUNAT_BETA_*); la URL cae al servicio beta de SUNAT.
func (s *Server) oseBeta(c *configFE) sunat.OSE {
	o := sunat.OSE{URL: c.OSEURL, Usuario: c.OSEUsuario, Clave: c.OSEClave, HTTP: s.HTTP}
	if o.URL == "" {
		o.URL = s.Cfg.SUNATBetaURL
	}
	if o.URL == "" {
		o.URL = URLBetaSUNAT
	}
	if o.Usuario == "" {
		o.Usuario = s.Cfg.SUNATBetaUsuario
	}
	if o.Clave == "" {
		o.Clave = s.Cfg.SUNATBetaClave
	}
	return o
}

// enviar firma y obtiene el CDR según el modo. nombre = RUC-TIPO-SERIE-NUMERO.
func (s *Server) enviar(ctx context.Context, c *configFE, cert *sunat.Certificado, doc, tipo, id string) (firmado, hash string, cdr sunat.CDR, err error) {
	firmado, hash, err = sunat.Firmar(doc, cert)
	if err != nil {
		return
	}
	if c.Modo == "beta" {
		o := s.oseBeta(c)
		if o.Usuario == "" || o.Clave == "" {
			err = P.Err(http.StatusUnprocessableEntity, "SIN_CREDENCIALES_BETA", "Para el modo beta configura el usuario y la clave SOL de pruebas (o SUNAT_BETA_* en el servidor).")
			return
		}
		cdr, err = o.EnviarComprobante(ctx, c.RUC+"-"+tipo+"-"+id, []byte(firmado))
		return
	}
	cdr = sunat.CDRSimulado(c.RUC, tipo, id, hash, time.Now().In(P.Lima))
	return
}

func (s *Server) listoParaEmitir(ctx context.Context, eid int64) (*configFE, error) {
	c, err := s.configFacturacion(ctx, s.DB, eid)
	if err != nil {
		return nil, err
	}
	switch c.Modo {
	case "off":
		return nil, P.Conflicto("FACTURACION_APAGADA", "La facturación electrónica está apagada: actívala en Configuración › Facturación electrónica.")
	case "produccion":
		return nil, P.Err(http.StatusUnprocessableEntity, "PRODUCCION_DESHABILITADA", "El modo producción está deshabilitado en esta entrega.")
	}
	if !sunat.RUCValido(c.RUC) || c.RazonSocial == "" {
		return nil, P.Err(http.StatusUnprocessableEntity, "CONFIG_INCOMPLETA", "Falta el RUC o la razón social del emisor.")
	}
	return c, nil
}

const sqlComprobante = `SELECT c.id, c.recibo_id, c.tipo, c.serie, c.numero, c.serie || '-' || c.numero AS numero_completo, to_char(c.fecha,'YYYY-MM-DD') AS fecha,
	c.cliente_tipo_doc, c.cliente_doc, c.cliente_nombre, c.gravado_cts, c.exonerado_cts, c.inafecto_cts, c.igv_cts, c.total_cts, c.modo, c.estado, c.hash,
	c.cdr_codigo, c.cdr_descripcion, c.firmado_prueba, c.referencia_id, c.anulacion, c.anulacion_motivo, c.baja_id, c.baja_ticket, c.creado_en
	FROM comprobante c`

// emitirComprobante: POST /recibos/{rid}/comprobante {cliente_doc?, cliente_nombre?} → 201 comprobante con CDR.
// Boleta a persona (DNI o sin documento), factura a empresa (RUC). 409 YA_TIENE_COMPROBANTE.
func (s *Server) emitirComprobante(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		ClienteDoc    string `json:"cliente_doc"`
		ClienteNombre string `json:"cliente_nombre"`
	}
	if r.ContentLength > 0 {
		if err := P.Leer(r, &in); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	e := edf(r)
	ctx := r.Context()
	out, err := s.EmitirComprobante(ctx, e, rid, ses(r).UsuarioID, in.ClienteDoc, in.ClienteNombre)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, out)
}

// EmitirComprobante: la emisión completa en una transacción (correlativo, XML, firma, CDR, registro).
func (s *Server) EmitirComprobante(ctx context.Context, e *Edificio, rid, uid int64, doc, nombre string) (map[string]any, error) {
	c, err := s.listoParaEmitir(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	rc, err := db.Fila(ctx, s.DB, `SELECT r.id, r.estado, r.origen, r.total_cts, COALESCE(r.numero,'') AS numero,
			COALESCE(pe.dni_ruc,'') AS doc, COALESCE(pe.nombre,'') AS nombre
		FROM recibo r LEFT JOIN LATERAL (SELECT pe.dni_ruc, pe.nombre FROM unidad_persona up JOIN persona pe ON pe.id=up.persona_id
			WHERE up.unidad_id=r.unidad_id AND up.rol='propietario' AND up.hasta IS NULL LIMIT 1) pe ON true
		WHERE r.id=$1 AND r.edificio_id=$2`, rid, e.ID)
	if err != nil {
		return nil, P.NoEncontrado("el recibo")
	}
	if rc["estado"] == "borrador" || rc["estado"] == "anulado" {
		return nil, P.Conflicto("RECIBO_NO_EMITIDO", "Solo se emite comprobante de un recibo emitido.")
	}
	if rc["origen"] != "periodo" {
		return nil, P.Err(http.StatusUnprocessableEntity, "RECIBO_SIN_COMPROBANTE", "La deuda anterior a EDISYS no lleva comprobante electrónico.")
	}
	if strings.TrimSpace(doc) == "" {
		doc = rc["doc"].(string)
	}
	if strings.TrimSpace(nombre) == "" {
		nombre = rc["nombre"].(string)
	}
	if nombre == "" {
		nombre = "Cliente varios"
	}
	cli, tipo := sunat.ClienteDe(doc, nombre)
	var ls []sunat.Linea
	fl, err := db.Filas(ctx, s.DB, `SELECT tipo, descripcion, monto_cts FROM recibo_linea WHERE recibo_id=$1 ORDER BY orden, id`, rid)
	if err != nil {
		return nil, err
	}
	for _, l := range fl {
		af := c.Afectacion[l["tipo"].(string)]
		if af == "" {
			af = sunat.Inafecto
		}
		ls = append(ls, sunat.Linea{Descripcion: l["descripcion"].(string), MontoCts: l["monto_cts"].(int64), Afectacion: af})
	}
	ls = sunat.Normalizar(ls)
	if len(ls) == 0 {
		return nil, P.Err(http.StatusUnprocessableEntity, "RECIBO_EN_CERO", "El recibo no tiene importes que facturar.")
	}
	if tipo == sunat.Boleta && cli.TipoDoc == "0" && rc["total_cts"].(int64) > 70000 {
		return nil, P.Validacion("Una boleta de más de S/ 700 necesita el DNI del cliente.").Campo("cliente_doc", "DNI o RUC.")
	}
	cert, err := s.certificadoPara(ctx, c)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Un recibo, un comprobante vivo (se bloquea el recibo para que dos emisiones no choquen).
	if _, err := tx.Exec(ctx, `SELECT 1 FROM recibo WHERE id=$1 FOR UPDATE`, rid); err != nil {
		return nil, err
	}
	var ya int64
	if err := tx.QueryRow(ctx, `SELECT id FROM comprobante WHERE recibo_id=$1 AND tipo IN ('01','03') AND estado NOT IN ('anulado','rechazado')`, rid).Scan(&ya); err == nil {
		return nil, P.Conflicto("YA_TIENE_COMPROBANTE", "Ese recibo ya tiene comprobante electrónico.").Con("comprobante_id", ya)
	}
	serie := c.SerieBoleta
	if tipo == sunat.Factura {
		serie = c.SerieFactura
	}
	num, err := siguienteNumero(ctx, tx, e.ID, serie)
	if err != nil {
		return nil, err
	}
	ahora := time.Now().In(P.Lima)
	cp := sunat.Comprobante{Tipo: tipo, Serie: serie, Numero: num, Fecha: ahora.Format("2006-01-02"), Hora: ahora.Format("15:04:05"),
		Emisor: sunat.Emisor{RUC: c.RUC, RazonSocial: c.RazonSocial, Direccion: c.Direccion, Ubigeo: c.Ubigeo}, Cliente: cli, Lineas: ls}
	return s.registrar(ctx, tx, e.ID, &rid, nil, c, cert, cp, uid)
}

func (s *Server) registrar(ctx context.Context, tx pgx.Tx, eid int64, rid, ref *int64, c *configFE, cert *sunat.Certificado, cp sunat.Comprobante, uid int64) (map[string]any, error) {
	doc, tot, err := sunat.XML(cp)
	if err != nil {
		return nil, err
	}
	firmado, hash, cdr, err := s.enviar(ctx, c, cert, doc, cp.Tipo, cp.ID())
	if err != nil {
		return nil, err
	}
	estado := "aceptado"
	if !cdr.Aceptado() {
		estado = "rechazado"
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO comprobante (edificio_id, recibo_id, tipo, serie, numero, fecha, cliente_tipo_doc, cliente_doc, cliente_nombre,
			gravado_cts, exonerado_cts, inafecto_cts, igv_cts, total_cts, modo, estado, xml, hash, cdr_codigo, cdr_descripcion, cdr_xml, firmado_prueba, referencia_id, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24) RETURNING id`,
		eid, rid, cp.Tipo, cp.Serie, cp.Numero, cp.Fecha, cp.Cliente.TipoDoc, cp.Cliente.NumDoc, cp.Cliente.Nombre, tot.GravadoCts, tot.ExoneradoCts, tot.InafectoCts,
		tot.IGVCts, tot.TotalCts, c.Modo, estado, firmado, hash, cdr.Codigo, cdr.Descripcion, cdr.XML, cert.Prueba, ref, uid).Scan(&id); err != nil {
		return nil, P.Traducir(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.comprobante(ctx, eid, id)
}

func (s *Server) comprobante(ctx context.Context, eid, id int64) (map[string]any, error) {
	f, err := db.Fila(ctx, s.DB, sqlComprobante+` WHERE c.id=$1 AND c.edificio_id=$2`, id, eid)
	if err != nil {
		return nil, P.NoEncontrado("el comprobante")
	}
	base := fmt.Sprintf("/api/v1/edificios/%d/comprobantes/%d", eid, id)
	f["xml_url"], f["pdf_url"], f["cdr_url"] = base+"/xml", base+"/pdf", base+"/cdr"
	f["nombre_tipo"] = map[string]string{"01": "Factura electrónica", "03": "Boleta de venta electrónica", "07": "Nota de crédito electrónica"}[f["tipo"].(string)]
	return f, nil
}

// comprobanteVisible: el comprobante del edificio; al propietario, solo los de sus recibos.
func (s *Server) comprobanteVisible(ctx context.Context, e *Edificio, cid int64) (map[string]any, error) {
	var rid *int64
	if err := s.DB.QueryRow(ctx, `SELECT COALESCE(c.recibo_id, ref.recibo_id) FROM comprobante c LEFT JOIN comprobante ref ON ref.id=c.referencia_id
		WHERE c.id=$1 AND c.edificio_id=$2`, cid, e.ID).Scan(&rid); err != nil {
		return nil, P.NoEncontrado("el comprobante")
	}
	if rid != nil {
		if _, err := s.reciboVisible(ctx, e, *rid); err != nil {
			return nil, P.NoEncontrado("el comprobante")
		}
	} else if e.SoloLoSuyo() {
		return nil, P.NoEncontrado("el comprobante")
	}
	return s.comprobante(ctx, e.ID, cid)
}

// comprobantesDeRecibo: GET /recibos/{rid}/comprobante → {comprobantes, modo, puede_emitir}.
func (s *Server) comprobantesDeRecibo(w http.ResponseWriter, r *http.Request) {
	rid, err := idRuta(r, "rid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	e := edf(r)
	ctx := r.Context()
	if _, err := s.reciboVisible(ctx, e, rid); err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT c.id FROM comprobante c WHERE c.edificio_id=$1 AND (c.recibo_id=$2 OR c.referencia_id IN (SELECT id FROM comprobante WHERE recibo_id=$2)) ORDER BY c.id`, e.ID, rid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	out := []map[string]any{}
	for _, f := range filas {
		if c, err := s.comprobante(ctx, e.ID, f["id"].(int64)); err == nil {
			out = append(out, c)
		}
	}
	cfg, _ := s.configFacturacion(ctx, s.DB, e.ID)
	modo := "off"
	if cfg != nil {
		modo = cfg.Modo
	}
	P.JSON(w, http.StatusOK, map[string]any{"comprobantes": out, "modo": modo, "puede_emitir": e.Puede("comprobantes.emitir") && modo != "off" && modo != "produccion"})
}

func (s *Server) cidDe(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	cid, err := idRuta(r, "cid")
	if err != nil {
		P.Fallo(w, r, err)
		return nil, false
	}
	c, err := s.comprobanteVisible(r.Context(), edf(r), cid)
	if err != nil {
		P.Fallo(w, r, err)
		return nil, false
	}
	return c, true
}

// verComprobante: GET /comprobantes/{cid}
func (s *Server) verComprobante(w http.ResponseWriter, r *http.Request) {
	if c, ok := s.cidDe(w, r); ok {
		P.JSON(w, http.StatusOK, c)
	}
}

// xmlComprobante: GET /comprobantes/{cid}/xml (y /cdr).
func (s *Server) xmlComprobante(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cidDe(w, r)
	if !ok {
		return
	}
	col, suf := "xml", ""
	if strings.HasSuffix(r.URL.Path, "/cdr") {
		col, suf = "cdr_xml", "R-"
	}
	var contenido, ruc string
	_ = s.DB.QueryRow(r.Context(), `SELECT `+col+`, COALESCE((SELECT ruc FROM facturacion_config WHERE edificio_id=$2),'') FROM comprobante WHERE id=$1`, c["id"], edf(r).ID).Scan(&contenido, &ruc)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s%s-%s-%s.xml"`, suf, ruc, c["tipo"], c["numero_completo"]))
	_, _ = io.WriteString(w, contenido)
}

// QRTexto: el contenido del QR de la representación impresa (RUC|TIPO|SERIE|NÚMERO|IGV|TOTAL|FECHA|TIPO DOC|DOC|HASH|).
func QRTexto(ruc string, c map[string]any) string {
	return strings.Join([]string{ruc, c["tipo"].(string), c["serie"].(string), strconv.FormatInt(c["numero"].(int64), 10),
		soles2(c["igv_cts"].(int64)), soles2(c["total_cts"].(int64)), c["fecha"].(string), c["cliente_tipo_doc"].(string), c["cliente_doc"].(string), c["hash"].(string)}, "|") + "|"
}

func soles2(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }

// pdfComprobante: GET /comprobantes/{cid}/pdf — representación impresa con código QR.
func (s *Server) pdfComprobante(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cidDe(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	cfg, _ := s.configFacturacion(ctx, s.DB, edf(r).ID)
	d := pdf.Nuevo()
	d.Rect(0, 770, 595, 72, 0.082, 0.369, 0.459)
	d.Color(1, 1, 1)
	d.Texto(40, 808, 15, true, cfg.RazonSocial)
	d.Texto(40, 790, 10, false, cfg.Direccion)
	d.Texto(40, 776, 10, false, "RUC "+cfg.RUC)
	d.TextoDerecha(555, 808, 12, true, strings.ToUpper(c["nombre_tipo"].(string)))
	d.TextoDerecha(555, 788, 14, true, c["numero_completo"].(string))
	d.Color(tinta[0], tinta[1], tinta[2])
	y := 740.0
	nombreDoc := map[string]string{"1": "DNI", "6": "RUC", "0": "Doc."}[c["cliente_tipo_doc"].(string)]
	d.Texto(40, y, 10, true, "Cliente: "+c["cliente_nombre"].(string))
	d.Texto(40, y-15, 10, false, nombreDoc+": "+c["cliente_doc"].(string)+"    Fecha de emisión: "+c["fecha"].(string)+"    Moneda: soles")
	if ref, ok := c["referencia_id"].(int64); ok {
		var refNum, motivo string
		_ = s.DB.QueryRow(ctx, `SELECT serie || '-' || numero, COALESCE(anulacion_motivo,'') FROM comprobante WHERE id=$1`, ref).Scan(&refNum, &motivo)
		d.Texto(40, y-30, 10, false, "Documento que modifica: "+refNum+" · Motivo: 01 Anulación de la operación. "+motivo)
	}
	y -= 60
	d.Texto(40, y, 10, true, "Descripción")
	d.TextoDerecha(555, y, 10, true, "Importe")
	d.Linea(40, y-6, 555, y-6)
	y -= 22
	lineas := lineasDelXML(ctx, s, c["id"].(int64))
	for _, l := range lineas {
		d.Texto(40, y, 9.5, false, recortar(l[0], 80))
		d.TextoDerecha(555, y, 9.5, false, l[1])
		y -= 16
	}
	d.Linea(40, y+6, 555, y+6)
	y -= 12
	for _, t := range [][2]string{{"Op. gravada", P.Soles(c["gravado_cts"].(int64))}, {"Op. exonerada", P.Soles(c["exonerado_cts"].(int64))},
		{"Op. inafecta", P.Soles(c["inafecto_cts"].(int64))}, {"IGV (18 %)", P.Soles(c["igv_cts"].(int64))}} {
		d.Texto(360, y, 9.5, false, t[0])
		d.TextoDerecha(555, y, 9.5, false, t[1])
		y -= 15
	}
	d.Texto(360, y-2, 12, true, "Importe total")
	d.TextoDerecha(555, y-2, 12, true, P.Soles(c["total_cts"].(int64)))
	y -= 26
	d.Texto(40, y, 9, false, sunat.MontoEnLetras(c["total_cts"].(int64)))
	// QR.
	if q, err := qrcode.New(QRTexto(cfg.RUC, c), qrcode.Medium); err == nil {
		q.DisableBorder = true
		bm := q.Bitmap()
		lado := 110.0
		modulo := lado / float64(len(bm))
		x0, y0 := 40.0, y-30-lado
		for i, fila := range bm {
			for j, negro := range fila {
				if negro {
					d.Rect(x0+float64(j)*modulo, y0+lado-float64(i+1)*modulo, modulo+0.05, modulo+0.05, 0, 0, 0)
				}
			}
		}
		d.Color(tinta[0], tinta[1], tinta[2])
		d.Texto(170, y0+lado-10, 9, false, "Hash (DigestValue): "+c["hash"].(string))
		d.Texto(170, y0+lado-26, 9, false, fmt.Sprintf("Estado SUNAT: %s · CDR %s", c["estado"], c["cdr_codigo"]))
		d.Texto(170, y0+lado-40, 8.5, false, recortar(c["cdr_descripcion"].(string), 90))
		d.Texto(170, y0+lado-56, 8.5, false, "Representación impresa de la "+strings.ToLower(c["nombre_tipo"].(string))+".")
	}
	if c["modo"] == "simulado" {
		d.Color(0.75, 0.1, 0.1)
		d.Texto(40, 90, 11, true, "SIMULADO: no se envió a SUNAT y no tiene validez tributaria.")
		if c["firmado_prueba"] == true {
			d.Texto(40, 74, 9, false, "Firmado con el certificado de prueba de EDISYS.")
		}
		d.Color(tinta[0], tinta[1], tinta[2])
	}
	if c["estado"] == "anulado" {
		d.Color(0.75, 0.1, 0.1)
		d.Texto(40, 58, 11, true, fmt.Sprintf("ANULADO por %s: %v", strings.ReplaceAll(fmt.Sprint(c["anulacion"]), "_", " de "), val(c["anulacion_motivo"])))
		d.Color(tinta[0], tinta[1], tinta[2])
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.pdf"`, c["numero_completo"]))
	_, _ = w.Write(d.Bytes())
}

var reLinea = regexp.MustCompile(`<cbc:Description>([^<]*)</cbc:Description></cac:Item><cac:Price>`)
var reImporteLinea = regexp.MustCompile(`<cac:AlternativeConditionPrice><cbc:PriceAmount currencyID="PEN">([0-9.]+)</cbc:PriceAmount>`)

// lineasDelXML: descripción e importe (con IGV) de cada línea, leídos del XML firmado (lo que vale ante SUNAT).
func lineasDelXML(ctx context.Context, s *Server, id int64) [][2]string {
	var doc string
	_ = s.DB.QueryRow(ctx, `SELECT xml FROM comprobante WHERE id=$1`, id).Scan(&doc)
	ds, ms := reLinea.FindAllStringSubmatch(doc, -1), reImporteLinea.FindAllStringSubmatch(doc, -1)
	out := [][2]string{}
	for i := range ds {
		imp := ""
		if i < len(ms) {
			v, _ := strconv.ParseFloat(ms[i][1], 64)
			imp = P.Soles(int64(v*100 + 0.5))
		}
		out = append(out, [2]string{strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(ds[i][1]), imp})
	}
	return out
}

// anularComprobante: POST /comprobantes/{cid}/anular {motivo}. Factura de hasta 7 días → comunicación de baja;
// boleta o factura más antigua → nota de crédito (07, motivo 01 «Anulación de la operación»).
func (s *Server) anularComprobante(w http.ResponseWriter, r *http.Request) {
	cid, err := idRuta(r, "cid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Motivo) == "" {
		P.Fallo(w, r, P.Validacion("Escribe el motivo de la anulación.").Campo("motivo", "Obligatorio."))
		return
	}
	e := edf(r)
	ctx := r.Context()
	c, err := s.listoParaEmitir(ctx, e.ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	cert, err := s.certificadoPara(ctx, c)
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
	var tipo, serie, estado, xmlOrig string
	var numero int64
	var fecha time.Time
	var recibo *int64
	if err := tx.QueryRow(ctx, `SELECT tipo, serie, numero, fecha, estado, xml, recibo_id FROM comprobante WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, cid, e.ID).
		Scan(&tipo, &serie, &numero, &fecha, &estado, &xmlOrig, &recibo); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el comprobante"))
		return
	}
	if tipo == sunat.NotaCredito {
		P.Fallo(w, r, P.Conflicto("NO_ANULABLE", "Una nota de crédito no se anula desde aquí."))
		return
	}
	if estado != "aceptado" {
		P.Fallo(w, r, P.Conflicto("NO_ANULABLE", "Solo se anula un comprobante aceptado (este está "+estado+")."))
		return
	}
	hoy := time.Now().In(P.Lima)
	uid := ses(r).UsuarioID
	if tipo == sunat.Factura && hoy.Sub(fecha) <= 7*24*time.Hour {
		// Comunicación de baja.
		var n int64
		if n, err = siguienteNumero(ctx, tx, e.ID, "RA-"+hoy.Format("20060102")); err != nil {
			P.Fallo(w, r, err)
			return
		}
		id := fmt.Sprintf("RA-%s-%d", hoy.Format("20060102"), n)
		doc := sunat.Baja(sunat.Emisor{RUC: c.RUC, RazonSocial: c.RazonSocial}, id, hoy.Format("2006-01-02"), fecha.Format("2006-01-02"), tipo, serie, numero, in.Motivo)
		firmado, hash, err := sunat.Firmar(doc, cert)
		if err != nil {
			P.Fallo(w, r, err)
			return
		}
		ticket := "SIM-" + strconv.FormatInt(time.Now().UnixNano(), 10)
		if c.Modo == "beta" {
			if ticket, err = s.oseBeta(c).EnviarResumen(ctx, c.RUC+"-"+id, []byte(firmado)); err != nil {
				P.Fallo(w, r, P.Err(http.StatusBadGateway, "SUNAT_ERROR", err.Error()))
				return
			}
		}
		cdr := sunat.CDRSimulado(c.RUC, "RA", id, hash, hoy)
		if _, err := tx.Exec(ctx, `UPDATE comprobante SET estado='anulado', anulacion='baja', anulacion_motivo=$2, baja_id=$3, baja_ticket=$4,
			cdr_descripcion = cdr_descripcion || ' · ' || $5 WHERE id=$1`, cid, in.Motivo, id, ticket, cdr.Descripcion); err != nil {
			P.Fallo(w, r, err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			P.Fallo(w, r, err)
			return
		}
		s.auditarCambio(ctx, s.DB, r, "facturacion", "baja", "comprobante", cid, map[string]any{"estado": estado}, map[string]any{"baja": id, "ticket": ticket, "motivo": in.Motivo})
		out, _ := s.comprobante(ctx, e.ID, cid)
		out["via"] = "baja"
		P.JSON(w, http.StatusOK, out)
		return
	}
	// Nota de crédito por el total, con las mismas líneas.
	var cliTipo, cliDoc, cliNombre string
	_ = tx.QueryRow(ctx, `SELECT cliente_tipo_doc, cliente_doc, cliente_nombre FROM comprobante WHERE id=$1`, cid).Scan(&cliTipo, &cliDoc, &cliNombre)
	ls := lineasComprobante(xmlOrig)
	serieNC := "BC01"
	if tipo == sunat.Factura {
		serieNC = "FC01"
	}
	num, err := siguienteNumero(ctx, tx, e.ID, serieNC)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE comprobante SET estado='anulado', anulacion='nota_credito', anulacion_motivo=$2 WHERE id=$1`, cid, in.Motivo); err != nil {
		P.Fallo(w, r, err)
		return
	}
	cp := sunat.Comprobante{Tipo: sunat.NotaCredito, Serie: serieNC, Numero: num, Fecha: hoy.Format("2006-01-02"), Hora: hoy.Format("15:04:05"),
		Emisor:  sunat.Emisor{RUC: c.RUC, RazonSocial: c.RazonSocial, Direccion: c.Direccion, Ubigeo: c.Ubigeo},
		Cliente: sunat.Cliente{TipoDoc: cliTipo, NumDoc: cliDoc, Nombre: cliNombre}, Lineas: ls,
		Referencia: fmt.Sprintf("%s-%d", serie, numero), TipoRef: tipo, MotivoCodigo: "01", Motivo: recortar(in.Motivo, 250)}
	ref := cid
	out, err := s.registrar(ctx, tx, e.ID, nil, &ref, c, cert, cp, uid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "facturacion", "nota_credito", "comprobante", cid, map[string]any{"estado": estado}, map[string]any{"nota_credito": out["numero_completo"], "motivo": in.Motivo})
	out["via"] = "nota_credito"
	out["anulado_id"] = cid
	P.JSON(w, http.StatusOK, out)
}

var reLineaCompleta = regexp.MustCompile(`<cbc:PriceAmount currencyID="PEN">([0-9.]+)</cbc:PriceAmount><cbc:PriceTypeCode>01</cbc:PriceTypeCode>.*?<cbc:TaxExemptionReasonCode>(\d+)</cbc:TaxExemptionReasonCode>.*?<cbc:Description>([^<]*)</cbc:Description>`)

// lineasComprobante reconstruye las líneas (importe con IGV y afectación) desde el XML firmado.
func lineasComprobante(doc string) []sunat.Linea {
	var out []sunat.Linea
	for _, m := range reLineaCompleta.FindAllStringSubmatch(doc, -1) {
		v, _ := strconv.ParseFloat(m[1], 64)
		af := map[string]string{"10": sunat.Gravado, "20": sunat.Exonerado, "30": sunat.Inafecto}[m[2]]
		out = append(out, sunat.Linea{Descripcion: strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(m[3]), MontoCts: int64(v*100 + 0.5), Afectacion: af})
	}
	return out
}

// listarComprobantes: GET /comprobantes?estado=&tipo=
func (s *Server) listarComprobantes(w http.ResponseWriter, r *http.Request) {
	e := edf(r)
	q := r.URL.Query()
	filas, err := db.Filas(r.Context(), s.DB, sqlComprobante+` WHERE c.edificio_id=$1 AND ($2='' OR c.estado=$2) AND ($3='' OR c.tipo=$3) ORDER BY c.id DESC LIMIT 500`,
		e.ID, q.Get("estado"), q.Get("tipo"))
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// CertDelXML: el certificado incluido en la firma (para verificar un XML guardado).
func CertDelXML(doc string) (*x509.Certificate, error) {
	i := strings.Index(doc, "<ds:X509Certificate>")
	j := strings.Index(doc, "</ds:X509Certificate>")
	if i < 0 || j < i {
		return nil, errors.New("sin certificado")
	}
	return parseCertB64(doc[i+len("<ds:X509Certificate>") : j])
}

func parseCertB64(s string) (*x509.Certificate, error) {
	der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}
