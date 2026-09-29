package conciliacion

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestEmparejarPorRegla(t *testing.T) {
	movs := []Mov{
		{Fecha: "2026-09-05", MontoCts: 99000, Codigo: "0030007919"}, // 1 · código (ceros a la izquierda)
		{Fecha: "2026-09-10", MontoCts: 50000},                       // 2 · monto y fecha ±2 (el más cercano)
		{Fecha: "2026-09-20", MontoCts: -1800},                       // 3 · solo monto
		{Fecha: "2026-09-21", MontoCts: 12345},                       // sin pareja
		{Fecha: "2026-09-22", MontoCts: 70000, Codigo: "X1"},         // código con otro monto: sugerido
	}
	items := []Item{
		{Tipo: "pago", ID: 1, Fecha: "2026-09-05", MontoCts: 99000, Codigo: "30007919"},
		{Tipo: "pago", ID: 2, Fecha: "2026-09-01", MontoCts: 50000}, // a 9 días: no entra en la regla 2
		{Tipo: "pago", ID: 3, Fecha: "2026-09-11", MontoCts: 50000}, // a 1 día
		{Tipo: "egreso", ID: 4, Fecha: "2026-08-01", MontoCts: -1800},
		{Tipo: "pago", ID: 5, Fecha: "2026-09-22", MontoCts: 69000, Codigo: "x-1"},
	}
	p := Emparejar(movs, items)
	quiero := []struct {
		item          int
		estado, regla string
	}{{0, Conciliado, ReglaCodigo}, {2, Sugerido, ReglaMontoFecha}, {3, Sugerido, ReglaMonto}, {-1, SinPareja, ""}, {4, Sugerido, ReglaCodigo}}
	for i, q := range quiero {
		if p[i].Item != q.item || p[i].Estado != q.estado || p[i].Regla != q.regla {
			t.Errorf("mov %d: %+v, quiero %+v", i, p[i], q)
		}
	}
	// Un ítem no se usa dos veces.
	p = Emparejar([]Mov{{Fecha: "2026-09-05", MontoCts: 100}, {Fecha: "2026-09-05", MontoCts: 100}}, []Item{{ID: 1, Fecha: "2026-09-05", MontoCts: 100}})
	if p[0].Item != 0 || p[1].Item != -1 {
		t.Errorf("ítem usado dos veces: %+v", p)
	}
}

func TestMontosYFechas(t *testing.T) {
	for in, quiero := range map[string]int64{"1.234,56": 123456, "1,234.56": 123456, "-80.00": -8000, "S/ 80,00": 8000, "(18.00)": -1800, "760": 76000, "1.500": 150000, "": 0} {
		if v, err := MontoCts(in); err != nil || v != quiero {
			t.Errorf("MontoCts(%q) = %d, %v; quiero %d", in, v, err, quiero)
		}
	}
	for in, quiero := range map[string]string{"05/09/2026": "2026-09-05", "2026-09-05": "2026-09-05", "05-09-2026": "2026-09-05", "46270": "2026-09-05"} {
		if v, err := Fecha(in); err != nil || v != quiero {
			t.Errorf("Fecha(%q) = %s, %v", in, v, err)
		}
	}
}

func TestLeerCSVyXLSXConCargoAbono(t *testing.T) {
	csv := "Fecha;Descripción;Nro. operación;Cargo;Abono;Saldo\n05/09/2026;ABONO YAPE;123;;990,00;10.990,00\n06/09/2026;PAGO LUZ;;98,00;;10.892,00\n"
	cab, filas, err := Leer("x.csv", []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	m := AutoMapeo(cab)
	if m.Fecha != "Fecha" || m.Cargo != "Cargo" || m.Abono != "Abono" || m.Codigo != "Nro. operación" || m.Saldo != "Saldo" || !m.Completo() {
		t.Fatalf("mapeo %+v", m)
	}
	movs, errs := Aplicar(cab, filas, m)
	if len(errs) > 0 || len(movs) != 2 || movs[0].MontoCts != 99000 || movs[1].MontoCts != -9800 || movs[0].Codigo != "123" {
		t.Errorf("movs %+v %v", movs, errs)
	}
	if s, ok := SaldoFinal(cab, filas, m); !ok || s != 1089200 {
		t.Errorf("saldo final %d %v", s, ok)
	}
	f := excelize.NewFile()
	_ = f.SetSheetRow("Sheet1", "A1", &[]any{"F. operación", "Concepto", "Importe", "Referencia"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]any{"05/09/2026", "DEPOSITO", "760.00", "BX9"})
	_ = f.SetSheetRow("Sheet1", "A3", &[]any{"30/09/2026", "COMISION", "-18.00", ""})
	var b bytes.Buffer
	_ = f.Write(&b)
	cab, filas, err = Leer("x.xlsx", b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	m = Mapeo{Fecha: "F. operación", Descripcion: "Concepto", Monto: "Importe", Codigo: "Referencia"}
	movs, _ = Aplicar(cab, filas, m)
	if len(movs) != 2 || movs[0].MontoCts != 76000 || movs[1].MontoCts != -1800 || movs[1].Fecha != "2026-09-30" {
		t.Errorf("xlsx %+v", movs)
	}
}
