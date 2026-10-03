package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	P "edisys/api/internal/plataforma"
)

// Bloques H1, H2 y H3 · reservas avanzadas.

func (e *entorno) idAreaRsv(slug string) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM area WHERE slug=$1`, slug).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *entorno) idRecursoRsv(nombre string) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM recurso WHERE nombre=$1`, nombre).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// cuerpoReserva arma el POST /reservas para una franja (hora de Lima) dentro de n días.
func cuerpoReservaRsv(recurso, unidad int64, dias int, desde, hasta string) map[string]any {
	dia := time.Now().In(P.Lima).AddDate(0, 0, dias).Format("2006-01-02")
	ini, _ := time.ParseInLocation("2006-01-02 15:04", dia+" "+desde, P.Lima)
	fin, _ := time.ParseInLocation("2006-01-02 15:04", dia+" "+hasta, P.Lima)
	return map[string]any{"recurso_id": recurso, "unidad_id": unidad, "inicio": ini.Format(time.RFC3339), "fin": fin.Format(time.RFC3339), "acepta_normas": true}
}

// H2 · anticipación mínima, separación, garantía + limpieza en el total, aforo, cupo y morosos con deuda parcial.
func TestReservasRestricciones(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	parrillas := e.idAreaRsv("parrillas")
	p1, p2 := e.idRecursoRsv("Parrilla 1"), e.idRecursoRsv("Parrilla 2")
	u201 := e.idUnidad("201")

	// La anticipación mínima no puede pasar a la máxima (30).
	if st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", parrillas), tok, map[string]any{"anticipacion_min_dias": 40}); st != 422 {
		t.Fatalf("mínima > máxima: %d %v", st, d)
	}
	st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", parrillas), tok, map[string]any{
		"anticipacion_min_dias": 3, "separacion_dias": 5, "garantia_cts": 10000, "limpieza_cts": 3000, "descripcion": "Dos parrillas techadas"})
	if st != 200 || num(d["garantia_cts"]) != 10000 || d["descripcion"] != "Dos parrillas techadas" {
		t.Fatalf("configurar parrillas: %d %v", st, d)
	}

	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p1, u201, 1, "12:00", "17:00")); st != 422 || codigo(d) != "FUERA_DE_PLAZO" {
		t.Errorf("anticipación mínima: %d %v", st, d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p1, u201, 12, "12:00", "17:00"))
	if st != 201 || num(d["total_cts"]) != 8000+10000+3000 || num(d["garantia_cts"]) != 10000 || num(d["limpieza_cts"]) != 3000 {
		t.Fatalf("reserva con garantía y limpieza: %d %v", st, d)
	}
	// Dos días después en la otra parrilla: misma área, dentro de la separación.
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p2, u201, 14, "18:00", "23:00")); st != 422 || codigo(d) != "SEPARACION_MINIMA" {
		t.Errorf("separación: %d %v", st, d)
	}
	// Cinco días después ya se puede; otra unidad no se ve afectada por la separación del 201.
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p2, u201, 17, "18:00", "23:00")); st != 201 {
		t.Errorf("fuera de la separación: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p2, e.idUnidad("202"), 14, "18:00", "23:00")); st != 201 {
		t.Errorf("otra unidad: %d %v", st, d)
	}

	// H3 · aforo y cupo mensual en el SUM (aforo 40).
	sum := e.idAreaRsv("sum")
	if st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", sum), tok, map[string]any{"cupo_mensual_unidad": 1}); st != 200 || num(d["cupo_mensual_unidad"]) != 1 {
		t.Fatalf("cupo: %d %v", st, d)
	}
	rsum := e.idRecursoRsv("SUM")
	u203 := e.idUnidad("203")
	c := cuerpoReservaRsv(rsum, u203, 10, "10:00", "14:00")
	c["asistentes"] = 50
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, c); st != 422 || codigo(d) != "AFORO_EXCEDIDO" {
		t.Errorf("aforo: %d %v", st, d)
	}
	c["asistentes"] = 25
	c["titulo"] = "Cumpleaños"
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, c); st != 201 || d["titulo"] != "Cumpleaños" || num(d["asistentes"]) != 25 {
		t.Fatalf("SUM con asistentes: %d %v", st, d)
	}
	// Otro día del mismo mes: el cupo de 1 ya se usó.
	d10 := time.Now().In(P.Lima).AddDate(0, 0, 10)
	otro := 11
	if d10.AddDate(0, 0, 1).Month() != d10.Month() {
		otro = 9
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(rsum, u203, otro, "10:00", "14:00")); st != 422 || codigo(d) != "CUPO_AGOTADO" {
		t.Errorf("cupo agotado: %d %v", st, d)
	}
	// Sin tope (0) vuelve a dejar.
	e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", sum), tok, map[string]any{"cupo_mensual_unidad": 0})
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(rsum, u203, otro, "10:00", "14:00")); st != 201 {
		t.Errorf("sin cupo: %d %v", st, d)
	}

	// Morosos con deuda parcial: el 402 debe S/ 1.420,00.
	piscina := e.idAreaRsv("piscina")
	rpis := e.idRecursoRsv("Piscina")
	u402 := e.idUnidad("402")
	e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", piscina), tok, map[string]any{"permite_parciales": true, "deuda_tolerada_cts": 100000})
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(rpis, u402, 6, "08:00", "12:00")); st != 403 || codigo(d) != "MOROSO" {
		t.Errorf("deuda sobre la tolerada: %d %v", st, d)
	}
	e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", piscina), tok, map[string]any{"deuda_tolerada_cts": 150000})
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(rpis, u402, 6, "08:00", "12:00")); st != 201 {
		t.Errorf("deuda parcial tolerada: %d %v", st, d)
	}
	// La base aplica la misma excepción: en Parrillas (sin excepción) el 402 sigue bloqueado.
	ini := time.Now().Add(240 * time.Hour)
	if _, err := e.pool.Exec(context.Background(), `INSERT INTO reserva (edificio_id, recurso_id, unidad_id, codigo, inicio, fin, estado) VALUES (1,$1,$2,'R-T402',$3,$4,'confirmada')`,
		p1, u402, ini, ini.Add(time.Hour)); err == nil || !strings.Contains(err.Error(), "ED002") {
		t.Errorf("la base dejó reservar al moroso: %v", err)
	}
	// Sin acuerdo_pago (bloque D1) la deuda financiada no habilita.
	var fin bool
	_ = e.pool.QueryRow(context.Background(), `SELECT reserva_unidad_financiada($1)`, u402).Scan(&fin)
	if fin {
		t.Error("sin acuerdos de pago, nadie tiene deuda financiada")
	}
}

