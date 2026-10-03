package seed

import (
	"fmt"
	"image/color"
	"time"
)

// personal siembra los bloques F1–F3: tres colaboradores con turno (el conserje marca con la cuenta
// del operario), documentos, checklist, la asistencia de la segunda quincena de setiembre y los
// mínimos del almacén sobre los extras con stock del catálogo (sin crear productos nuevos).
func (s *sembrador) personal() {
	admin := s.usuarios["admin@demo.pe"]
	turno := map[string]int64{}
	for _, t := range []struct {
		nombre, entra, sale string
		tol                 int
		dias                string
	}{
		{"Mañana", "07:00", "15:00", 10, "{1,2,3,4,5,6}"},
		{"Tarde", "15:00", "23:00", 10, "{1,2,3,4,5,6}"},
		{"Noche", "23:00", "07:00", 15, "{1,2,3,4,5,6,7}"},
	} {
		turno[t.nombre] = s.id(`INSERT INTO turno (edificio_id, nombre, hora_entrada, hora_salida, tolerancia_min, dias)
			VALUES ($1,$2,$3::time,$4::time,$5,$6::smallint[]) RETURNING id`, s.eid, t.nombre, t.entra, t.sale, t.tol, t.dias)
	}

	type col struct {
		nombre, dni, cargo, tel, turno, cuenta string
		fondo                                  color.RGBA
	}
	cols := []col{
		{"Óscar Operario", "41234567", "Conserje", "51999000007", "Mañana", "operario@demo.pe", color.RGBA{0x0f, 0x5e, 0x63, 0xff}},
		{"Rosa Huamán Ccori", "42345678", "Limpieza", "51999000021", "Tarde", "", color.RGBA{0x6b, 0x4f, 0x9e, 0xff}},
		{"Julio Ramos Paredes", "43456789", "Vigilante nocturno", "51999000022", "Noche", "", color.RGBA{0x37, 0x41, 0x51, 0xff}},
	}
	ids := map[string]int64{}
	fotoMarca := map[string]int64{}
	for i, c := range cols {
		foto := s.foto(fmt.Sprintf("colaborador-%d", i+1), c.fondo, c.nombre, c.cargo)
		var cuenta *int64
		if c.cuenta != "" {
			v := s.usuarios[c.cuenta]
			cuenta = &v
		}
		ids[c.nombre] = s.id(`INSERT INTO colaborador (edificio_id, nombre, tipo_doc, num_doc, cargo, telefono, foto_id, usuario_id, turno_id, fecha_ingreso, creado_por)
			VALUES ($1,$2,'1',$3,$4,$5,$6,$7,$8,'2025-03-01',$9) RETURNING id`, s.eid, c.nombre, c.dni, c.cargo, c.tel, foto, cuenta, turno[c.turno], admin)
		// Una sola foto de marca por colaborador para toda la quincena: basta para la demo.
		fotoMarca[c.nombre] = s.foto(fmt.Sprintf("marca-%d", i+1), c.fondo, "Marca de asistencia", c.nombre)

		cv := s.subir(fmt.Sprintf("cv-%d.pdf", i+1), "application/pdf", []byte("%PDF-1.4\n% CV de demostración\ntrailer<<>>\n%%EOF"))
		s.exec(`INSERT INTO colaborador_documento (edificio_id, colaborador_id, tipo, titulo, archivo_id, visible_propietarios, creado_por)
			VALUES ($1,$2,'cv',$3,$4,true,$5)`, s.eid, ids[c.nombre], "CV de "+c.nombre, cv, admin)
		plame := s.subir(fmt.Sprintf("plame-%d.pdf", i+1), "application/pdf", []byte("%PDF-1.4\n% PLAME de demostración\ntrailer<<>>\n%%EOF"))
		s.exec(`INSERT INTO colaborador_documento (edificio_id, colaborador_id, tipo, titulo, periodo, archivo_id, visible_propietarios, creado_por)
			VALUES ($1,$2,'plame','PLAME setiembre','2026-09',$3,false,$4)`, s.eid, ids[c.nombre], plame, admin)
	}

	for _, t := range []struct{ texto, turno string }{
		{"Revisar bombas de agua y tablero eléctrico", ""},
		{"Abrir portón de cocheras", "Mañana"},
		{"Sacar la basura a la calle", "Tarde"},
		{"Ronda de pisos y azotea", "Noche"},
	} {
		var tid *int64
		if t.turno != "" {
			v := turno[t.turno]
			tid = &v
		}
		s.exec(`INSERT INTO checklist_item (edificio_id, turno_id, texto, orden) VALUES ($1,$2,$3,(SELECT COALESCE(max(orden),0)+1 FROM checklist_item WHERE edificio_id=$1))`, s.eid, tid, t.texto)
	}

	// Asistencia del 16 al 30 de setiembre del conserje y de limpieza: minutos de llegada fijos y
	// repetibles (algunos fuera de la tolerancia) y dos faltas a propósito para el panel.
	llegadas := []int{-5, 3, 12, 0, 8, -2, 25, 4, 1, 15, -8, 6, 9, 2, 30}
	for _, c := range []struct {
		nombre, entra, sale string
		falta               int // día sin marca
	}{{"Óscar Operario", "07:00", "15:00", 23}, {"Rosa Huamán Ccori", "15:00", "23:00", 18}} {
		for i, min := range llegadas {
			dia := 16 + i
			fecha := fmt.Sprintf("2026-09-%02d", dia)
			if dia == c.falta || lima(fecha, "12:00").Weekday() == time.Sunday {
				continue
			}
			entrada := lima(fecha, c.entra).Add(time.Duration(min) * time.Minute)
			salida := lima(fecha, c.sale).Add(time.Duration(i%4) * time.Minute)
			tarde := max(min, 0)
			s.exec(`INSERT INTO asistencia (edificio_id, colaborador_id, fecha, turno_id, entrada_en, entrada_foto_id, salida_en, salida_foto_id, minutos_tarde, puntual, registrado_por)
				SELECT $1, c.id, $2::date, c.turno_id, $3, $4, $5, $4, $6, $6 <= t.tolerancia_min, c.usuario_id
				FROM colaborador c JOIN turno t ON t.id=c.turno_id WHERE c.id=$7`,
				s.eid, fecha, entrada, fotoMarca[c.nombre], salida, tarde, ids[c.nombre])
		}
	}

	// Almacén: mínimos sobre los extras con stock y su saldo inicial en el kárdex (stock = Σ movimientos).
	// La placa de parqueo queda bajo su mínimo para que la alerta se vea en la demo.
	for _, m := range []struct {
		codigo string
		minimo float64
	}{{"EXR-LLA", 5}, {"EXR-PLA", 12}, {"EXR-KEY", 10}} {
		s.exec(`UPDATE producto SET stock_minimo=$3 WHERE edificio_id=$1 AND codigo=$2`, s.eid, m.codigo, m.minimo)
		s.exec(`INSERT INTO almacen_movimiento (edificio_id, producto_id, tipo, delta, saldo, costo_unit_cts, motivo, creado_por, creado_en)
			SELECT $1, id, 'ajuste', stock, stock, costo_cts, 'Saldo inicial del almacén', $3, $4 FROM producto WHERE edificio_id=$1 AND codigo=$2 AND stock > 0`,
			s.eid, m.codigo, admin, lima("2026-09-01", "09:00"))
	}
}
