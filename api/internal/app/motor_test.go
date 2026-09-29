package app

import "testing"

// Las guardas del GoldenSQL (§4 del plan): si una de estas pasara, es un agujero.
func TestValidaSQL(t *testing.T) {
	oks := []string{
		"SELECT count(*) FROM recibo WHERE edificio_id = :edificio_id",
		"select u.codigo from unidad u join recibo r on r.unidad_id=u.id where u.edificio_id = :edificio_id group by u.codigo",
		"SELECT u.codigo, SUM(r.total_cts-r.pagado_cts) AS saldo_cts FROM unidad u LEFT JOIN recibo r ON r.unidad_id=u.id AND r.estado NOT IN ('borrador','anulado') WHERE u.edificio_id = :edificio_id GROUP BY u.codigo ORDER BY saldo_cts DESC",
		"WITH deudas AS (SELECT unidad_id, SUM(total_cts-pagado_cts) s FROM recibo WHERE estado='emitido' GROUP BY unidad_id), top AS (SELECT unidad_id, s FROM deudas ORDER BY s DESC LIMIT 5) SELECT u.codigo, t.s FROM top t JOIN unidad u ON u.id=t.unidad_id WHERE u.edificio_id = :edificio_id",
	}
	for _, sql := range oks {
		if _, err := validaSQL(sql); err != nil {
			t.Errorf("debería pasar: %s → %v", sql, err)
		}
	}

	malos := [][2]string{ // cada caso: {sql, por qué debe rechazarse}
		{"SELECT * FROM recibo", "sin filtro de edificio"},
		{"DELETE FROM recibo WHERE edificio_id = :edificio_id", "DELETE"},
		{"UPDATE recibo SET pagado_cts=0 WHERE edificio_id = :edificio_id", "UPDATE"},
		{"INSERT INTO recibo (unidad_id) VALUES (1)", "INSERT y sin filtro"},
		{"SELECT * FROM recibo WHERE edificio_id = :edificio_id; DROP TABLE recibo", "sentencia doble"},
		{"SELECT * FROM recibo WHERE edificio_id = :edificio_id -- y cola", "comentario"},
		{"SELECT * /* x */ FROM recibo WHERE edificio_id = :edificio_id", "comentario de bloque"},
		{"SELECT nombre FROM persona WHERE edificio_id = :edificio_id", "tabla de personas (Ley 29733)"},
		{"SELECT correo FROM correo_mensaje WHERE edificio_id = :edificio_id", "tabla de correo"},
		{"SELECT pe.nombre FROM usuario pe WHERE pe.edificio_id = :edificio_id", "tabla usuario"},
		{"TRUNCATE recibo", "TRUNCATE"},
		{"SELECT * FROM pg_catalog.pg_tables", "tablas del sistema"},
	}
	for _, c := range malos {
		if _, err := validaSQL(c[0]); err == nil {
			t.Errorf("debería rechazarse (%s): %s", c[1], c[0])
		}
	}
}

// evaluaMotor guía al admin (F5) y al humo: cifra esperada suma, evasiva resta.
func TestEvaluaMotor(t *testing.T) {
	if v := evaluaMotor("¿cuál es la morosidad del edificio?", "La morosidad de setiembre es 13,1 %.", "13,1"); v <= 0 {
		t.Errorf("respuesta buena debería sumar, dio %d", v)
	}
	if v := evaluaMotor("¿cuál es la morosidad?", "No tengo ese dato, escríbele a la administración.", ""); v >= 0 {
		t.Errorf("la evasiva debería dar negativo, dio %d", v)
	}
	if v := evaluaMotor("¿cuánto debe el 402?", "reñ ¯\\_(ツ)_/¯ \ufffd", ""); v > 0 {
		t.Errorf("el mojibake debería dar negativo, dio %d", v)
	}
}
