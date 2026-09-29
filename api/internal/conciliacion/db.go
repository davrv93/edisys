package conciliacion

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
)

// rango del periodo con 2 días de margen a cada lado (la regla de fecha ±2).
func rango(periodo string) (string, string) {
	t, _ := time.Parse("2006-01", periodo)
	return t.AddDate(0, 0, -2).Format("2006-01-02"), t.AddDate(0, 1, 1).Format("2006-01-02")
}

// ItemsSistema: pagos validados (un ítem por operación: las partes de un pago repartido se suman) y egresos
// con fecha en el periodo (±2 días) que no estén ya emparejados con un movimiento de otro extracto.
func ItemsSistema(ctx context.Context, q db.Q, eid int64, periodo string, extracto int64) ([]Item, error) {
	desde, hasta := rango(periodo)
	filas, err := q.Query(ctx, `
		SELECT 'pago', pg.id, to_char(pg.fecha,'YYYY-MM-DD'),
		       CASE WHEN COALESCE(pg.codigo_operacion,'') = '' THEN pg.monto_cts ELSE
		         (SELECT sum(p2.monto_cts) FROM pago p2 WHERE p2.edificio_id=pg.edificio_id AND p2.estado='validado' AND p2.medio=pg.medio
		            AND p2.fecha=pg.fecha AND p2.codigo_operacion=pg.codigo_operacion) END,
		       COALESCE(pg.codigo_operacion,''), 'Pago ' || pg.medio || ' · Dpto ' || u.codigo || ' · ' || COALESCE(r.numero,'')
		FROM pago pg JOIN recibo r ON r.id=pg.recibo_id JOIN unidad u ON u.id=r.unidad_id
		WHERE pg.edificio_id=$1 AND pg.estado='validado' AND pg.parte=1 AND pg.fecha BETWEEN $2 AND $3
		  AND NOT EXISTS (SELECT 1 FROM movimiento_banco mb WHERE mb.pago_id=pg.id AND mb.estado <> 'sin_pareja' AND mb.extracto_id <> $4)
		UNION ALL
		SELECT 'egreso', e.id, to_char(e.fecha,'YYYY-MM-DD'), -e.monto_cts, '', e.descripcion
		FROM egreso e WHERE e.edificio_id=$1 AND e.fecha BETWEEN $2 AND $3
		  AND NOT EXISTS (SELECT 1 FROM movimiento_banco mb WHERE mb.egreso_id=e.id AND mb.estado <> 'sin_pareja' AND mb.extracto_id <> $4)
		ORDER BY 3, 1, 2`, eid, desde, hasta, extracto)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	var out []Item
	for filas.Next() {
		var it Item
		if err := filas.Scan(&it.Tipo, &it.ID, &it.Fecha, &it.MontoCts, &it.Codigo, &it.Descripcion); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, filas.Err()
}

// Guardar reemplaza el extracto del periodo y banco, inserta sus movimientos y los empareja.
func Guardar(ctx context.Context, tx pgx.Tx, eid int64, banco, periodo, archivo string, saldoFinal int64, movs []Mov, usuario *int64) (int64, error) {
	if _, err := tx.Exec(ctx, `UPDATE egreso SET movimiento_banco_id=NULL WHERE movimiento_banco_id IN
		(SELECT mb.id FROM movimiento_banco mb JOIN extracto x ON x.id=mb.extracto_id WHERE x.edificio_id=$1 AND x.periodo=$2 AND x.banco=$3)`, eid, periodo, banco); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM extracto WHERE edificio_id=$1 AND periodo=$2 AND banco=$3`, eid, periodo, banco); err != nil {
		return 0, err
	}
	var suma int64
	for _, m := range movs {
		suma += m.MontoCts
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO extracto (edificio_id, banco, periodo, archivo_nombre, saldo_inicial_cts, saldo_final_cts, subido_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, eid, banco, periodo, archivo, saldoFinal-suma, saldoFinal, usuario).Scan(&id); err != nil {
		return 0, err
	}
	for _, m := range movs {
		if _, err := tx.Exec(ctx, `INSERT INTO movimiento_banco (extracto_id, edificio_id, fecha, descripcion, monto_cts, codigo_operacion) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, eid, m.Fecha, m.Descripcion, m.MontoCts, m.Codigo); err != nil {
			return 0, err
		}
	}
	return id, Reemparejar(ctx, tx, eid, id)
}

