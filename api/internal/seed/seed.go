// Package seed siembra el Edificio Demo de design/DISENO.md con 6 meses de historia coherente
// (abril–setiembre 2026). Setiembre cuadra al céntimo con el diseño:
// emitido S/ 22.400 (16.800 + 4.800 + 200 + 600), cobrado S/ 19.460, egresos S/ 18.950,
// saldo S/ 510, banco S/ 34.120, morosidad 13,1 % (402 S/ 1.420, 503 S/ 760, 104 S/ 760),
// recibo del 201 = 705,60 + 196,00 + 8,40 + 80,00 = S/ 990,00, INC-014 esperando a la junta (2 de 3).
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"edisys/api/internal/archivo"
	"edisys/api/internal/auth"
	"edisys/api/internal/conciliacion"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/reparto"
)

// ClaveDemo de todos los usuarios de demostración.
const ClaveDemo = "Demo2026!"

// Opciones de la semilla.
type Opciones struct {
	// LecturasPendientes deja sin leer las últimas N unidades de setiembre (para tomarlas en vivo en la demo).
	LecturasPendientes int
}

// Periodos sembrados.
var Periodos = []string{"2026-04", "2026-05", "2026-06", "2026-07", "2026-08", "2026-09"}

var propietarios = map[string]string{
	"101": "Juan Pérez Rojas", "102": "Rosa Díaz Quispe", "103": "Carlos Mendoza Silva", "104": "Lucía Torres Vega",
	"201": "María Demo", "202": "Jorge Castillo Ramos", "203": "Ana Flores Huamán", "204": "Miguel Chávez León",
	"301": "Patricia Gutiérrez Salas", "302": "Ricardo Vargas Paredes", "303": "Sofía Ramírez Cruz", "304": "Fernando Rojas Medina",
	"401": "Gabriela Herrera Soto", "402": "Luis Alberto Campos", "403": "Carmen Delgado Ríos", "404": "Diego Morales Núñez",
	"501": "Valeria Castro Ortiz", "502": "Andrés Navarro Pinto", "503": "Elena Paredes Luna", "504": "Raúl Espinoza Arias",
	"601": "Isabel Romero Cáceres", "602": "Martín Aguilar Benites", "603": "Claudia Ruiz Zapata", "604": "Óscar Sánchez Villa",
}

// Morosos de setiembre: no pagan nada del periodo.
var morososSetiembre = map[string]bool{"402": true, "503": true, "104": true}

// Pagos tardíos (pagan al mes siguiente): dan forma a la morosidad mensual de la analítica.
var tardios = map[string]map[string]bool{
	"2026-04": {"402": true},
	"2026-05": {"402": true, "503": true},
	"2026-06": {"104": true},
	"2026-07": {"402": true, "104": true},
	"2026-08": {"402": true, "503": true},
}

var factorConsumo = map[string]float64{"2026-04": 1.08, "2026-05": 1.03, "2026-06": 0.96, "2026-07": 0.92, "2026-08": 0.97}

var presupuestoMes = map[string]int64{"2026-04": 1620000, "2026-05": 1620000, "2026-06": 1650000, "2026-07": 1650000, "2026-08": 1680000, "2026-09": 1680000}

// Presupuesto por rubro de setiembre (suma 16.800); los meses anteriores se escalan.
var presupuestoRubros = []struct {
	slug  string
	monto int64
}{{"administracion", 1050000}, {"servicios", 120000}, {"mantenimiento", 200000}, {"fondo", 310000}}

type sembrador struct {
	ctx      context.Context
	tx       pgx.Tx
	alm      archivo.Almacen
	eid      int64
	n        int
	unidades map[string]int64
	medidor  map[string]int64
	usuarios map[string]int64
	rubros   map[string]int64
	concepto map[string]int64
	recurso  map[string]int64
	periodo  map[string]int64
	codOp    int
}

func (s *sembrador) exec(sql string, args ...any) {
	if _, err := s.tx.Exec(s.ctx, sql, args...); err != nil {
		panic(fmt.Errorf("%w\nSQL: %s", err, sql))
	}
}

func (s *sembrador) id(sql string, args ...any) int64 {
	var id int64
	if err := s.tx.QueryRow(s.ctx, sql, args...).Scan(&id); err != nil {
		panic(fmt.Errorf("%w\nSQL: %s", err, sql))
	}
	return id
}

func (s *sembrador) subir(nombre, mime string, datos []byte) int64 {
	s.n++
	clave := fmt.Sprintf("e%d/semilla/%04d-%s", s.eid, s.n, nombre)
	if err := s.alm.Subir(s.ctx, clave, datos, mime); err != nil {
		panic(fmt.Errorf("subir %s al S3: %w", clave, err))
	}
	return s.id(`INSERT INTO archivo (edificio_id, clave, nombre, tipo_mime, tamano) VALUES ($1,$2,$3,$4,$5) RETURNING id`, s.eid, clave, nombre, mime, len(datos))
}

func (s *sembrador) foto(nombre string, fondo color.RGBA, titulo string, lineas ...string) int64 {
	return s.subir(nombre+".png", "image/png", imagen(fondo, titulo, lineas...))
}

func lima(fecha string, hora string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", fecha+" "+hora, P.Lima)
	if err != nil {
		panic(err)
	}
	return t
}

