package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/chatbot"
	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// RespuestaBot es lo que contesta el chatbot.
type RespuestaBot struct {
	Respuesta  string
	Intencion  string
	Datos      map[string]any
	EdificioID int64
	UnidadID   *int64
}

const menuBot = "Puedo ayudarte con:\n1. Cuánto debo\n2. Mi último recibo\n3. Cómo pagar\n4. Reservar un área común\n5. Reportar un problema\n6. Horarios y normas\n7. Hablar con la administración\nEscribe el número o tu consulta."

// quien: la persona que escribe, identificada por su celular.
type quien struct {
	personaID  int64
	usuarioID  *int64
	nombre     string
	edificioID int64
	edificio   string
	unidades   []int64
	codigos    []string
	rol        string
}

func (s *Server) identificar(ctx context.Context, eid int64, tel string) (*quien, error) {
	q := &quien{}
	// Comparamos solo dígitos: «51 900 000 201», «900000201» y «51900000201» son el mismo celular.
	err := s.DB.QueryRow(ctx, `SELECT pe.id, pe.usuario_id, pe.nombre, e.id, e.nombre, up.rol
		FROM persona pe JOIN unidad_persona up ON up.persona_id=pe.id AND up.hasta IS NULL JOIN unidad u ON u.id=up.unidad_id JOIN edificio e ON e.id=u.edificio_id
		WHERE ($2::bigint = 0 OR e.id=$2) AND pe.celular <> '' AND
		  (regexp_replace(pe.celular,'\D','','g') = $1 OR '51' || regexp_replace(pe.celular,'\D','','g') = $1)
		ORDER BY (up.rol='propietario') DESC, pe.id LIMIT 1`, tel, eid).Scan(&q.personaID, &q.usuarioID, &q.nombre, &q.edificioID, &q.edificio, &q.rol)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	filas, err := s.DB.Query(ctx, `SELECT u.id, u.codigo FROM unidad_persona up JOIN unidad u ON u.id=up.unidad_id
		WHERE up.persona_id=$1 AND up.hasta IS NULL AND u.edificio_id=$2 ORDER BY u.codigo`, q.personaID, q.edificioID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var id int64
		var c string
		if err := filas.Scan(&id, &c); err != nil {
			return nil, err
		}
		q.unidades = append(q.unidades, id)
		q.codigos = append(q.codigos, c)
	}
	return q, filas.Err()
}

func primerNombre(n string) string { return strings.Split(strings.TrimSpace(n), " ")[0] }

func dptos(c []string) string {
	if len(c) == 1 {
		return "Dpto " + c[0]
	}
	return "Dptos " + strings.Join(c, ", ")
}