// H3 · horario por día de la semana con tarifa propia de la franja.
func TestReservasHorarioPorDia(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")
	parrillas := e.idAreaRsv("parrillas")
	p1 := e.idRecursoRsv("Parrilla 1")
	dia := time.Now().In(P.Lima).AddDate(0, 0, 8)
	iso := int(dia.Weekday())
	if iso == 0 {
		iso = 7
	}
	horarios := map[string]any{fmt.Sprint(iso): []map[string]any{{"inicio": "18:00", "fin": "22:00", "tarifa_cts": 5000}}}
	if st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", parrillas), tok, map[string]any{"horarios": map[string]any{"9": []any{}}}); st != 422 {
		t.Errorf("día 9 inválido: %d %v", st, d)
	}
	if st, d := e.pedir("PUT", fmt.Sprintf("/api/v1/edificios/1/areas/%d", parrillas), tok, map[string]any{"horarios": horarios, "limpieza_cts": 2000}); st != 200 {
		t.Fatalf("horarios: %d %v", st, d)
	}
	st, d := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/disponibilidad?recurso=%d&desde=%s&hasta=%s", p1, dia.Format("2006-01-02"), dia.Format("2006-01-02")), tok, nil)
	fr, _ := d["franjas"].([]any)
	if st != 200 || len(fr) != 1 || fr[0].(map[string]any)["hora_inicio"] != "18:00" || num(fr[0].(map[string]any)["tarifa_cts"]) != 5000 {
		t.Fatalf("disponibilidad con horario del día: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p1, e.idUnidad("201"), 8, "12:00", "17:00")); st != 422 || codigo(d) != "FUERA_DE_HORARIO" {
		t.Errorf("franja general ese día: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p1, e.idUnidad("201"), 8, "18:00", "22:00")); st != 201 || num(d["total_cts"]) != 5000+2000 {
		t.Errorf("franja del día con su tarifa: %d %v", st, d)
	}
	// El día siguiente sigue con las franjas generales.
	if st, d := e.pedir("POST", "/api/v1/edificios/1/reservas", tok, cuerpoReservaRsv(p1, e.idUnidad("202"), 9, "12:00", "17:00")); st != 201 || num(d["total_cts"]) != 8000+2000 {
		t.Errorf("día sin horario propio: %d %v", st, d)
	}
}

// H1 · QR de la reserva y check-in del conserje: franja, deuda del día del evento, repetidos y forzado.
func TestReservaCheckinQR(t *testing.T) {
	e := nuevo(t)
	ctx := context.Background()
	adm := e.login("admin@demo.pe")
	ope := e.login("operario@demo.pe")
	prop := e.login("propietario201@demo.pe")

	// Un área propia para no chocar con las reservas sembradas.
	st, d := e.pedir("POST", "/api/v1/edificios/1/areas", adm, map[string]any{"nombre": "Terraza", "franjas": []map[string]string{{"inicio": "10:00", "fin": "14:00"}}, "checkin_tolerancia_min": 15,
		"recursos": []string{"Terraza 1", "Terraza 2", "Terraza 3", "Terraza 4", "Terraza 5"}})
	if st != 201 {
		t.Fatalf("crear terraza: %d %v", st, d)
	}
	// Un recurso por reserva: así ninguna choca con otra (EXCLUDE).
	n9 := 0
	ahora := time.Now()
	nueva := func(cod, unidad string, ini time.Time, horas int, estado, forzado string) int64 {
		n9++
		rec := e.idRecursoRsv(fmt.Sprintf("Terraza %d", n9))
		var id int64
		var forz any
		if forzado != "" {
			forz = forzado
		}
		if err := e.pool.QueryRow(ctx, `INSERT INTO reserva (edificio_id, recurso_id, unidad_id, codigo, inicio, fin, estado, forzado_motivo) VALUES (1,$1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			rec, e.idUnidad(unidad), cod, ini, ini.Add(time.Duration(horas)*time.Hour), estado, forz).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	enCurso := nueva("R-9001", "201", ahora.Add(-time.Hour), 2, "confirmada", "")

	// El QR: el admin y el propietario de la unidad lo ven; otro propietario no.
	st, q := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/reservas/%d/qr", enCurso), prop, nil)
	contenido, _ := q["contenido"].(string)
	if st != 200 || !strings.HasPrefix(contenido, "EDISYS-R:") || !strings.HasPrefix(fmt.Sprint(q["png"]), "data:image/png;base64,") {
		t.Fatalf("qr del propietario: %d %v", st, q)
	}
	ajena := nueva("R-9002", "202", ahora.Add(48*time.Hour), 2, "confirmada", "")
	if st, _ := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/reservas/%d/qr", ajena), prop, nil); st != 404 {
		t.Errorf("qr ajeno: %d, quiero 404", st)
	}

	// El propietario no hace check-in; el conserje (operario) sí.
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/checkin", prop, map[string]any{"codigo": contenido}); st != 403 {
		t.Errorf("propietario haciendo check-in: %d", st)
	}
	st, v := e.pedir("GET", "/api/v1/edificios/1/checkin?codigo="+contenido, ope, nil)
	if st != 200 || v["valido"] != true {
		t.Fatalf("vista previa: %d %v", st, v)
	}
	st, v = e.pedir("POST", "/api/v1/edificios/1/checkin", ope, map[string]any{"codigo": contenido})
	if st != 200 || v["valido"] != true {
		t.Fatalf("check-in válido: %d %v", st, v)
	}
	var valido *bool
	var por *int64
	_ = e.pool.QueryRow(ctx, `SELECT checkin_valido, checkin_por FROM reserva WHERE id=$1`, enCurso).Scan(&valido, &por)
	if valido == nil || !*valido || por == nil {
		t.Errorf("la reserva no guardó el ingreso: %v %v", valido, por)
	}
	if st, v := e.pedir("POST", "/api/v1/edificios/1/checkin", ope, map[string]any{"codigo": "r-9001"}); st != 409 || codigo(v) != "CHECKIN_REPETIDO" {
		t.Errorf("segundo ingreso: %d %v", st, v)
	}
	// La base tampoco deja reescribir un ingreso válido.
	if _, err := e.pool.Exec(ctx, `UPDATE reserva SET checkin_en=now() WHERE id=$1`, enCurso); err == nil || !strings.Contains(err.Error(), "EDR04") {
		t.Errorf("la base reescribió el ingreso: %v", err)
	}

	// Moroso el día del evento (reservó con permiso, hoy debe): rechazado, y solo el admin lo fuerza con motivo.
	morosa := nueva("R-9003", "402", ahora.Add(-30*time.Minute), 2, "confirmada", "Autorizada por la junta")
	st, v = e.pedir("POST", "/api/v1/edificios/1/checkin", ope, map[string]any{"codigo": "R-9003"})
	if st != 200 || v["valido"] != false || !tieneMotivoRsv(v, "MOROSO") {
		t.Fatalf("check-in moroso: %d %v", st, v)
	}
	_ = e.pool.QueryRow(ctx, `SELECT checkin_valido FROM reserva WHERE id=$1`, morosa).Scan(&valido)
	if valido == nil || *valido {
		t.Errorf("el rechazo no quedó registrado: %v", valido)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE reserva SET checkin_en=now(), checkin_valido=true WHERE id=$1`, morosa); err == nil || !strings.Contains(err.Error(), "EDR03") {
		t.Errorf("la base dejó entrar al moroso: %v", err)
	}
	if st, _ := e.pedir("POST", "/api/v1/edificios/1/checkin", ope, map[string]any{"codigo": "R-9003", "forzar_motivo": "Pagó en efectivo"}); st != 403 {
		t.Errorf("el operario no fuerza: %d", st)
	}
	st, v = e.pedir("POST", "/api/v1/edificios/1/checkin", adm, map[string]any{"codigo": "R-9003", "forzar_motivo": "Pagó en efectivo en conserjería"})
	if st != 200 || v["valido"] != true || v["forzado"] != true {
		t.Errorf("ingreso forzado por el admin: %d %v", st, v)
	}
	var n int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM reserva_checkin WHERE reserva_id=$1`, morosa).Scan(&n)
	if n != 2 {
		t.Errorf("bitácora de intentos: %d, quiero 2", n)
	}

	// Fuera de franja: mañana. No se puede forzar.
	manana := nueva("R-9004", "201", ahora.Add(26*time.Hour), 2, "confirmada", "")
	st, v = e.pedir("POST", "/api/v1/edificios/1/checkin", ope, map[string]any{"codigo": "R-9004"})
	if st != 200 || v["valido"] != false || !tieneMotivoRsv(v, "ANTES_DE_FRANJA") {
		t.Errorf("antes de la franja: %d %v", st, v)
	}
	if st, v := e.pedir("POST", "/api/v1/edificios/1/checkin", adm, map[string]any{"codigo": "R-9004", "forzar_motivo": "x"}); st != 422 || codigo(v) != "NO_FORZABLE" {
		t.Errorf("forzar fuera de franja: %d %v", st, v)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE reserva SET checkin_en=now(), checkin_valido=true WHERE id=$1`, manana); err == nil || !strings.Contains(err.Error(), "EDR02") {
		t.Errorf("la base aceptó un ingreso fuera de franja: %v", err)
	}

	// Pendiente de pago: sin entrada.
	pend := nueva("R-9005", "203", ahora.Add(-10*time.Minute), 1, "pendiente_pago", "")
	if st, _ := e.pedir("GET", fmt.Sprintf("/api/v1/edificios/1/reservas/%d/qr", pend), adm, nil); st != 409 {
		t.Errorf("qr de reserva sin pagar: %d", st)
	}
	if st, v := e.pedir("GET", "/api/v1/edificios/1/checkin?codigo=R-9005", ope, nil); st != 200 || !tieneMotivoRsv(v, "PAGO_PENDIENTE") {
		t.Errorf("pendiente de pago: %d %v", st, v)
	}
	// QR de otro edificio o inventado: 404.
	if st, _ := e.pedir("GET", "/api/v1/edificios/1/checkin?codigo=EDISYS-R:ffffffffffffffffffffffffffffffff", ope, nil); st != 404 {
		t.Errorf("qr inventado: %d", st)
	}

	// La lista del día para el conserje.
	st, h := e.pedir("GET", "/api/v1/edificios/1/checkin/hoy", ope, nil)
	if st != 200 || num(h["total"]) < 1 {
		t.Errorf("reservas de hoy: %d %v", st, h)
	}
}

func tieneMotivoRsv(v map[string]any, cod string) bool {
	ms, _ := v["motivos"].([]any)
	for _, m := range ms {
		if mm, ok := m.(map[string]any); ok && mm["codigo"] == cod {
			return true
		}
	}
	return false
}
