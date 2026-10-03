package app

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"edisys/api/internal/conciliacion"
	P "edisys/api/internal/plataforma"
)

// Recaudación por archivo (bloques A1 y A2): lo que comparten la recaudadora externa y el CDPG del banco.
// Las dos cosas terminan igual: un código que identifica el recibo, un monto y un código de operación
// que se convierten en pago validado (con su asiento en fondos). Nada sale a la red.

// rutasRecaudacion registra las rutas de A1 y A2. Se llama desde los dos enrutadores de servidor.go.
func (s *Server) rutasRecaudacion(r chi.Router) {
	q := s.requiere
	// A1 · recaudadora externa
	r.With(q("recaudacion.ver")).Get("/recaudadora/cuentas", s.listarCuentasRecaudadora)
	r.With(q("recaudacion.gestionar")).Post("/recaudadora/cuentas", s.crearCuentaRecaudadora)
	r.With(q("recaudacion.gestionar")).Put("/recaudadora/cuentas/{id}", s.editarCuentaRecaudadora)
	r.With(q("recaudacion.gestionar")).Post("/recaudadora/cuentas/{id}/transacciones/importar", s.importarTransaccionesRecaudadora)
	r.With(q("recaudacion.gestionar")).Post("/recaudadora/cuentas/{id}/liquidaciones/importar", s.importarLiquidacionesRecaudadora)
	r.With(q("recaudacion.ver")).Get("/recaudadora/transacciones", s.listarTransaccionesRecaudadora)
	r.With(q("recaudacion.ver")).Get("/recaudadora/liquidaciones", s.listarLiquidacionesRecaudadora)
	r.With(q("recaudacion.ver")).Get("/recaudadora/recibos", s.listarRecibosRecaudadora)
	r.With(q("recaudacion.gestionar")).Post("/recaudadora/enviar", s.enviarRecibosRecaudadora)
	r.With(q("recaudacion.ver")).Get("/recaudadora/deudas.csv", s.deudasRecaudadoraCSV)
	// A2 · CREP (descargas) y CDPG (cobranza masiva)
	r.With(q("recaudacion.ver")).Get("/crep/layouts", s.listarLayoutsCrep)
	r.With(q("recaudacion.ver")).Get("/crep", s.listarCrep)
	r.With(q("recaudacion.gestionar")).Post("/crep", s.generarCrep)
	r.With(q("recaudacion.ver")).Get("/crep/{id}/descargar", s.descargarCrep)
	r.With(q("recaudacion.ver")).Get("/cdpg", s.listarCdpg)
	r.With(q("recaudacion.gestionar")).Post("/cdpg", s.subirCdpg)
}

// reciboObjetivo es el recibo al que apunta un código de un archivo de recaudación.
type reciboObjetivo struct {
	ID       int64
	UnidadID int64
	Numero   string
	Unidad   string
}

// resolverRecibo encuentra el recibo de un código: primero por número o correlativo del recibo
// («2026-09-402», «R-000123»); si no, por el código de la unidad (su recibo con saldo más antiguo).
func resolverRecibo(ctx context.Context, tx pgx.Tx, eid int64, codigos ...string) (*reciboObjetivo, error) {
	for _, c := range codigos {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		var o reciboObjetivo
		err := tx.QueryRow(ctx, `SELECT r.id, r.unidad_id, COALESCE(r.numero,''), u.codigo FROM recibo r JOIN unidad u ON u.id=r.unidad_id
			WHERE r.edificio_id=$1 AND r.estado NOT IN ('borrador','anulado') AND (upper(r.numero)=upper($2) OR upper(r.correlativo)=upper($2))
			ORDER BY r.id DESC LIMIT 1`, eid, c).Scan(&o.ID, &o.UnidadID, &o.Numero, &o.Unidad)
		if err == nil {
			return &o, nil
		}
	}
	// Por unidad: el código del archivo puede venir con ceros a la izquierda o guiones («000402», «A-101»).
	for _, c := range codigos {
		n := conciliacion.NormalizarCodigo(c)
		if n == "" {
			continue
		}
		filas, err := tx.Query(ctx, `SELECT id, codigo FROM unidad WHERE edificio_id=$1`, eid)
		if err != nil {
			return nil, err
		}
		var uid int64
		var ucod string
		for filas.Next() {
			var id int64
			var cod string
			if err := filas.Scan(&id, &cod); err != nil {
				filas.Close()
				return nil, err
			}
			if conciliacion.NormalizarCodigo(cod) == n {
				uid, ucod = id, cod
			}
		}
		filas.Close()
		if uid == 0 {
			continue
		}
		var o reciboObjetivo
		err = tx.QueryRow(ctx, `SELECT r.id, COALESCE(r.numero,'') FROM recibo r WHERE r.unidad_id=$1 AND r.estado IN ('emitido','pagado_parcial')
			AND r.total_cts > r.pagado_cts ORDER BY r.vence NULLS LAST, r.id LIMIT 1`, uid).Scan(&o.ID, &o.Numero)
		if err != nil {
			return nil, P.Err(http.StatusUnprocessableEntity, "UNIDAD_SIN_DEUDA", "La unidad "+ucod+" no tiene recibos con saldo.")
		}
		o.UnidadID, o.Unidad = uid, ucod
		return &o, nil
	}
	return nil, P.Err(http.StatusUnprocessableEntity, "RECIBO_NO_ENCONTRADO", "Ningún recibo ni unidad con ese código.")
}