// Reemparejar corre las tres reglas sobre los movimientos sin pareja del extracto, con los ítems libres.
func Reemparejar(ctx context.Context, tx pgx.Tx, eid, extracto int64) error {
	var periodo string
	if err := tx.QueryRow(ctx, `SELECT periodo FROM extracto WHERE id=$1 AND edificio_id=$2`, extracto, eid).Scan(&periodo); err != nil {
		return err
	}
	items, err := ItemsSistema(ctx, tx, eid, periodo, extracto)
	if err != nil {
		return err
	}
	// Quita los ítems que ya usa este mismo extracto.
	usados := map[string]bool{}
	fu, err := tx.Query(ctx, `SELECT COALESCE(pago_id,0), COALESCE(egreso_id,0) FROM movimiento_banco WHERE extracto_id=$1 AND estado <> 'sin_pareja'`, extracto)
	if err != nil {
		return err
	}
	for fu.Next() {
		var p, e int64
		_ = fu.Scan(&p, &e)
		usados[fmt.Sprintf("pago%d", p)] = p > 0
		usados[fmt.Sprintf("egreso%d", e)] = e > 0
	}
	fu.Close()
	libres := items[:0]
	for _, it := range items {
		if !usados[fmt.Sprintf("%s%d", it.Tipo, it.ID)] {
			libres = append(libres, it)
		}
	}
	fm, err := tx.Query(ctx, `SELECT id, to_char(fecha,'YYYY-MM-DD'), descripcion, monto_cts, codigo_operacion FROM movimiento_banco
		WHERE extracto_id=$1 AND estado='sin_pareja' ORDER BY fecha, id`, extracto)
	if err != nil {
		return err
	}
	var ids []int64
	var movs []Mov
	for fm.Next() {
		var id int64
		var m Mov
		if err := fm.Scan(&id, &m.Fecha, &m.Descripcion, &m.MontoCts, &m.Codigo); err != nil {
			fm.Close()
			return err
		}
		ids = append(ids, id)
		movs = append(movs, m)
	}
	fm.Close()
	for _, p := range Emparejar(movs, libres) {
		if p.Item < 0 {
			continue
		}
		it := libres[p.Item]
		var pago, egreso *int64
		if it.Tipo == "pago" {
			pago = &it.ID
		} else {
			egreso = &it.ID
		}
		if _, err := tx.Exec(ctx, `UPDATE movimiento_banco SET estado=$2, regla=$3, pago_id=$4, egreso_id=$5,
			confirmado_en = CASE WHEN $2='conciliado' THEN now() END WHERE id=$1`, ids[p.Mov], p.Estado, p.Regla, pago, egreso); err != nil {
			return err
		}
		if egreso != nil && p.Estado == Conciliado {
			_, _ = tx.Exec(ctx, `UPDATE egreso SET movimiento_banco_id=$2 WHERE id=$1`, *egreso, ids[p.Mov])
		}
	}
	return nil
}