// Responder clasifica el mensaje y arma la respuesta con cifras reales de la base. eid=0: busca en todos.
func (s *Server) Responder(ctx context.Context, eid int64, tel, texto string) (*RespuestaBot, error) {
	hoy := time.Now().In(P.Lima)
	cl := chatbot.Clasificar(texto, hoy)
	res := &RespuestaBot{Intencion: cl.Intencion, Datos: map[string]any{"identificado": false}}
	q, err := s.identificar(ctx, eid, tel)
	if err != nil {
		return nil, err
	}
	if q == nil {
		res.EdificioID = eid
		res.Respuesta = "Hola. No encuentro tu número entre los propietarios e inquilinos registrados. Pide a la administración que lo registre y vuelve a escribirme."
		return res, nil
	}
	res.EdificioID = q.edificioID
	if len(q.unidades) > 0 {
		res.UnidadID = &q.unidades[0]
	}
	nombre := primerNombre(q.nombre)
	res.Datos = map[string]any{"identificado": true, "nombre": q.nombre, "edificio": q.edificio, "unidades": q.codigos, "rol": q.rol}
	base := fmt.Sprintf("%s/app/e/%d", s.Cfg.URLPublica, q.edificioID)

	switch cl.Intencion {
	case chatbot.Saludo:
		res.Respuesta = fmt.Sprintf("¡Hola, %s! Soy el asistente de %s. %s", nombre, q.edificio, menuBot)

	case chatbot.Menu:
		res.Respuesta = menuBot

	case chatbot.Saldo:
		deuda, err := s.deudaPorUnidad(ctx, q.edificioID, q.unidades)
		if err != nil {
			return nil, err
		}
		var total, vencida int64
		var lineas []string
		for _, d := range deuda {
			total += d["deuda_cts"].(int64)
			vencida += d["deuda_vencida_cts"].(int64)
			var meses []map[string]any
			b, _ := json.Marshal(d["meses"])
			_ = json.Unmarshal(b, &meses)
			for _, m := range meses {
				saldo := int64(m["saldo_cts"].(float64))
				if m["origen"] == "deuda_inicial" {
					lineas = append(lineas, fmt.Sprintf("• Dpto %s · deuda anterior de %s: %s", d["unidad"], P.NombrePeriodo(m["periodo"].(string)), P.Soles(saldo)))
					continue
				}
				lineas = append(lineas, fmt.Sprintf("• Dpto %s · %s: %s (vence %s)", d["unidad"], P.NombrePeriodo(m["periodo"].(string)), P.Soles(saldo), fechaCorta(m["vence"])))
			}
		}
		res.Datos["deuda_cts"] = total
		res.Datos["deuda_vencida_cts"] = vencida
		res.Datos["detalle"] = deuda
		if total == 0 {
			ult, _ := db.Fila(ctx, s.DB, `SELECT p.periodo, r.total_cts FROM recibo r JOIN periodo p ON p.id=r.periodo_id WHERE r.unidad_id = ANY($1) AND r.estado NOT IN ('borrador','anulado') ORDER BY p.periodo DESC LIMIT 1`, q.unidades)
			extra := ""
			if ult != nil {
				extra = fmt.Sprintf(" Tu último recibo (%s) de %s ya está pagado.", P.NombrePeriodo(ult["periodo"].(string)), P.Soles(ult["total_cts"].(int64)))
			}
			res.Respuesta = fmt.Sprintf("Estás al día, %s. ¡Gracias!%s", nombre, extra)
		} else {
			res.Respuesta = fmt.Sprintf("%s, tu %s tiene un saldo pendiente de %s:\n%s", nombre, dptos(q.codigos), P.Soles(total), strings.Join(lineas, "\n"))
			if vencida > 0 {
				res.Respuesta += fmt.Sprintf("\nDe eso, %s ya está vencido. Mientras tengas deuda vencida no podrás reservar áreas comunes.", P.Soles(vencida))
			}
			res.Respuesta += "\nEscribe «pagar» y te digo cómo hacerlo."
		}

	case chatbot.UltimoRecibo:
		rc, err := db.Fila(ctx, s.DB, `SELECT r.id, p.periodo, r.numero, r.estado, r.total_cts, r.total_cts - r.pagado_cts AS saldo_cts, to_char(r.vence,'DD/MM/YYYY') AS vence, u.codigo
			FROM recibo r JOIN periodo p ON p.id=r.periodo_id JOIN unidad u ON u.id=r.unidad_id
			WHERE r.unidad_id = ANY($1) AND r.estado NOT IN ('borrador','anulado') ORDER BY p.periodo DESC, r.id DESC LIMIT 1`, q.unidades)
		if err != nil {
			res.Respuesta = "Todavía no tienes recibos emitidos. Tu primer recibo llega a inicios de mes."
			break
		}
		lineas, _ := db.Filas(ctx, s.DB, `SELECT descripcion, monto_cts FROM recibo_linea WHERE recibo_id=$1 ORDER BY orden, id`, rc["id"])
		var b strings.Builder
		fmt.Fprintf(&b, "Tu recibo de %s del Dpto %s es de %s:\n", P.NombrePeriodo(rc["periodo"].(string)), rc["codigo"], P.Soles(rc["total_cts"].(int64)))
		for _, l := range lineas {
			fmt.Fprintf(&b, "• %s: %s\n", l["descripcion"], P.Soles(l["monto_cts"].(int64)))
		}
		if rc["saldo_cts"].(int64) == 0 {
			b.WriteString("Estado: pagado. ¡Gracias!")
		} else {
			fmt.Fprintf(&b, "Saldo pendiente: %s (vence el %v).", P.Soles(rc["saldo_cts"].(int64)), rc["vence"])
		}
		fmt.Fprintf(&b, "\nMíralo con la foto de tu medidor en %s/recibos/%d", base, rc["id"])
		res.Respuesta = b.String()
		res.Datos["recibo"] = rc
		res.Datos["lineas"] = lineas

	case chatbot.Pagar:
		var yape string
		_ = s.DB.QueryRow(ctx, `SELECT yape_numero FROM edificio WHERE id=$1`, q.edificioID).Scan(&yape)
		var saldo int64
		_ = s.DB.QueryRow(ctx, `SELECT COALESCE(sum(total_cts - pagado_cts),0) FROM recibo WHERE unidad_id = ANY($1) AND estado IN ('emitido','pagado_parcial')`, q.unidades).Scan(&saldo)
		periodo := P.PeriodoActual()
		b := fmt.Sprintf("Puedes pagar por Yape o Plin al %s (%s) o por transferencia a la cuenta del edificio. En el concepto pon «%s %s». Después sube la foto del voucher y el código de operación en %s/portal y la administración lo valida.",
			valorO(yape, "número del edificio"), q.edificio, dptos(q.codigos), periodo, base)
		if saldo > 0 {
			b += fmt.Sprintf(" Tu saldo pendiente es %s.", P.Soles(saldo))
		} else {
			b += " Por ahora no tienes saldo pendiente."
		}
		res.Respuesta = b
		res.Datos["saldo_cts"] = saldo
		res.Datos["yape"] = yape

	case chatbot.Reservar:
		var deuda int64
		if len(q.unidades) > 0 {
			_ = s.DB.QueryRow(ctx, `SELECT deuda_vencida_cts($1)`, q.unidades[0]).Scan(&deuda)
		}
		if deuda > 0 {
			res.Respuesta = fmt.Sprintf("%s, por ahora no puedes reservar porque tu %s tiene una deuda vencida de %s. Cuando la regularices te ayudo con la reserva. Escribe «pagar» para ver cómo.", nombre, dptos(q.codigos), P.Soles(deuda))
			res.Datos["moroso"] = true
			res.Datos["deuda_vencida_cts"] = deuda
			break
		}
		areas, _ := db.Filas(ctx, s.DB, `SELECT id, nombre, slug, tarifa_cts FROM area WHERE edificio_id=$1 AND activo ORDER BY id`, q.edificioID)
		if len(areas) == 0 {
			res.Respuesta = "Tu edificio aún no tiene áreas reservables."
			break
		}
		var area map[string]any
		for _, a := range areas {
			if cl.Area != "" && (a["slug"] == cl.Area || strings.Contains(a["slug"].(string), cl.Area)) {
				area = a
			}
		}
		if area == nil {
			var ns []string
			for _, a := range areas {
				ns = append(ns, fmt.Sprintf("%s (%s)", a["nombre"], P.Soles(a["tarifa_cts"].(int64))))
			}
			res.Respuesta = "¿Qué área quieres reservar? Tenemos: " + strings.Join(ns, ", ") + ". Escríbeme, por ejemplo, «parrilla el sábado»."
			res.Datos["areas"] = areas
			break
		}
		if cl.Fecha == nil {
			res.Respuesta = fmt.Sprintf("¿Para qué día quieres %s? Escríbeme, por ejemplo, «mañana», «el sábado» o «5/10».", strings.ToLower(area["nombre"].(string)))
			res.Datos["area"] = area
			break
		}
		franjas, err := s.calcularDisponibilidad(ctx, q.edificioID, "", fmt.Sprint(area["id"]), *cl.Fecha, *cl.Fecha, false)
		if err != nil {
			return nil, err
		}
		libres := map[string][]string{}
		var orden []string
		for _, f := range franjas {
			if f["estado"] != "libre" {
				continue
			}
			rec := f["recurso"].(string)
			if _, ok := libres[rec]; !ok {
				orden = append(orden, rec)
			}
			libres[rec] = append(libres[rec], fmt.Sprintf("%s a %s", f["hora_inicio"], f["hora_fin"]))
		}
		dia := diaTexto(*cl.Fecha, hoy)
		if len(orden) == 0 {
			res.Respuesta = fmt.Sprintf("Lo siento, %s no hay franjas libres en %s. ¿Probamos otro día?", dia, strings.ToLower(area["nombre"].(string)))
		} else {
			var partes []string
			for _, rec := range orden {
				partes = append(partes, fmt.Sprintf("%s: de %s", rec, strings.Join(libres[rec], " y de ")))
			}
			res.Respuesta = fmt.Sprintf("%s tienes libre en %s → %s. La tarifa es %s. Para reservar entra a %s/reservas/nueva y elige la franja.",
				mayus(dia), strings.ToLower(area["nombre"].(string)), strings.Join(partes, "; "), P.Soles(area["tarifa_cts"].(int64)), base)
		}
		res.Datos["area"] = area
		res.Datos["fecha"] = cl.Fecha.Format("2006-01-02")
		res.Datos["libres"] = libres

	case chatbot.Reportar:
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		id, codigo, err := s.crearIncidencia(ctx, tx, q.edificioID, q.usuarioID, res.UnidadID, "", strings.TrimSpace(texto), "Reportado por WhatsApp", categoriaDe(cl.Texto), "whatsapp")
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		res.Respuesta = fmt.Sprintf("Listo, %s. Registré tu reporte %s: «%s». La administración lo revisará y te avisaremos por aquí cuando cambie de estado. Si puedes, agrega una foto desde %s/mantenimiento/reportar.",
			nombre, codigo, recortar(strings.TrimSpace(texto), 80), base)
		res.Datos["incidencia_id"] = id
		res.Datos["codigo"] = codigo

	case chatbot.Horarios:
		areas, _ := db.Filas(ctx, s.DB, `SELECT nombre, slug, franjas, normas, aforo, tarifa_cts FROM area WHERE edificio_id=$1 AND activo ORDER BY id`, q.edificioID)
		var b strings.Builder
		b.WriteString("Horarios de las áreas comunes:\n")
		n := 0
		for _, a := range areas {
			if cl.Area != "" && a["slug"] != cl.Area && !strings.Contains(a["slug"].(string), cl.Area) {
				continue
			}
			n++
			var fs []Franja
			bb, _ := json.Marshal(a["franjas"])
			_ = json.Unmarshal(bb, &fs)
			var hs []string
			for _, f := range fs {
				hs = append(hs, f.Inicio+" a "+f.Fin)
			}
			fmt.Fprintf(&b, "• %s: %s.", a["nombre"], strings.Join(hs, " y "))
			if a["aforo"] != nil {
				fmt.Fprintf(&b, " Aforo: %v personas.", a["aforo"])
			}
			if nm, _ := a["normas"].(string); nm != "" {
				fmt.Fprintf(&b, " Normas: %s", nm)
			}
			b.WriteString("\n")
		}
		if n == 0 {
			b.Reset()
			b.WriteString("No encontré esa área. ")
		}
		var normas string
		_ = s.DB.QueryRow(ctx, `SELECT normas_texto FROM edificio WHERE id=$1`, q.edificioID).Scan(&normas)
		if normas != "" && cl.Area == "" {
			b.WriteString("Normas generales: " + normas)
		}
		res.Respuesta = strings.TrimSpace(b.String())
		res.Datos["areas"] = areas

	case chatbot.HablarAdmin:
		res.Respuesta = fmt.Sprintf("Le aviso a la administración para que te escriba, %s. Atendemos de lunes a viernes de 9:00 a 18:00. Si es una emergencia (fuga, corte de luz, ascensor detenido), escríbeme el problema y lo registro de inmediato.", nombre)
		// Aviso a los administradores con celular.
		admins, _ := db.Filas(ctx, s.DB, `SELECT us.telefono FROM usuario_edificio_rol uer JOIN usuario us ON us.id=uer.usuario_id
			WHERE uer.edificio_id=$1 AND uer.rol='administrador' AND us.activo AND us.telefono <> ''`, q.edificioID)
		for _, a := range admins {
			_, _ = s.encolar(ctx, s.DB, q.edificioID, res.UnidadID, a["telefono"].(string), "aviso_general",
				map[string]string{"mensaje": fmt.Sprintf("%s (%s) pide hablar con la administración por WhatsApp. Su mensaje: «%s»", q.nombre, dptos(q.codigos), recortar(texto, 120))}, "chatbot", nil)
		}
		res.Datos["administradores_avisados"] = len(admins)

	default:
		res.Intencion = chatbot.NoEntendi
		res.Respuesta = "No te entendí bien, " + nombre + ". " + menuBot
	}
	return res, nil
}