// acreditarRecibo registra un pago validado: primero cubre el recibo indicado y lo que sobra se reparte
// en los demás cargos de la unidad, del más antiguo al más nuevo (mora y saldo). Si pasa la deuda de la
// unidad no registra nada (422 MONTO_MAYOR_AL_SALDO). Asienta cada parte en fondos.
func (s *Server) acreditarRecibo(ctx context.Context, tx pgx.Tx, eid int64, obj *reciboObjetivo, usuario *int64, monto int64, medio, codigoOp, fecha string) ([]Aplicacion, error) {
	cargos, err := cargosPendientes(ctx, tx, obj.UnidadID)
	if err != nil {
		return nil, err
	}
	// El recibo pagado por código va primero; el resto conserva el orden de antigüedad.
	orden := make([]cargoPendiente, 0, len(cargos))
	for _, c := range cargos {
		if c.id == obj.ID {
			orden = append(orden, c)
		}
	}
	for _, c := range cargos {
		if c.id != obj.ID {
			orden = append(orden, c)
		}
	}
	saldos := make([]int64, len(orden))
	var deuda int64
	for i, c := range orden {
		saldos[i] = c.saldo
		deuda += c.saldo
	}
	if monto > deuda {
		return nil, P.Err(http.StatusUnprocessableEntity, "MONTO_MAYOR_AL_SALDO", "El pago pasa la deuda de la unidad ("+P.Soles(deuda)+").")
	}
	codigo := strings.TrimSpace(codigoOp)
	var cod *string
	if codigo != "" {
		cod = &codigo
	}
	fechaT, _ := conciliacionFecha(fecha)
	apls := []Aplicacion{}
	parte := 0
	for i, a := range Repartir(monto, saldos) {
		if a == 0 {
			continue
		}
		parte++
		var pid int64
		if err := tx.QueryRow(ctx, `INSERT INTO pago (edificio_id, recibo_id, monto_cts, medio, codigo_operacion, fecha, estado, registrado_por, validado_por, validado_en, parte)
			VALUES ($1,$2,$3,$4,$5,$6,'validado',$7,$7,now(),$8) RETURNING id`, eid, orden[i].id, a, medio, cod, fecha, usuario, parte).Scan(&pid); err != nil {
			if esUnico(err) {
				return nil, P.Conflicto("PAGO_DUPLICADO", "Ya hay un pago con ese código de operación.")
			}
			return nil, P.Traducir(err)
		}
		if err := s.asentarIngresoPago(ctx, tx, eid, pid, orden[i].id, a, fechaT); err != nil {
			return nil, err
		}
		apls = append(apls, Aplicacion{ReciboID: orden[i].id, Numero: orden[i].numero, Periodo: orden[i].per, Origen: orden[i].origen,
			MontoCts: a, SaldoCts: orden[i].saldo - a, PagoID: pid})
	}
	return apls, nil
}

// medioDePago traduce el medio que trae el archivo («YAPE», «Agente», «Tarjeta débito») al del pago.
func medioDePago(texto, porDefecto string) string {
	t := strings.ToLower(texto)
	switch {
	case strings.Contains(t, "yape"):
		return "yape"
	case strings.Contains(t, "plin"):
		return "plin"
	case strings.Contains(t, "tarj"), strings.Contains(t, "pos"):
		return "tarjeta"
	case strings.Contains(t, "transf"), strings.Contains(t, "banca"), strings.Contains(t, "web"), strings.Contains(t, "app"):
		return "transferencia"
	case strings.Contains(t, "efect"):
		return "efectivo"
	case strings.Contains(t, "agente"), strings.Contains(t, "ventanilla"), strings.Contains(t, "depos"):
		return "deposito"
	}
	if porDefecto == "" {
		return "deposito"
	}
	return porDefecto
}

// archivoSubido lee el campo «archivo» de un multipart (hasta 10 MB).
func archivoSubido(r *http.Request) (string, []byte, error) {
	if err := leerMultipart(r); err != nil {
		return "", nil, err
	}
	fh := firstFile(r, "archivo")
	if fh == nil {
		return "", nil, P.Validacion("Adjunta el archivo.").Campo("archivo", "Obligatorio.")
	}
	if fh.Size > MaxArchivo {
		return "", nil, P.Err(http.StatusRequestEntityTooLarge, "ARCHIVO_GRANDE", "El archivo pasa de 10 MB.")
	}
	f, err := fh.Open()
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	datos, err := io.ReadAll(io.LimitReader(f, MaxArchivo+1))
	return fh.Filename, datos, err
}

// columnaDe devuelve el índice de una cabecera (sin distinguir mayúsculas ni tildes); -1 si no está.
func columnaDe(cab []string, nombre string) int {
	n := claveCol(nombre)
	if n == "" {
		return -1
	}
	for i, c := range cab {
		if claveCol(c) == n {
			return i
		}
	}
	return -1
}

func claveCol(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", ".", "", "_", " ").Replace(s)
}

// celda lee la columna i de la fila (vacío si no existe).
func celda(fila []string, i int) string {
	if i < 0 || i >= len(fila) {
		return ""
	}
	return strings.TrimSpace(fila[i])
}

// adivinarColumna propone la cabecera cuyo nombre contiene alguna de las pistas.
func adivinarColumna(cab []string, pistas ...string) string {
	for _, p := range pistas {
		for _, c := range cab {
			if strings.Contains(claveCol(c), p) {
				return c
			}
		}
	}
	return ""
}

// conciliacionFecha normaliza una fecha del archivo a AAAA-MM-DD y la devuelve también como time.
func conciliacionFecha(s string) (time.Time, error) {
	f, err := conciliacion.Fecha(s)
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02", f)
}
