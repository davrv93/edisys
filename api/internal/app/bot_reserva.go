package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/chatbot"
	P "edisys/api/internal/plataforma"
)

// Reservar desde el chatbot (bloque 6): una conversación con estado por teléfono (chatbot_sesion, 0013) que caduca
// a los 15 minutos. Pide área y fecha, ofrece las franjas libres numeradas, resume y pregunta «¿Confirmo?».
// La reserva la crea CrearReserva, el mismo servicio de la app: moroso, doble reserva (EXCLUDE) y horarios del
// reglamento se aplican igual. «cancelar» o «salir» en cualquier paso vuelve al menú.

const vidaSesionBot = 15 * time.Minute

type franjaBot struct {
	RecursoID int64     `json:"recurso_id"`
	Recurso   string    `json:"recurso"`
	Area      string    `json:"area"`
	Inicio    time.Time `json:"inicio"`
	Fin       time.Time `json:"fin"`
	TarifaCts int64     `json:"tarifa_cts"`
}

type datosSesionBot struct {
	AreaID  int64       `json:"area_id,omitempty"`
	Area    string      `json:"area,omitempty"`
	Fecha   string      `json:"fecha,omitempty"`
	Franjas []franjaBot `json:"franjas,omitempty"`
	Elegida *franjaBot  `json:"elegida,omitempty"`
}

type sesionBot struct {
	Paso  string
	Datos datosSesionBot
}