// Sembrar borra todo y deja el Edificio Demo listo.
func Sembrar(ctx context.Context, pool *pgxpool.Pool, alm archivo.Almacen, op Opciones) (res map[string]any, err error) {
	hash, err := auth.HashClave(ClaveDemo)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("semilla: %v", v)
		}
	}()
	s := &sembrador{ctx: ctx, tx: tx, alm: alm, unidades: map[string]int64{}, medidor: map[string]int64{}, usuarios: map[string]int64{},
		rubros: map[string]int64{}, concepto: map[string]int64{}, recurso: map[string]int64{}, periodo: map[string]int64{}}

	s.exec(`TRUNCATE chatbot_sesion, comprobante, comprobante_serie, facturacion_config, movimiento_banco, extracto, banco_mapeo, correo_adjunto, correo_mensaje, ajuste, whatsapp_mensaje, whatsapp_config, voto, incidencia_evidencia, incidencia_evento, incidencia, junta_miembro,
		reparto_medidor, recibo_general, lectura, medidor, reserva, recurso, area, egreso, pago, recibo_linea, recibo, presupuesto, periodo,
		concepto, rubro, importacion, deuda_inicial, unidad_persona, persona, unidad, auditoria, invitacion, sesion_refresh,
		usuario_edificio_rol, rol_permiso_edificio, usuario, edificio, archivo, administradora, contacto, motor_consulta, motor_golden_sql RESTART IDENTITY CASCADE`)
	s.exec(`ALTER SEQUENCE recibo_correlativo_seq RESTART WITH 100`)

	adm := s.id(`INSERT INTO administradora (nombre, ruc) VALUES ('Demo Administraciones SAC', '20600000001') RETURNING id`)
	s.eid = s.id(`INSERT INTO edificio (administradora_id, nombre, direccion, distrito, dia_corte, dias_vencimiento, dias_gracia, cobra_agua,
		umbral_aprobacion_cts, modo_aprobacion, yape_numero, normas_texto)
		VALUES ($1, 'Edificio Demo', 'Av. José Larco 1234', 'Miraflores, Lima', 1, 9, 15, true, 100000, 'mayoria', '987 654 321',
		'Silencio de 22:00 a 7:00. Mascotas con correa en áreas comunes. La basura se baja de 19:00 a 21:00. Las visitas se anuncian en portería.') RETURNING id`, adm)

	// --- Usuarios de demostración (clave Demo2026!).
	type us struct{ correo, nombre, tel, rol string }
	lista := []us{
		{"admin@demo.pe", "Ana Administradora", "51999000001", "administrador"},
		{"junta@demo.pe", "Jorge Junta", "51999000002", "junta"},
		{"junta2@demo.pe", "Carmen Salazar", "51999000003", "junta"},
		{"junta3@demo.pe", "Luis Fernández", "51999000004", "junta"},
		{"junta4@demo.pe", "Rosa Quispe", "51999000005", "junta"},
		{"junta5@demo.pe", "Pedro Huamán", "51999000006", "junta"},
		{"propietario201@demo.pe", "María Demo", "51900000201", "propietario"},
		{"inquilino@demo.pe", "Iván Inquilino", "51911000302", "inquilino"},
		{"operario@demo.pe", "Óscar Operario", "51999000007", "operario"},
		{"tecnico@demo.pe", "Tomás Técnico", "51999000008", "tecnico"},
		{"supervisor@demo.pe", "Sofía Supervisora", "51999000009", "superadmin"},
	}
	for _, u := range lista {
		id := s.id(`INSERT INTO usuario (administradora_id, correo, nombre, telefono, clave_hash, es_superadmin) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			adm, u.correo, u.nombre, u.tel, hash, u.rol == "superadmin")
		s.usuarios[u.correo] = id
		s.exec(`INSERT INTO usuario_edificio_rol (usuario_id, edificio_id, rol) VALUES ($1,$2,$3)`, id, s.eid, u.rol)
	}
	for i, c := range []string{"junta@demo.pe", "junta2@demo.pe", "junta3@demo.pe", "junta4@demo.pe", "junta5@demo.pe"} {
		cargo := []string{"presidente", "vicepresidenta", "tesorero", "secretaria", "vocal"}[i]
		s.exec(`INSERT INTO junta_miembro (edificio_id, usuario_id, cargo, presidente) VALUES ($1,$2,$3,$4)`, s.eid, s.usuarios[c], cargo, i == 0)
	}

	// --- Unidades, propietarios, inquilino y medidores.
	for i, c := range reparto.CodigosDemo {
		piso := int(c[0] - '0')
		part := float64(reparto.ParticipacionDemo[c]) / 10000
		uid := s.id(`INSERT INTO unidad (edificio_id, codigo, tipo, piso, participacion_pct, alquilado) VALUES ($1,$2,'departamento',$3,$4,$5) RETURNING id`,
			s.eid, c, piso, part, c == "302")
		s.unidades[c] = uid
		var usuario *int64
		correo := "propietario" + c + "@demo.pe" // Mailpit en local: nada sale a terceros
		if c == "201" {
			v := s.usuarios["propietario201@demo.pe"]
			usuario = &v
			correo = "propietario201@demo.pe"
		}
		pid := s.id(`INSERT INTO persona (edificio_id, nombre, dni_ruc, correo, celular, usuario_id) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			s.eid, propietarios[c], "40000"+c, correo, "900000"+c, usuario)
		s.exec(`INSERT INTO unidad_persona (unidad_id, persona_id, rol, desde) VALUES ($1,$2,'propietario','2023-01-15')`, uid, pid)
		if c == "302" {
			inq := s.usuarios["inquilino@demo.pe"]
			ip := s.id(`INSERT INTO persona (edificio_id, nombre, dni_ruc, correo, celular, usuario_id) VALUES ($1,'Iván Inquilino','41000302','inquilino@demo.pe','911000302',$2) RETURNING id`, s.eid, inq)
			s.exec(`INSERT INTO unidad_persona (unidad_id, persona_id, rol, desde) VALUES ($1,$2,'inquilino','2026-02-01')`, uid, ip)
			s.exec(`UPDATE unidad SET permisos_inquilino='{"reservar": true, "reportar": true, "ver_recibos": false}' WHERE id=$1`, uid)
		}
		if c == "201" {
			// Historial: la unidad tuvo otra dueña antes que María (RF-02).
			ant := s.id(`INSERT INTO persona (edificio_id, nombre, dni_ruc, celular) VALUES ($1,'Beatriz Antigua Soto','40999201','900999201') RETURNING id`, s.eid)
			s.exec(`INSERT INTO unidad_persona (unidad_id, persona_id, rol, desde, hasta) VALUES ($1,$2,'propietario','2019-03-01','2023-01-14')`, uid, ant)
		}
		s.medidor[c] = s.id(`INSERT INTO medidor (edificio_id, unidad_id, tipo, serie, orden_ronda, lectura_inicial) VALUES ($1,$2,'agua',$3,$4,$5::numeric) RETURNING id`,
			s.eid, uid, "AG-"+c, i+1, P.TextoMilesimas(1000000+int64(i)*37500))
	}
	s.exec(`INSERT INTO medidor (edificio_id, unidad_id, tipo, serie, orden_ronda, lectura_inicial) VALUES ($1,NULL,'agua','SEDAPAL-GENERAL',0,50000)`, s.eid)

	// --- Rubros y conceptos.
	rubros := []struct {
		slug, nombre string
		conceptos    [][2]string
	}{
		{"administracion", "Administración", [][2]string{{"conserjeria", "Conserjería"}, {"limpieza", "Limpieza"}, {"administrador", "Administrador"}}},
		{"servicios", "Servicios básicos", [][2]string{{"agua", "Agua (Sedapal)"}, {"luz", "Luz de áreas comunes"}, {"internet", "Internet de áreas sociales"}}},
		{"mantenimiento", "Mantenimiento preventivo", [][2]string{{"ascensores", "Mantenimiento de ascensores"}, {"fumigacion", "Fumigación"}}},
		{"correctivo", "Mantenimiento correctivo", [][2]string{{"reparaciones", "Reparaciones"}}},
		{"fondo", "Fondo de contingencia", [][2]string{{"aporte", "Aporte al fondo"}}},
	}
	for i, r := range rubros {
		rid := s.id(`INSERT INTO rubro (edificio_id, slug, nombre, orden) VALUES ($1,$2,$3,$4) RETURNING id`, s.eid, r.slug, r.nombre, i+1)
		s.rubros[r.slug] = rid
		for j, c := range r.conceptos {
			s.concepto[r.slug+"."+c[0]] = s.id(`INSERT INTO concepto (rubro_id, slug, nombre, orden) VALUES ($1,$2,$3,$4) RETURNING id`, rid, c[0], c[1], j+1)
		}
	}

	// --- Áreas comunes.
	type area struct {
		nombre, slug   string
		tarifa         int64
		franjas        string
		aforo          int
		incluye, norma string
		recursos       []string
	}
	for _, a := range []area{
		{"Parrillas", "parrillas", 8000, `[{"inicio":"12:00","fin":"17:00"},{"inicio":"18:00","fin":"23:00"}]`, 15,
			"Parrilla, mesa para 10 personas, lavadero y punto de luz.", "Deja la parrilla limpia y el carbón apagado. Música a volumen moderado hasta las 22:00.", []string{"Parrilla 1", "Parrilla 2"}},
		{"Salón de usos múltiples", "sum", 20000, `[{"inicio":"10:00","fin":"14:00"},{"inicio":"15:00","fin":"19:00"},{"inicio":"19:00","fin":"23:00"}]`, 40,
			"40 sillas, 6 mesas, cocina y baño.", "Entrega el salón limpio. Prohibido el uso de confeti y pirotecnia.", []string{"SUM"}},
		{"Piscina", "piscina", 0, `[{"inicio":"08:00","fin":"12:00"},{"inicio":"12:00","fin":"16:00"},{"inicio":"16:00","fin":"20:00"}]`, 20,
			"Piscina temperada y 8 perezosas.", "Ducha antes de entrar. Niños siempre con un adulto. Sin vidrio en la zona de la piscina.", []string{"Piscina"}},
	} {
		aid := s.id(`INSERT INTO area (edificio_id, nombre, slug, tarifa_cts, franjas, aforo, incluye, normas) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			s.eid, a.nombre, a.slug, a.tarifa, a.franjas, a.aforo, a.incluye, a.norma)
		for _, r := range a.recursos {
			s.recurso[r] = s.id(`INSERT INTO recurso (area_id, nombre) VALUES ($1,$2) RETURNING id`, aid, r)
		}
	}

	// --- Reservas (antes que los recibos: el disparador de morosos mira la deuda vencida al insertar).
	type reserva struct {
		codigo, recurso, unidad, fecha, ini, fin string
	}
	var reservas []reserva
	hist := []struct{ per, rec, uni, dia, ini, fin string }{
		{"2026-04", "Parrilla 1", "103", "11", "12:00", "17:00"}, {"2026-04", "SUM", "304", "18", "15:00", "19:00"},
		{"2026-05", "Parrilla 2", "202", "09", "18:00", "23:00"}, {"2026-05", "Parrilla 1", "401", "16", "12:00", "17:00"}, {"2026-05", "SUM", "603", "23", "19:00", "23:00"},
		{"2026-06", "Parrilla 1", "204", "13", "12:00", "17:00"}, {"2026-06", "SUM", "501", "20", "15:00", "19:00"},
		{"2026-07", "Parrilla 2", "101", "04", "18:00", "23:00"}, {"2026-07", "Parrilla 1", "303", "18", "12:00", "17:00"}, {"2026-07", "SUM", "604", "25", "15:00", "19:00"},
		{"2026-08", "Parrilla 1", "203", "08", "12:00", "17:00"}, {"2026-08", "Parrilla 2", "403", "15", "18:00", "23:00"}, {"2026-08", "SUM", "601", "22", "19:00", "23:00"},
	}
	for i, h := range hist {
		reservas = append(reservas, reserva{fmt.Sprintf("R-%04d", 390+i), h.rec, h.uni, h.per + "-" + h.dia, h.ini, h.fin})
	}
	reservas = append(reservas,
		reserva{"R-0407", "Piscina", "303", "2026-09-27", "12:00", "16:00"},
		reserva{"R-0408", "Parrilla 2", "102", "2026-09-05", "18:00", "23:00"},
		reserva{"R-0409", "Parrilla 1", "203", "2026-09-06", "12:00", "17:00"},
		reserva{"R-0410", "SUM", "301", "2026-09-12", "15:00", "19:00"},
		reserva{"R-0411", "Parrilla 2", "404", "2026-09-13", "18:00", "23:00"},
		reserva{"R-0412", "Parrilla 1", "201", "2026-09-19", "12:00", "17:00"},
		reserva{"R-0413", "Parrilla 1", "601", "2026-09-20", "18:00", "23:00"},
		reserva{"R-0414", "SUM", "602", "2026-10-01", "15:00", "19:00"},
		reserva{"R-0415", "Parrilla 2", "302", "2026-10-03", "18:00", "23:00"},
		reserva{"R-0416", "Piscina", "101", "2026-10-04", "12:00", "16:00"},
	)
	reservaID := map[string]int64{}
	reservasPorMes := map[string][]string{} // periodo → códigos (cargadas al recibo)
	for _, r := range reservas {
		var tarifa int64
		if err := tx.QueryRow(ctx, `SELECT a.tarifa_cts FROM recurso rc JOIN area a ON a.id=rc.area_id WHERE rc.id=$1`, s.recurso[r.recurso]).Scan(&tarifa); err != nil {
			return nil, err
		}
		usuario := (*int64)(nil)
		if r.unidad == "201" {
			v := s.usuarios["propietario201@demo.pe"]
			usuario = &v
		}
		id := s.id(`INSERT INTO reserva (edificio_id, recurso_id, unidad_id, usuario_id, codigo, inicio, fin, estado, total_cts, modo_cobro, acepta_normas, creado_en)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'confirmada',$8,'cargo_recibo',true,$9) RETURNING id`,
			s.eid, s.recurso[r.recurso], s.unidades[r.unidad], usuario, r.codigo, lima(r.fecha, r.ini), lima(r.fecha, r.fin), tarifa, lima(r.fecha, r.ini).AddDate(0, 0, -6))
		reservaID[r.codigo] = id
		if tarifa > 0 {
			per := r.fecha[:7]
			reservasPorMes[per] = append(reservasPorMes[per], r.codigo)
		}
	}
	s.exec(`SELECT setval('reserva_codigo_seq', 416)`)

	// --- Periodos: presupuesto, lecturas con foto, recibo general, reparto, recibos, pagos y egresos.
	lecturaPrev := map[string]int64{}
	for i, c := range reparto.CodigosDemo {
		lecturaPrev[c] = 1000000 + int64(i)*37500
	}
	resumen := map[string]any{}
	for _, per := range Periodos {
		ini, _ := P.RangoPeriodo(per)
		pid := s.id(`INSERT INTO periodo (edificio_id, periodo, fecha_corte, estado, creado_en) VALUES ($1,$2,$3,'emitido',$4) RETURNING id`, s.eid, per, per+"-01", ini)
		s.periodo[per] = pid
		escala := float64(presupuestoMes[per]) / 1680000
		var acum int64
		for j, pr := range presupuestoRubros {
			m := int64(math.Round(float64(pr.monto) * escala))
			if j == len(presupuestoRubros)-1 {
				m = presupuestoMes[per] - acum
			}
			acum += m
			s.exec(`INSERT INTO presupuesto (periodo_id, rubro_id, monto_cts) VALUES ($1,$2,$3)`, pid, s.rubros[pr.slug], m)
		}

		// Consumo del mes.
		consumo := map[string]int64{}
		var montoGeneral, litrosGeneral int64
		if per == "2026-09" {
			consumo = reparto.ConsumoSetiembreDemo
			montoGeneral, litrosGeneral = reparto.SedapalSetiembreCts, reparto.SedapalSetiembreLitros
		} else {
			f := factorConsumo[per]
			var suma int64
			for _, c := range reparto.CodigosDemo {
				base := reparto.ConsumoSetiembreDemo[c]
				switch c {
				case "402":
					base = 13000
				case "104":
					base = 11500
				case "503":
					base = 11000
				}
				consumo[c] = int64(math.Round(float64(base) * f))
				suma += consumo[c]
			}
			litrosGeneral = suma + int64(math.Round(14286*f))
			montoGeneral = int64(math.Round(float64(litrosGeneral) * 1.4))
		}
		mesNombre := P.NombrePeriodo(per)
		fotoGeneral := s.foto("sedapal-"+per, colorRecibo, "SEDAPAL · Recibo de agua", mesNombre, "Edificio Demo · Suministro 4471203",
			"Consumo: "+strings.Replace(P.TextoMilesimas(litrosGeneral), ".", ",", 1)+" m3", "Total a pagar: "+P.Soles(montoGeneral))
		s.exec(`INSERT INTO recibo_general (periodo_id, tipo, monto_cts, consumo_total, foto_id, registrado_por, creado_en) VALUES ($1,'agua',$2,$3::numeric,$4,$5,$6)`,
			pid, montoGeneral, P.TextoMilesimas(litrosGeneral), fotoGeneral, s.usuarios["admin@demo.pe"], lima(per+"-02", "10:00"))

		// Lecturas (con foto: regla dura).
		fechaLectura := per + "-01"
		for idx, c := range reparto.CodigosDemo {
			if per == "2026-09" && op.LecturasPendientes > 0 && idx >= len(reparto.CodigosDemo)-op.LecturasPendientes {
				continue
			}
			ant := lecturaPrev[c]
			val := ant + consumo[c]
			fid := s.foto(fmt.Sprintf("medidor-%s-%s", c, per), colorMedidor, "Medidor AG-"+c+" · Dpto "+c, mesNombre,
				"Lectura: "+strings.Replace(P.TextoMilesimas(val), ".", ",", 1)+" m3", "Tomada por el operario")
			var alerta *string
			if per == "2026-09" && c == "402" {
				a := "PICO"
				alerta = &a
			}
			s.exec(`INSERT INTO lectura (medidor_id, periodo_id, valor, anterior, consumo, foto_id, tomada_en, subida_en, operario_id, alerta)
				VALUES ($1,$2,$3::numeric,$4::numeric,$5::numeric,$6,$7,$7,$8,$9)`, s.medidor[c], pid, P.TextoMilesimas(val), P.TextoMilesimas(ant),
				P.TextoMilesimas(consumo[c]), fid, lima(fechaLectura, fmt.Sprintf("%02d:%02d", 8+idx/6, (idx%6)*9)), s.usuarios["operario@demo.pe"], alerta)
			lecturaPrev[c] = val
		}
		rep, err := reparto.Medidores(montoGeneral, litrosGeneral, reparto.UnidadesDemo(consumo))
		if err != nil {
			return nil, fmt.Errorf("reparto %s: %w", per, err)
		}
		lineasRep, _ := json.Marshal(rep.Lineas)
		// Las unidades de UnidadesDemo usan ids 1..24 en orden; se reemplazan por los reales.
		var ls []reparto.Linea
		_ = json.Unmarshal(lineasRep, &ls)
		for i := range ls {
			ls[i].UnidadID = s.unidades[ls[i].Unidad]
		}
		lineasRep, _ = json.Marshal(ls)
		s.exec(`INSERT INTO reparto_medidor (periodo_id, tipo, tarifa_cts_x_1000, total_unidades_cts, diferencia_cts, lineas, aprobado_por, aprobado_en)
			VALUES ($1,'agua',$2,$3,$4,$5,$6,$7)`, pid, rep.TarifaCtsX1000, rep.TotalUnidadesCts, rep.DiferenciaCts, lineasRep, s.usuarios["admin@demo.pe"], lima(per+"-01", "18:00"))
		agua := map[string]reparto.Linea{}
		for _, l := range ls {
			agua[l.Unidad] = l
		}

		// Recibos.
		pesos := make([]int64, len(reparto.CodigosDemo))
		for i, c := range reparto.CodigosDemo {
			pesos[i] = reparto.ParticipacionDemo[c]
		}
		cuotas := reparto.PorPesos(presupuestoMes[per], pesos)
		reservasUnidad := map[string][]string{}
		for _, cod := range reservasPorMes[per] {
			for _, r := range reservas {
				if r.codigo == cod {
					reservasUnidad[r.unidad] = append(reservasUnidad[r.unidad], cod)
				}
			}
		}
		var emitido, cobrado int64
		for i, c := range reparto.CodigosDemo {
			type linea struct {
				tipo, desc string
				monto      int64
				reserva    *int64
			}
			ls := []linea{{"cuota", "Cuota de mantenimiento", cuotas[i], nil},
				{"agua", "Agua (consumo propio " + strings.Replace(agua[c].Consumo, ".", ",", 1) + " m³)", agua[c].PropioCts, nil},
				{"agua_comun", "Áreas comunes (agua)", agua[c].ComunCts, nil}}
			for _, cod := range reservasUnidad[c] {
				var monto int64
				var rec string
				for _, r := range reservas {
					if r.codigo == cod {
						rec = r.recurso
					}
				}
				_ = tx.QueryRow(ctx, `SELECT total_cts FROM reserva WHERE codigo=$1`, cod).Scan(&monto)
				id := reservaID[cod]
				ls = append(ls, linea{"reserva", "Reserva " + cod + " " + rec, monto, &id})
			}
			var total int64
			for _, l := range ls {
				total += l.monto
			}
			rid := s.id(`INSERT INTO recibo (edificio_id, periodo_id, unidad_id, numero, correlativo, estado, total_cts, emitido_en, vence, enviado_en)
				VALUES ($1,$2,$3,$4,'R-' || lpad(nextval('recibo_correlativo_seq')::text, 6, '0'),'emitido',$5,$6,$7,$6) RETURNING id`,
				s.eid, pid, s.unidades[c], per+"-"+c, total, lima(per+"-01", "09:00"), per+"-10")
			for k, l := range ls {
				s.exec(`INSERT INTO recibo_linea (recibo_id, tipo, descripcion, monto_cts, orden, reserva_id) VALUES ($1,$2,$3,$4,$5,$6)`, rid, l.tipo, l.desc, l.monto, k+1, l.reserva)
			}
			emitido += total
			// Pagos.
			if per == "2026-09" && morososSetiembre[c] {
				if c == "503" {
					v := s.foto("voucher-503-set", colorVoucher, "YAPE · Pago enviado", "A: Edificio Demo", "Monto: S/ 300,00", "Op. 77120503 · 27/09/2026")
					s.exec(`INSERT INTO pago (edificio_id, recibo_id, monto_cts, medio, codigo_operacion, fecha, voucher_id, estado, registrado_por)
						VALUES ($1,$2,30000,'yape','77120503','2026-09-27',$3,'pendiente_validacion',$4)`, s.eid, rid, v, s.usuarios["admin@demo.pe"])
				}
				continue
			}
			dia := 2 + (i*3)%9
			fecha := fmt.Sprintf("%s-%02d", per, dia)
			if tardios[per][c] {
				t, _ := time.Parse("2006-01", per)
				fecha = t.AddDate(0, 1, 0).Format("2006-01") + fmt.Sprintf("-%02d", 3+i%10)
			}
			medios := []string{"yape", "transferencia", "plin", "deposito", "efectivo"}
			medio := medios[i%len(medios)]
			var codigo *string
			if medio != "efectivo" {
				s.codOp++
				v := fmt.Sprintf("%08d", 30000000+s.codOp*7919)
				codigo = &v
			}
			var voucher *int64
			if per == "2026-09" {
				v := s.foto("voucher-"+c+"-set", colorVoucher, strings.ToUpper(medio)+" · Constancia", "A: Edificio Demo", "Monto: "+P.Soles(total),
					"Op. "+valor(codigo)+" · "+fecha, "Dpto "+c+" set-2026")
				voucher = &v
			}
			s.exec(`INSERT INTO pago (edificio_id, recibo_id, monto_cts, medio, codigo_operacion, fecha, voucher_id, estado, registrado_por, validado_por, validado_en)
				VALUES ($1,$2,$3,$4,$5,$6,$7,'validado',$8,$8,$9)`, s.eid, rid, total, medio, codigo, fecha, voucher, s.usuarios["admin@demo.pe"], lima(fecha, "19:00"))
			cobrado += total
		}

		// Egresos del mes.
		type egreso struct {
			rubro, concepto, desc string
			monto                 int64
			doc                   string // pdf | foto | "" (sin sustento) | sedapal
		}
		luz := map[string]int64{"2026-04": 101500, "2026-05": 99000, "2026-06": 95500, "2026-07": 94000, "2026-08": 96500, "2026-09": 98000}[per]
		egs := []egreso{
			{"administracion", "conserjeria", "Conserjería " + P.NombreMes(ini.Month()), 630000, "pdf"},
			{"administracion", "limpieza", "Limpieza " + P.NombreMes(ini.Month()), 280000, "foto"},
			{"administracion", "administrador", "Administrador " + P.NombreMes(ini.Month()), 140000, "pdf"},
			{"servicios", "agua", "Sedapal " + P.NombreMes(ini.Month()), montoGeneral, "sedapal"},
			{"servicios", "luz", "Luz de áreas comunes", luz, "foto"},
			{"servicios", "internet", "Internet de áreas sociales", 17000, "foto"},
			{"mantenimiento", "ascensores", "Mantenimiento de ascensores", 110000, "pdf"},
			{"fondo", "aporte", "Aporte al fondo de contingencia", 70000, "foto"},
		}
		if per == "2026-09" || per == "2026-06" || per == "2026-04" {
			doc := "foto"
			if per == "2026-09" {
				doc = "" // sin sustento: lo marca el balance
			}
			egs = append(egs, egreso{"mantenimiento", "fumigacion", "Fumigación", 50000, doc})
		}
		for k, e := range egs {
			fecha := fmt.Sprintf("%s-%02d", per, 5+k*2)
			var doc *int64
			tipo := "foto"
			switch e.doc {
			case "pdf":
				v := s.subir(slug(e.desc)+".pdf", "application/pdf", facturaPDF(proveedorDe(e.concepto), e.desc, P.Soles(e.monto), fecha))
				doc, tipo = &v, "pdf"
			case "foto":
				v := s.foto(slug(e.desc)+"-"+per, colorRecibo, e.desc, mesNombre, "Monto: "+P.Soles(e.monto), "Pagado el "+fecha)
				doc = &v
			case "sedapal":
				doc = &fotoGeneral
			}
			s.exec(`INSERT INTO egreso (edificio_id, periodo, rubro_id, concepto_id, descripcion, monto_cts, fecha, documento_id, tipo_documento, registrado_por)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, s.eid, per, s.rubros[e.rubro], s.concepto[e.rubro+"."+e.concepto], e.desc, e.monto, fecha, doc, tipo, s.usuarios["admin@demo.pe"])
		}
		resumen[per] = map[string]any{"emitido_cts": emitido, "cobrado_en_fecha_cts": cobrado, "agua_general_cts": montoGeneral}
	}

	// --- Mantenimiento: 14 incidencias con su historia.
	if err := s.incidencias(); err != nil {
		return nil, err
	}

	// --- WhatsApp: configuración simulada y algunos mensajes de ejemplo.
	s.exec(`INSERT INTO whatsapp_config (edificio_id, modo) VALUES ($1,'simulado')`, s.eid)
	// GoldenSQL de arranque del motor (F8): preguntas que el chatbot por reglas
	// no atiende. Los números salen de este mismo seed (los prueba el smoke).
	s.exec(`INSERT INTO motor_golden_sql (edificio_id, pregunta, sql, tablas, fuente) VALUES
		($1, '¿cuál es la morosidad del edificio?',
		 'SELECT u.edificio_id, COALESCE(ROUND(100.0*SUM(r.total_cts-r.pagado_cts)/NULLIF(SUM(r.total_cts),0),1),0) AS pct, COALESCE(SUM(r.total_cts-r.pagado_cts),0) AS saldo_cts FROM recibo r JOIN unidad u ON u.id=r.unidad_id WHERE u.edificio_id = :edificio_id AND r.estado NOT IN (''borrador'',''anulado'') GROUP BY u.edificio_id', '{recibo,unidad}', 'manual'),
		($1, '¿cuánto debe en total el dpto 402?',
		 'SELECT u.codigo, COALESCE(SUM(r.total_cts-r.pagado_cts),0) AS saldo_cts FROM unidad u JOIN recibo r ON r.unidad_id=u.id WHERE u.edificio_id = :edificio_id AND u.codigo=''402'' AND r.estado NOT IN (''borrador'',''anulado'') GROUP BY u.codigo', '{unidad,recibo}', 'manual'),
		($1, 'deuda por unidad',
		 'SELECT u.codigo, COALESCE(SUM(r.total_cts-r.pagado_cts),0) AS saldo_cts FROM unidad u LEFT JOIN recibo r ON r.unidad_id=u.id AND r.estado NOT IN (''borrador'',''anulado'') WHERE u.edificio_id = :edificio_id GROUP BY u.codigo ORDER BY saldo_cts DESC', '{unidad,recibo}', 'manual')`, s.eid)

	msgs := []struct{ uni, tel, plantilla, texto, estado, dir, origen, intencion, cuando string }{
		{"201", "51900000201", "recibo", "Hola María, tu recibo de agosto 2026 del Dpto 201 es de S/ 962,50 y vence el 10/08/2026. Ya está pagado, ¡gracias!", "simulado", "saliente", "recibo", "", "2026-08-01 09:30"},
		{"402", "51900000402", "recordatorio_deuda", "Hola Luis, el Dpto 402 tiene un saldo pendiente de S/ 1.420,00. Puedes pagar por Yape al 987 654 321 o por transferencia y enviarnos el voucher desde la app. Si ya pagaste, no tomes en cuenta este mensaje. ¡Gracias!", "simulado", "saliente", "manual", "", "2026-09-26 10:00"},
		{"201", "51900000201", "entrante", "Hola, ¿a qué hora cierra la piscina?", "recibido", "entrante", "webhook", "horarios", "2026-09-27 16:05"},
		{"201", "51900000201", "chatbot", "Horarios de las áreas comunes:\n• Piscina: 08:00 a 12:00 y 12:00 a 16:00 y 16:00 a 20:00. Aforo: 20 personas.", "simulado", "saliente", "chatbot", "horarios", "2026-09-27 16:05"},
	}
	for _, m := range msgs {
		uid := s.unidades[m.uni]
		s.exec(`INSERT INTO whatsapp_mensaje (edificio_id, direccion, unidad_id, telefono, plantilla, texto, estado, origen, intencion, creado_en, procesado_en)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)`, s.eid, m.dir, uid, m.tel, m.plantilla, m.texto, m.estado, m.origen, m.intencion, lima(m.cuando[:10], m.cuando[11:]))
	}

	// --- Banco: el saldo inicial se ajusta para que el acumulado a setiembre sea S/ 34.120,00.
	s.exec(`UPDATE edificio SET saldo_inicial_cts = 3412000 - (
		COALESCE((SELECT SUM(pg.monto_cts) FROM pago pg JOIN recibo r ON r.id=pg.recibo_id JOIN periodo p ON p.id=r.periodo_id
		          WHERE pg.edificio_id=$1 AND pg.estado='validado' AND p.periodo <= '2026-09'),0)
		- COALESCE((SELECT SUM(monto_cts) FROM egreso WHERE edificio_id=$1 AND periodo <= '2026-09'),0)) WHERE id=$1`, s.eid)

	// --- Facturación electrónica en modo simulado (no sale nada a SUNAT). RUC de demostración válido.
	s.exec(`INSERT INTO facturacion_config (edificio_id, ruc, razon_social, direccion, ubigeo, modo, actualizado_por)
		VALUES ($1,'20600000005','Junta de Propietarios del Edificio Demo','Av. José Larco 1234, Miraflores, Lima','150122','simulado',$2)`, s.eid, s.usuarios["admin@demo.pe"])

	// --- Conciliación: extracto de setiembre del BCP ya cargado (2 movimientos sin pareja a propósito).
	adminID := s.usuarios["admin@demo.pe"]
	if _, err := conciliacion.CargarDemo(ctx, tx, s.eid, "2026-09", 3412000, &adminID); err != nil {
		return nil, fmt.Errorf("extracto demo: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	slog.Info("semilla lista", "edificio", s.eid, "archivos", s.n)
	resumen["edificio_id"] = s.eid
	resumen["archivos"] = s.n
	resumen["usuarios"] = len(lista)
	return resumen, nil
}

func valor(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

func slug(s string) string {
	s = strings.ToLower(s)
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", " ", "-", "(", "", ")", "", ".", "")
	return r.Replace(s)
}

func proveedorDe(concepto string) string {
	switch concepto {
	case "conserjeria":
		return "Vigilancia y Conserjería Lima SAC"
	case "administrador":
		return "Demo Administraciones SAC"
	case "ascensores":
		return "Ascensores Andinos SAC"
	}
	return "Proveedor Demo SAC"
}

// incidencias siembra INC-001 … INC-014 con línea de tiempo, evidencias, votos y egresos de los terminados.
func (s *sembrador) incidencias() error {
	admin, op, tec, maria, inq := s.usuarios["admin@demo.pe"], s.usuarios["operario@demo.pe"], s.usuarios["tecnico@demo.pe"], s.usuarios["propietario201@demo.pe"], s.usuarios["inquilino@demo.pe"]
	junta := []int64{s.usuarios["junta@demo.pe"], s.usuarios["junta2@demo.pe"], s.usuarios["junta3@demo.pe"], s.usuarios["junta4@demo.pe"], s.usuarios["junta5@demo.pe"]}
	type paso struct {
		estado, fecha, nota string
		quien               int64
	}
	type inc struct {
		titulo, desc, ubic, cat, crit, estado, creado string
		reporta                                       int64
		unidad                                        string
		presup, costo                                 int64
		proveedor                                     string
		pasos                                         []paso
		votos                                         []string // aprueba/rechaza por miembro en orden
		motivo                                        string
	}
	lista := []inc{
		{"Fuga en la cisterna", "Se escucha agua corriendo en el cuarto de la cisterna.", "Cuarto de bombas", "gasfiteria", "critica", "terminado", "2026-04-03 07:40", op, "", 90000, 95000, "Gasfitería Rápida EIRL",
			[]paso{{"validado", "2026-04-03 09:00", "Crítica: se pierde agua.", admin}, {"presupuestado", "2026-04-04 11:00", "Cambio de válvula flotadora.", admin}, {"aprobado", "2026-04-04 15:00", "Dentro del umbral: aprueba la administración.", admin}, {"en_ejecucion", "2026-04-07 08:00", "", tec}, {"terminado", "2026-04-09 17:00", "Válvula cambiada, sin fuga.", tec}}, nil, ""},
		{"Luminaria del pasadizo piso 3", "El foco del pasadizo del tercer piso parpadea.", "Piso 3 – pasadizo", "electricidad", "baja", "terminado", "2026-04-15 20:10", op, "", 12000, 12000, "Electricidad Hogar",
			[]paso{{"validado", "2026-04-16 09:00", "", admin}, {"presupuestado", "2026-04-16 12:00", "", admin}, {"aprobado", "2026-04-16 12:30", "", admin}, {"en_ejecucion", "2026-04-18 10:00", "", tec}, {"terminado", "2026-04-18 11:30", "Luminaria LED nueva.", tec}}, nil, ""},
		{"Puerta del garaje hace ruido", "La puerta levadiza chirría y se traba al subir.", "Garaje", "otros", "media", "terminado", "2026-05-06 08:15", maria, "201", 38000, 38000, "Puertas Automáticas Lima",
			[]paso{{"validado", "2026-05-06 10:00", "", admin}, {"presupuestado", "2026-05-08 16:00", "Lubricación y cambio de rodamientos.", admin}, {"aprobado", "2026-05-09 09:00", "", admin}, {"en_ejecucion", "2026-05-18 09:00", "", tec}, {"terminado", "2026-05-20 13:00", "", tec}}, nil, ""},
		{"Ascensor se detiene en el piso 4", "El ascensor se queda parado entre el 4 y el 5.", "Ascensor principal", "ascensores", "critica", "terminado", "2026-05-22 19:30", op, "", 145000, 145000, "Ascensores Andinos SAC",
			[]paso{{"validado", "2026-05-23 08:30", "Crítica: gente atrapada dos veces.", admin}, {"presupuestado", "2026-05-25 12:00", "Cambio de tarjeta de control.", admin}, {"aprobado", "2026-05-27 20:00", "Decisión de la junta: 3 a favor, 0 en contra", junta[2]}, {"en_ejecucion", "2026-06-02 09:00", "", tec}, {"terminado", "2026-06-05 18:00", "Tarjeta reemplazada, pruebas OK.", tec}}, []string{"aprueba", "aprueba", "aprueba"}, ""},
		{"Filtración en la azotea", "Mancha de humedad en el techo del 604 cuando llueve.", "Azotea", "estructura", "media", "rechazado", "2026-06-10 11:00", admin, "604", 320000, 0, "Impermeabilizaciones Perú",
			[]paso{{"validado", "2026-06-10 12:00", "", admin}, {"presupuestado", "2026-06-15 10:00", "Impermeabilización total de la azotea.", admin}, {"rechazado", "2026-06-20 21:00", "Decisión de la junta: 0 a favor, 3 en contra", junta[3]}}, []string{"rechaza", "rechaza", "rechaza"}, "Pendiente no aprobado por la junta"},
		{"Intercomunicador del 5.º piso", "No suena el intercomunicador del 501 al 504.", "Piso 5", "electricidad", "baja", "terminado", "2026-06-25 18:00", inq, "302", 26000, 26000, "Electricidad Hogar",
			[]paso{{"validado", "2026-06-26 09:00", "", admin}, {"presupuestado", "2026-06-27 10:00", "", admin}, {"aprobado", "2026-06-27 10:30", "", admin}, {"en_ejecucion", "2026-07-01 09:00", "", tec}, {"terminado", "2026-07-02 12:00", "", tec}}, nil, ""},
		{"Jardín de la entrada sin riego", "Las plantas de la entrada están secas.", "Entrada", "jardineria", "baja", "descartado", "2026-07-08 09:20", op, "", 0, 0, "",
			[]paso{{"descartado", "2026-07-08 12:00", "Lo resolvió el operario ajustando el temporizador del riego.", admin}}, nil, "Lo resolvió el operario ajustando el temporizador del riego."},
		{"Cámara de seguridad del lobby", "La cámara del lobby no graba desde el lunes.", "Lobby", "seguridad", "media", "terminado", "2026-07-20 08:00", admin, "", 54000, 54000, "Seguridad Total SAC",
			[]paso{{"validado", "2026-07-20 09:00", "", admin}, {"presupuestado", "2026-07-22 15:00", "", admin}, {"aprobado", "2026-07-23 10:00", "", admin}, {"en_ejecucion", "2026-08-01 09:00", "", tec}, {"terminado", "2026-08-03 17:00", "Cámara y disco nuevos.", tec}}, nil, ""},
		{"Humedad en el sótano", "Paredes húmedas en el sótano 1, junto a los depósitos.", "Sótano 1", "estructura", "media", "aprobado", "2026-08-12 10:00", op, "", 90000, 0, "Impermeabilizaciones Perú",
			[]paso{{"validado", "2026-08-12 12:00", "", admin}, {"presupuestado", "2026-08-18 11:00", "Sellado de muro y drenaje.", admin}, {"aprobado", "2026-08-19 09:00", "Dentro del umbral: aprueba la administración.", admin}}, nil, ""},
		{"Tablero eléctrico de áreas comunes", "Salta la llave del tablero de áreas comunes por las noches.", "Cuarto de tableros", "electricidad", "critica", "en_ejecucion", "2026-08-26 22:40", op, "", 78000, 0, "Electricidad Hogar",
			[]paso{{"validado", "2026-08-27 08:00", "", admin}, {"presupuestado", "2026-08-28 12:00", "Cambio de interruptor diferencial y cableado.", admin}, {"aprobado", "2026-08-28 16:00", "", admin}, {"en_ejecucion", "2026-09-22 09:00", "Se cambió el diferencial; falta el cableado.", tec}}, nil, ""},
		{"Fuga de agua en el Dpto 402", "El medidor del 402 marca un consumo muy alto (52,857 m³). Posible fuga interna.", "Dpto 402", "gasfiteria", "critica", "validado", "2026-09-08 08:30", op, "402", 0, 0, "",
			[]paso{{"validado", "2026-09-08 10:00", "Crítica: se avisa al propietario del 402.", admin}}, nil, ""},
		{"Rejilla de la Parrilla 2 rota", "La rejilla de la Parrilla 2 está partida y no se puede usar.", "Parrillas – terraza", "areas_comunes", "", "reportado", "2026-09-20 21:15", maria, "201", 0, 0, "",
			nil, nil, ""},
		{"Pintura de la fachada lateral", "La pintura de la fachada lateral está descascarada.", "Fachada lateral", "estructura", "baja", "presupuestado", "2026-09-10 11:00", admin, "", 85000, 0, "Pinturas Lima EIRL",
			[]paso{{"validado", "2026-09-10 12:00", "", admin}, {"presupuestado", "2026-09-17 10:00", "Pintura de 120 m² con látex exterior.", admin}}, nil, ""},
		{"Bomba de agua N.º 2", "La bomba N.º 2 hace ruido y se recalienta; la N.º 1 trabaja sola.", "Cuarto de bombas", "bombas", "critica", "presupuestado", "2026-09-15 07:20", op, "", 185000, 0, "Bombas del Sur SAC",
			[]paso{{"validado", "2026-09-15 09:00", "Crítica: si falla la N.º 1 el edificio se queda sin agua.", admin}, {"presupuestado", "2026-09-18 12:00", "Rebobinado del motor y cambio de rodamientos.", admin}}, []string{"", "aprueba", "aprueba"}, ""},
	}
	for i, x := range lista {
		num := i + 1
		codigo := fmt.Sprintf("INC-%03d", num)
		creado := lima(x.creado[:10], x.creado[11:])
		var unidad *int64
		if x.unidad != "" {
			v := s.unidades[x.unidad]
			unidad = &v
		}
		var crit *string
		if x.crit != "" {
			crit = &x.crit
		}
		var presup *int64
		if x.presup > 0 {
			presup = &x.presup
		}
		var responsable *int64
		if x.estado == "terminado" || x.estado == "en_ejecucion" || x.estado == "aprobado" || codigo == "INC-014" {
			responsable = &tec
		}
		actualizado := creado
		if len(x.pasos) > 0 {
			u := x.pasos[len(x.pasos)-1]
			actualizado = lima(u.fecha[:10], u.fecha[11:])
		}
		var terminado *time.Time
		if x.estado == "terminado" {
			terminado = &actualizado
		}
		var rubro *int64
		if x.presup > 0 {
			v := s.rubros["correctivo"]
			rubro = &v
		}
		// El comprobante va en el INSERT: un UPDATE posterior pisaría actualizado_en (lo fija el disparador).
		var comp, costo *int64
		revision := false
		if x.estado == "terminado" {
			v := s.subir("comprobante-"+strings.ToLower(codigo)+".pdf", "application/pdf", facturaPDF(x.proveedor, codigo+" "+x.titulo, P.Soles(x.costo), actualizado.Format("2006-01-02")))
			comp = &v
			c := x.costo
			costo = &c
			revision = x.costo*10 > x.presup*11
		}
		id := s.id(`INSERT INTO incidencia (edificio_id, numero, codigo, titulo, descripcion, ubicacion, categoria, criticidad, estado, origen, reportado_por, unidad_id,
			responsable_id, proveedor, monto_presupuesto_cts, rubro_id, motivo, creado_en, actualizado_en, terminado_en, costo_real_cts, comprobante_id, revision_junta)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'app',$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22) RETURNING id`,
			s.eid, num, codigo, x.titulo, x.desc, x.ubic, x.cat, crit, x.estado, x.reporta, unidad, responsable, x.proveedor, presup, rubro, x.motivo, creado, actualizado, terminado,
			costo, comp, revision)
		foto := s.foto("reporte-"+strings.ToLower(codigo), colorObra, codigo+" · "+recorte(x.titulo, 26), x.ubic, "Foto del reporte", creado.Format("02/01/2006 15:04"))
		s.exec(`INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id, creado_en) VALUES ($1,$2,'reporte',$3,$4)`, id, foto, x.reporta, creado)
		s.exec(`INSERT INTO incidencia_evento (incidencia_id, estado_desde, estado_hasta, usuario_id, nota, creado_en) VALUES ($1,NULL,'reportado',$2,'Reportado desde la app',$3)`, id, x.reporta, creado)
		prev := "reportado"
		for _, p := range x.pasos {
			t := lima(p.fecha[:10], p.fecha[11:])
			s.exec(`INSERT INTO incidencia_evento (incidencia_id, estado_desde, estado_hasta, usuario_id, nota, creado_en) VALUES ($1,$2,$3,$4,$5,$6)`, id, prev, p.estado, p.quien, p.nota, t)
			prev = p.estado
		}
		for k, v := range x.votos {
			if v == "" {
				continue
			}
			t := creado.Add(time.Duration(72+k*20) * time.Hour)
			s.exec(`INSERT INTO voto (incidencia_id, usuario_id, voto, comentario, creado_en) VALUES ($1,$2,$3,$4,$5)`, id, junta[k], v, "", t)
		}
		if x.estado == "terminado" {
			cierre := s.foto("cierre-"+strings.ToLower(codigo), colorObra, codigo+" · Trabajo terminado", x.ubic, "Foto de cierre", actualizado.Format("02/01/2006"))
			s.exec(`INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id, creado_en) VALUES ($1,$2,'cierre',$3,$4)`, id, cierre, tec, actualizado)
			fecha := actualizado.Format("2006-01-02")
			s.exec(`INSERT INTO incidencia_evidencia (incidencia_id, archivo_id, tipo, usuario_id, creado_en) VALUES ($1,$2,'comprobante',$3,$4)`, id, *comp, tec, actualizado)
			s.exec(`INSERT INTO egreso (edificio_id, periodo, rubro_id, concepto_id, descripcion, monto_cts, fecha, documento_id, tipo_documento, origen, incidencia_id, registrado_por)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pdf','trabajo',$9,$10)`, s.eid, fecha[:7], s.rubros["correctivo"], s.concepto["correctivo.reparaciones"],
				codigo+" · "+x.titulo, x.costo, fecha, *comp, id, admin)
		}
	}
	return nil
}

func recorte(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "."
}