// ExtractoDemo arma el CSV de un banco de ejemplo para el periodo: cada pago validado y cada egreso del mes tal como
// los vería el banco (los depósitos en efectivo llegan al día siguiente y sin código), más dos movimientos sin pareja
// a propósito: la comisión de mantenimiento de cuenta (−S/ 18,00) y un depósito en ventanilla del Dpto 104 (S/ 760,00).
// El saldo final es el banco del sistema más esos dos: al resolverlos, la diferencia queda en cero.
func ExtractoDemo(ctx context.Context, q db.Q, eid int64, periodo string, saldoSistema int64) ([]byte, int64, error) {
	t, _ := time.Parse("2006-01", periodo)
	ini, fin := t.Format("2006-01-02"), t.AddDate(0, 1, -1).Format("2006-01-02")
	filas, err := q.Query(ctx, `
		SELECT to_char(pg.fecha + CASE WHEN pg.medio='efectivo' THEN 1 ELSE 0 END,'DD/MM/YYYY'),
		       CASE WHEN pg.medio='efectivo' THEN 'DEPOSITO EFECTIVO' ELSE 'ABONO ' || upper(pg.medio) END,
		       sum(pg.monto_cts), COALESCE(pg.codigo_operacion,''), min(pg.fecha)
		FROM pago pg WHERE pg.edificio_id=$1 AND pg.estado='validado' AND pg.fecha BETWEEN $2 AND $3
		GROUP BY pg.fecha, pg.medio, COALESCE(pg.codigo_operacion,''), CASE WHEN COALESCE(pg.codigo_operacion,'')='' THEN pg.id END
		UNION ALL
		SELECT to_char(fecha,'DD/MM/YYYY'), 'PAGO ' || upper(descripcion), -monto_cts, '', fecha FROM egreso WHERE edificio_id=$1 AND fecha BETWEEN $2 AND $3
		ORDER BY 5, 2`, eid, ini, fin)
	if err != nil {
		return nil, 0, err
	}
	type fila struct {
		fecha, desc, cod string
		monto            int64
	}
	var fs []fila
	for filas.Next() {
		var f fila
		var orden time.Time
		if err := filas.Scan(&f.fecha, &f.desc, &f.monto, &f.cod, &orden); err != nil {
			filas.Close()
			return nil, 0, err
		}
		fs = append(fs, f)
	}
	filas.Close()
	ult := t.AddDate(0, 1, -1)
	fs = append(fs,
		fila{ult.AddDate(0, 0, -1).Format("02/01/2006"), "DEPOSITO VENTANILLA DPTO 104", "", 76000},
		fila{ult.Format("02/01/2006"), "COMISION MANTENIMIENTO DE CUENTA", "", -1800})
	var suma int64
	for _, f := range fs {
		suma += f.monto
	}
	saldoFinal := saldoSistema + 76000 - 1800
	saldo := saldoFinal - suma
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma = ';'
	_ = w.Write([]string{"Fecha", "Descripción", "Nro. operación", "Cargo", "Abono", "Saldo"})
	for _, f := range fs {
		saldo += f.monto
		cargo, abono := "", ""
		if f.monto < 0 {
			cargo = soles(-f.monto)
		} else {
			abono = soles(f.monto)
		}
		_ = w.Write([]string{f.fecha, f.desc, f.cod, cargo, abono, soles(saldo)})
	}
	w.Flush()
	return b.Bytes(), saldoFinal, nil
}

func soles(c int64) string { return strings.Replace(fmt.Sprintf("%d.%02d", c/100, c%100), ".", ",", 1) }

// CargarDemo deja cargado el extracto de ejemplo del periodo (lo usa la semilla).
func CargarDemo(ctx context.Context, tx pgx.Tx, eid int64, periodo string, saldoSistema int64, usuario *int64) (int64, error) {
	datos, _, err := ExtractoDemo(ctx, tx, eid, periodo, saldoSistema)
	if err != nil {
		return 0, err
	}
	cab, filas, err := Leer("extracto.csv", datos)
	if err != nil {
		return 0, err
	}
	m := AutoMapeo(cab)
	movs, errs := Aplicar(cab, filas, m)
	if len(errs) > 0 {
		return 0, fmt.Errorf("extracto demo: %v", errs)
	}
	saldo, _ := SaldoFinal(cab, filas, m)
	if _, err := tx.Exec(ctx, `INSERT INTO banco_mapeo (edificio_id, banco, mapeo) VALUES ($1,'BCP',$2) ON CONFLICT (edificio_id, banco) DO UPDATE SET mapeo=EXCLUDED.mapeo`, eid, m); err != nil {
		return 0, err
	}
	return Guardar(ctx, tx, eid, "BCP", periodo, "extracto-bcp-"+periodo+".csv", saldo, movs, usuario)
}