// leerSesion devuelve la sesión vigente del teléfono; caducada=true si había una y ya venció (se borra).
func (s *Server) leerSesion(ctx context.Context, tel string) (*sesionBot, bool, error) {
	var paso string
	var datos []byte
	var vence time.Time
	err := s.DB.QueryRow(ctx, `SELECT paso, datos, vence_en FROM chatbot_sesion WHERE telefono=$1`, tel).Scan(&paso, &datos, &vence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if time.Now().After(vence) {
		_ = s.borrarSesion(ctx, tel)
		return nil, true, nil
	}
	se := &sesionBot{Paso: paso}
	_ = json.Unmarshal(datos, &se.Datos)
	return se, false, nil
}

func (s *Server) guardarSesion(ctx context.Context, tel string, eid int64, paso string, d datosSesionBot) error {
	b, _ := json.Marshal(d)
	_, err := s.DB.Exec(ctx, `INSERT INTO chatbot_sesion (telefono, edificio_id, paso, datos, vence_en) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (telefono) DO UPDATE SET edificio_id=EXCLUDED.edificio_id, paso=EXCLUDED.paso, datos=EXCLUDED.datos, vence_en=EXCLUDED.vence_en, actualizado_en=now()`,
		tel, eid, paso, b, time.Now().Add(vidaSesionBot))
	return err
}

func (s *Server) borrarSesion(ctx context.Context, tel string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM chatbot_sesion WHERE telefono=$1`, tel)
	return err
}

func palabra(n string, ps ...string) bool {
	for _, p := range ps {
		if n == p {
			return true
		}
	}
	return false
}

func esCancelar(n string) bool {
	return palabra(n, "cancelar", "cancela", "salir", "cancelo", "anular", "no quiero")
}
func esSi(n string) bool {
	return palabra(n, "si", "confirmo", "confirmar", "ok", "okey", "dale", "claro", "correcto", "si confirmo", "si por favor", "de acuerdo", "yes")
}
func esNo(n string) bool { return palabra(n, "no", "nop", "mejor no", "no gracias") }

// conversacionReserva atiende el mensaje si hay una reserva en curso (o si pide cancelar). manejado=false:
// el mensaje sigue por el flujo normal del chatbot.
func (s *Server) conversacionReserva(ctx context.Context, q *quien, tel, texto string, cl chatbot.Resultado, res *RespuestaBot, hoy time.Time) (bool, error) {
	n := cl.Texto
	se, caducada, err := s.leerSesion(ctx, tel)
	if err != nil {
		return false, err
	}
	if esCancelar(n) {
		if se != nil {
			_ = s.borrarSesion(ctx, tel)
			res.Respuesta = "Listo, cancelé la reserva en curso.\n" + menuBot
		} else {
			res.Respuesta = menuBot
		}
		res.Intencion = chatbot.Menu
		return true, nil
	}
	if se == nil {
		if caducada {
			if _, err := strconv.Atoi(n); err == nil || esSi(n) {
				res.Intencion = chatbot.Reservar
				res.Datos["sesion_caducada"] = true
				res.Respuesta = "Tu reserva en curso caducó (pasaron más de 15 minutos) y no reservé nada. Escríbeme de nuevo qué área y qué día quieres, por ejemplo «parrilla el sábado»."
				return true, nil
			}
		}
		return false, nil
	}
	res.Intencion = chatbot.Reservar
	res.Datos["paso"] = se.Paso
	switch se.Paso {
	case "elegir_franja":
		if k, err := strconv.Atoi(n); err == nil {
			if k < 1 || k > len(se.Datos.Franjas) {
				res.Respuesta = fmt.Sprintf("Elige un número del 1 al %d, o escribe «cancelar».", len(se.Datos.Franjas))
				return true, nil
			}
			f := se.Datos.Franjas[k-1]
			se.Datos.Elegida = &f
			if err := s.guardarSesion(ctx, tel, q.edificioID, "confirmar", se.Datos); err != nil {
				return false, err
			}
			res.Datos["paso"] = "confirmar"
			res.Datos["franja"] = f
			res.Respuesta = s.resumenReserva(ctx, q.edificioID, f, hoy) + "\n¿Confirmo? Responde «sí» o «no»."
			return true, nil
		}
		if cl.Fecha != nil && (cl.Intencion == chatbot.Reservar || cl.Intencion == chatbot.NoEntendi || cl.Area != "") {
			return true, s.ofrecerFranjas(ctx, q, tel, se.Datos.AreaID, se.Datos.Area, *cl.Fecha, hoy, res, "")
		}
	case "confirmar":
		if esSi(n) {
			return true, s.confirmarReservaBot(ctx, q, tel, se, hoy, res)
		}
		if esNo(n) {
			_ = s.borrarSesion(ctx, tel)
			res.Respuesta = "Listo, no reservé nada.\n" + menuBot
			return true, nil
		}
		if _, err := strconv.Atoi(n); err != nil && cl.Intencion != chatbot.NoEntendi {
			break // cambió de tema
		}
		res.Respuesta = "Responde «sí» para confirmar la reserva o «no» para cancelarla."
		return true, nil
	case "elegir_fecha":
		if cl.Fecha != nil {
			return true, s.ofrecerFranjas(ctx, q, tel, se.Datos.AreaID, se.Datos.Area, *cl.Fecha, hoy, res, "")
		}
		if cl.Intencion == chatbot.NoEntendi {
			res.Respuesta = "No entendí la fecha. Escríbeme «hoy», «mañana», «el sábado» o una fecha como «12/10»."
			return true, nil
		}
	case "elegir_area":
		if cl.Area != "" {
			res.Intencion = chatbot.Reservar
			return true, s.flujoReserva(ctx, q, tel, cl, hoy, res)
		}
		if cl.Intencion == chatbot.NoEntendi {
			res.Respuesta = "¿Qué área quieres reservar? Escríbeme, por ejemplo, «parrilla», «SUM» o «piscina»."
			return true, nil
		}
	}
	// Cambió de tema: se olvida la reserva en curso y sigue el flujo normal.
	_ = s.borrarSesion(ctx, tel)
	res.Intencion = cl.Intencion
	delete(res.Datos, "paso")
	return false, nil
}

// flujoReserva: la intención «reservar» sin sesión. Revisa morosidad, pide lo que falte y ofrece las franjas.
func (s *Server) flujoReserva(ctx context.Context, q *quien, tel string, cl chatbot.Resultado, hoy time.Time, res *RespuestaBot) error {
	nombre := primerNombre(q.nombre)
	var deuda int64
	if len(q.unidades) > 0 {
		_ = s.DB.QueryRow(ctx, `SELECT deuda_vencida_cts($1)`, q.unidades[0]).Scan(&deuda)
	}
	if deuda > 0 {
		_ = s.borrarSesion(ctx, tel)
		res.Respuesta = fmt.Sprintf("%s, por ahora no puedes reservar porque tu %s tiene una deuda vencida de %s. Cuando la regularices te ayudo con la reserva. Escribe «pagar» para ver cómo.", nombre, dptos(q.codigos), P.Soles(deuda))
		res.Datos["moroso"] = true
		res.Datos["deuda_vencida_cts"] = deuda
		return nil
	}
	if q.rol == "inquilino" && len(q.unidades) > 0 {
		var puede bool
		_ = s.DB.QueryRow(ctx, `SELECT COALESCE((permisos_inquilino->>'reservar')::boolean, false) FROM unidad WHERE id=$1`, q.unidades[0]).Scan(&puede)
		if !puede {
			res.Respuesta = "El propietario de tu departamento no habilitó las reservas para el inquilino. Pídeselo o escribe a la administración."
			return nil
		}
	}
	areas, err := s.DB.Query(ctx, `SELECT id, nombre, slug, tarifa_cts FROM area WHERE edificio_id=$1 AND activo ORDER BY id`, q.edificioID)
	if err != nil {
		return err
	}
	type area struct {
		id           int64
		nombre, slug string
		tarifa       int64
	}
	var as []area
	for areas.Next() {
		var a area
		if err := areas.Scan(&a.id, &a.nombre, &a.slug, &a.tarifa); err == nil {
			as = append(as, a)
		}
	}
	areas.Close()
	if len(as) == 0 {
		res.Respuesta = "Tu edificio aún no tiene áreas reservables."
		return nil
	}
	var elegida *area
	for i, a := range as {
		if cl.Area != "" && (a.slug == cl.Area || strings.Contains(a.slug, cl.Area)) {
			elegida = &as[i]
		}
	}
	if elegida == nil {
		var ns []string
		lista := []map[string]any{}
		for _, a := range as {
			ns = append(ns, fmt.Sprintf("%s (%s)", a.nombre, P.Soles(a.tarifa)))
			lista = append(lista, map[string]any{"id": a.id, "nombre": a.nombre, "slug": a.slug, "tarifa_cts": a.tarifa})
		}
		res.Datos["areas"] = lista
		res.Datos["paso"] = "elegir_area"
		res.Respuesta = "¿Qué área quieres reservar? Tenemos: " + strings.Join(ns, ", ") + ". Escríbeme, por ejemplo, «parrilla el sábado»."
		return s.guardarSesion(ctx, tel, q.edificioID, "elegir_area", datosSesionBot{})
	}
	res.Datos["area"] = map[string]any{"id": elegida.id, "nombre": elegida.nombre, "slug": elegida.slug, "tarifa_cts": elegida.tarifa}
	if cl.Fecha == nil {
		res.Datos["paso"] = "elegir_fecha"
		res.Respuesta = fmt.Sprintf("¿Para qué día quieres %s? Escríbeme, por ejemplo, «mañana», «el sábado» o «5/10».", strings.ToLower(elegida.nombre))
		return s.guardarSesion(ctx, tel, q.edificioID, "elegir_fecha", datosSesionBot{AreaID: elegida.id, Area: elegida.nombre})
	}
	return s.ofrecerFranjas(ctx, q, tel, elegida.id, elegida.nombre, *cl.Fecha, hoy, res, "")
}

// ofrecerFranjas lista las franjas libres del área ese día, numeradas, y deja la sesión en «elegir_franja».
func (s *Server) ofrecerFranjas(ctx context.Context, q *quien, tel string, areaID int64, area string, fecha, hoy time.Time, res *RespuestaBot, prefijo string) error {
	franjas, err := s.calcularDisponibilidad(ctx, q.edificioID, "", strconv.FormatInt(areaID, 10), fecha, fecha, false)
	if err != nil {
		return err
	}
	var libres []franjaBot
	for _, f := range franjas {
		if f["estado"] != "libre" {
			continue
		}
		libres = append(libres, franjaBot{RecursoID: f["recurso_id"].(int64), Recurso: f["recurso"].(string), Area: f["area"].(string),
			Inicio: f["inicio"].(time.Time), Fin: f["fin"].(time.Time), TarifaCts: f["tarifa_cts"].(int64)})
	}
	if len(libres) > 9 {
		libres = libres[:9]
	}
	dia := diaTexto(fecha, hoy)
	res.Datos["fecha"] = fecha.Format("2006-01-02")
	res.Datos["franjas"] = libres
	if len(libres) == 0 {
		res.Datos["paso"] = "elegir_fecha"
		res.Respuesta = prefijo + fmt.Sprintf("Lo siento, %s no hay franjas libres en %s. ¿Probamos otro día? Escríbeme la fecha o «cancelar».", dia, strings.ToLower(area))
		return s.guardarSesion(ctx, tel, q.edificioID, "elegir_fecha", datosSesionBot{AreaID: areaID, Area: area})
	}
	var b strings.Builder
	b.WriteString(prefijo)
	fmt.Fprintf(&b, "%s tienes libre en %s:\n", mayus(dia), strings.ToLower(area))
	for i, f := range libres {
		fmt.Fprintf(&b, "%d. %s · %s a %s (%s)\n", i+1, f.Recurso, f.Inicio.In(P.Lima).Format("15:04"), f.Fin.In(P.Lima).Format("15:04"), P.Soles(f.TarifaCts))
	}
	b.WriteString("Responde con el número de la franja, o «cancelar».")
	res.Respuesta = b.String()
	res.Datos["paso"] = "elegir_franja"
	return s.guardarSesion(ctx, tel, q.edificioID, "elegir_franja", datosSesionBot{AreaID: areaID, Area: area, Fecha: fecha.Format("2006-01-02"), Franjas: libres})
}

func (s *Server) resumenReserva(ctx context.Context, eid int64, f franjaBot, hoy time.Time) string {
	var modo string
	_ = s.DB.QueryRow(ctx, `SELECT modo_cobro_reservas FROM edificio WHERE id=$1`, eid).Scan(&modo)
	ini := f.Inicio.In(P.Lima)
	costo := "Sin costo."
	if f.TarifaCts > 0 {
		if modo == "pago_inmediato" {
			costo = fmt.Sprintf("Costo: %s, se paga por Yape al confirmar (te guardo la franja 15 minutos).", P.Soles(f.TarifaCts))
		} else {
			costo = fmt.Sprintf("Costo: %s, se carga a tu recibo de %s.", P.Soles(f.TarifaCts), P.NombrePeriodo(ini.Format("2006-01")))
		}
	}
	return fmt.Sprintf("Resumen: %s (%s), %s de %s a %s. %s", f.Recurso, f.Area, diaTexto(time.Date(ini.Year(), ini.Month(), ini.Day(), 0, 0, 0, 0, P.Lima), hoy),
		ini.Format("15:04"), f.Fin.In(P.Lima).Format("15:04"), costo)
}

// confirmarReservaBot crea la reserva con el mismo servicio que la app.
func (s *Server) confirmarReservaBot(ctx context.Context, q *quien, tel string, se *sesionBot, hoy time.Time, res *RespuestaBot) error {
	f := se.Datos.Elegida
	if f == nil || len(q.unidades) == 0 {
		_ = s.borrarSesion(ctx, tel)
		res.Respuesta = "No encontré la franja elegida. Empecemos de nuevo: escríbeme qué área y qué día quieres."
		return nil
	}
	e := &Edificio{ID: q.edificioID, Rol: q.rol, Unidades: q.unidades, Permisos: map[string]bool{"reservas.crear": true}}
	var uid int64
	if q.usuarioID != nil {
		uid = *q.usuarioID
	}
	r, err := s.CrearReserva(ctx, e, uid, f.RecursoID, q.unidades[0], f.Inicio.Format(time.RFC3339), f.Fin.Format(time.RFC3339), true, "")
	if err != nil {
		var pe *P.Error
		if !errors.As(err, &pe) {
			return err
		}
		switch pe.Codigo {
		case "FRANJA_OCUPADA":
			fecha, _ := time.ParseInLocation("2006-01-02", se.Datos.Fecha, P.Lima)
			res.Datos["franja_ocupada"] = true
			return s.ofrecerFranjas(ctx, q, tel, se.Datos.AreaID, se.Datos.Area, fecha, hoy, res, "Alguien acaba de reservar esa franja. ")
		case "MOROSO":
			_ = s.borrarSesion(ctx, tel)
			var deuda int64
			_ = s.DB.QueryRow(ctx, `SELECT deuda_vencida_cts($1)`, q.unidades[0]).Scan(&deuda)
			res.Datos["moroso"] = true
			res.Respuesta = fmt.Sprintf("No pude reservar: tu %s tiene una deuda vencida de %s. Escribe «pagar» para ver cómo regularizarla.", dptos(q.codigos), P.Soles(deuda))
			return nil
		default:
			_ = s.borrarSesion(ctx, tel)
			res.Respuesta = "No pude reservar: " + pe.Mensaje + " Escríbeme otra fecha o área."
			res.Datos["error"] = pe.Codigo
			return nil
		}
	}
	_ = s.borrarSesion(ctx, tel)
	res.Datos["reserva"] = r
	res.Datos["paso"] = "reservado"
	ini := f.Inicio.In(P.Lima)
	cuando := fmt.Sprintf("%s de %s a %s", diaTexto(time.Date(ini.Year(), ini.Month(), ini.Day(), 0, 0, 0, 0, P.Lima), hoy), ini.Format("15:04"), f.Fin.In(P.Lima).Format("15:04"))
	if r["estado"] == "pendiente_pago" {
		var yape string
		_ = s.DB.QueryRow(ctx, `SELECT yape_numero FROM edificio WHERE id=$1`, q.edificioID).Scan(&yape)
		res.Respuesta = fmt.Sprintf("Listo, te guardo %s %s por 15 minutos (código %s). Para confirmarla paga %s por Yape al %s y sube el voucher con el código de operación en %s/app/reservas/. Si no llega el pago, la franja se libera.",
			f.Recurso, cuando, r["codigo"], P.Soles(f.TarifaCts), valorO(yape, "número del edificio"), s.Cfg.URLPublica)
		return nil
	}
	costo := ""
	if f.TarifaCts > 0 {
		costo = fmt.Sprintf(" El costo de %s se carga a tu recibo de %s.", P.Soles(f.TarifaCts), P.NombrePeriodo(ini.Format("2006-01")))
	}
	res.Respuesta = fmt.Sprintf("¡Reservado! %s %s. Código %s.%s Recuerda dejar el área limpia.", f.Recurso, cuando, r["codigo"], costo)
	return nil
}