func categoriaDe(n string) string {
	switch {
	case strings.Contains(n, "bomba"):
		return "bombas"
	case strings.Contains(n, "fuga") || strings.Contains(n, "agua") || strings.Contains(n, "gotea") || strings.Contains(n, "desague") || strings.Contains(n, "cano"):
		return "gasfiteria"
	case strings.Contains(n, "luz") || strings.Contains(n, "foco") || strings.Contains(n, "electric") || strings.Contains(n, "enchufe"):
		return "electricidad"
	case strings.Contains(n, "ascensor"):
		return "ascensores"
	case strings.Contains(n, "limpi") || strings.Contains(n, "basura"):
		return "limpieza"
	case strings.Contains(n, "puerta") || strings.Contains(n, "camara") || strings.Contains(n, "seguridad"):
		return "seguridad"
	case strings.Contains(n, "parrilla") || strings.Contains(n, "piscina") || strings.Contains(n, "sum"):
		return "areas_comunes"
	}
	return "otros"
}

func recortar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func valorO(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func mayus(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

var diasES = []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

func diaTexto(f, hoy time.Time) string {
	d := time.Date(hoy.Year(), hoy.Month(), hoy.Day(), 0, 0, 0, 0, hoy.Location())
	switch int(f.Sub(d).Hours() / 24) {
	case 0:
		return "hoy"
	case 1:
		return "mañana"
	}
	return fmt.Sprintf("el %s %s", diasES[f.Weekday()], f.Format("02/01"))
}

func fechaCorta(v any) string {
	s, _ := v.(string)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("02/01")
	}
	return s
}
