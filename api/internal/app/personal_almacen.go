package app

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// F3 · almacén sobre el producto de 0017_comercio. No hay tabla de artículos aparte:
// un artículo de almacén es un producto con controla_stock. Cada cambio de stock deja una fila
// en almacen_movimiento con el delta y el saldo que deja (kárdex); producto.stock es ese saldo.
// Las cantidades viajan como número con 2 decimales; dentro se cuentan en centésimas para no
// arrastrar errores de coma flotante.

const sqlArticulo = `SELECT p.id, p.codigo, p.nombre, p.unidad, p.costo_cts, p.controla_stock, p.stock, p.stock_minimo, p.activo,
		(p.controla_stock AND p.stock_minimo > 0 AND p.stock <= p.stock_minimo) AS bajo_minimo,
		c.nombre AS categoria,
		(SELECT to_char(max(m.creado_en) AT TIME ZONE 'America/Lima','YYYY-MM-DD') FROM almacen_movimiento m WHERE m.producto_id=p.id) AS ultimo_movimiento
	FROM producto p LEFT JOIN producto_categoria c ON c.id=p.categoria_id`

// centesimas convierte 2,5 → 250 (redondea lo que pase de dos decimales).
func centesimas(v float64) int64 { return int64(math.Round(v * 100)) }

// listarArticulosAlmacen: GET /almacen/articulos?todos=1 (todos = también los productos sin control de stock)
func (s *Server) listarArticulosAlmacen(w http.ResponseWriter, r *http.Request) {
	todos := r.URL.Query().Get("todos") == "1"
	filas, err := db.Filas(r.Context(), s.DB, sqlArticulo+` WHERE p.edificio_id=$1 AND p.activo AND ($2 OR p.controla_stock)
		ORDER BY (p.controla_stock AND p.stock_minimo > 0 AND p.stock <= p.stock_minimo) DESC, lower(p.nombre)`, edf(r).ID, todos)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// alertasAlmacen: GET /almacen/alertas — artículos en o bajo su stock mínimo.
func (s *Server) alertasAlmacen(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, sqlArticulo+` WHERE p.edificio_id=$1 AND p.activo AND p.controla_stock
			AND p.stock_minimo > 0 AND p.stock <= p.stock_minimo
		ORDER BY (p.stock / p.stock_minimo), lower(p.nombre)`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas, "total": len(filas), "pagina": 1})
}

// crearArticuloAlmacen: POST /almacen/articulos {nombre, codigo, unidad, costo_cts, stock_minimo, stock_inicial}
// Crea el producto con control de stock y precio 0 (es un insumo, no algo que se venda) y, si trae
// stock inicial, su primera entrada en el kárdex.
func (s *Server) crearArticuloAlmacen(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	var in struct {
		Nombre       string  `json:"nombre"`
		Codigo       string  `json:"codigo"`
		Unidad       string  `json:"unidad"`
		CostoCts     int64   `json:"costo_cts"`
		StockMinimo  float64 `json:"stock_minimo"`
		StockInicial float64 `json:"stock_inicial"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Unidad == "" {
		in.Unidad = "unidad"
	}
	nombre, codigo := in.Nombre, in.Codigo
	precio, controla := int64(0), true
	if err := validarProducto(productoIn{Nombre: &nombre, Codigo: &codigo, Unidad: &in.Unidad, PrecioCts: &precio, CostoCts: &in.CostoCts, ControlaStock: &controla}, true); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ev := P.Validacion("Revisa el artículo.")
	if in.StockMinimo < 0 {
		ev.Campo("stock_minimo", "Cero o más.")
	}
	if in.StockInicial < 0 {
		ev.Campo("stock_inicial", "Cero o más.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	codigo = codigoProducto.Replace(strings.ToUpper(strings.TrimSpace(codigo)))
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	uid := ses(r).UsuarioID
	var pid int64
	err = tx.QueryRow(ctx, `INSERT INTO producto (edificio_id, codigo, nombre, unidad, precio_cts, costo_cts, controla_stock, stock, stock_minimo, creado_por)
		VALUES ($1,$2,$3,$4,0,$5,true,0,$6::numeric/100,$7) RETURNING id`,
		e.ID, codigo, strings.TrimSpace(nombre), in.Unidad, in.CostoCts, centesimas(in.StockMinimo), uid).Scan(&pid)
	if err != nil {
		if esUnico(err) {
			P.Fallo(w, r, P.Conflicto("ARTICULO_DUPLICADO", "Ya hay un producto con ese nombre o código."))
			return
		}
		P.Fallo(w, r, err)
		return
	}
	if c := centesimas(in.StockInicial); c > 0 {
		if _, err := moverStock(r, tx, pid, "entrada", c, in.CostoCts, "Stock inicial"); err != nil {
			P.Fallo(w, r, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, map[string]any{"id": pid})
}

// configurarArticuloAlmacen: PUT /almacen/articulos/{pid} {controla_stock?, stock_minimo?}
// Al activar el control sobre un producto que ya traía stock sin kárdex, ese saldo entra como ajuste inicial.
func (s *Server) configurarArticuloAlmacen(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	pid := idURL(r, "pid")
	var in struct {
		ControlaStock *bool    `json:"controla_stock"`
		StockMinimo   *float64 `json:"stock_minimo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.StockMinimo != nil && *in.StockMinimo < 0 {
		P.Fallo(w, r, P.Validacion("El stock mínimo no puede ser negativo.").Campo("stock_minimo", "Cero o más."))
		return
	}
	var minimo *int64
	if in.StockMinimo != nil {
		v := centesimas(*in.StockMinimo)
		minimo = &v
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	var antes bool
	var stock int64
	if err := tx.QueryRow(ctx, `SELECT controla_stock, (stock*100)::bigint FROM producto WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, pid, e.ID).Scan(&antes, &stock); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el producto"))
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE producto SET controla_stock=COALESCE($2,controla_stock),
			stock_minimo=COALESCE($3::numeric/100,stock_minimo), actualizado_en=now() WHERE id=$1`, pid, in.ControlaStock, minimo); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if !antes && in.ControlaStock != nil && *in.ControlaStock && stock > 0 {
		var hay bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM almacen_movimiento WHERE producto_id=$1)`, pid).Scan(&hay)
		if !hay {
			if _, err := tx.Exec(ctx, `INSERT INTO almacen_movimiento (edificio_id, producto_id, tipo, delta, saldo, motivo, creado_por)
				VALUES ($1,$2,'ajuste',$3::numeric/100,$3::numeric/100,'Saldo inicial al activar el control de stock',$4)`, e.ID, pid, stock, ses(r).UsuarioID); err != nil {
				P.Fallo(w, r, err)
				return
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	fila, err := db.Fila(ctx, s.DB, sqlArticulo+` WHERE p.id=$1`, pid)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, fila)
}

// moverStock bloquea el producto, aplica el movimiento y devuelve el id del movimiento.
// tipo entrada/salida: cant = cantidad en centésimas (> 0). tipo ajuste: cant = conteo físico (≥ 0).
func moverStock(r *http.Request, tx pgx.Tx, pid int64, tipo string, cant, costoCts int64, motivo string) (map[string]any, error) {
	ctx, e := r.Context(), edf(r)
	var controla, activo bool
	var stock, minimo int64
	var nombre string
	err := tx.QueryRow(ctx, `SELECT nombre, controla_stock, activo, (stock*100)::bigint, (stock_minimo*100)::bigint
		FROM producto WHERE id=$1 AND edificio_id=$2 FOR UPDATE`, pid, e.ID).Scan(&nombre, &controla, &activo, &stock, &minimo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, P.NoEncontrado("el artículo")
	}
	if err != nil {
		return nil, err
	}
	if !controla || !activo {
		return nil, P.Validacion("Ese producto no lleva control de stock.").Campo("producto_id", "Activa el control de stock primero.")
	}
	var delta int64
	switch tipo {
	case "entrada":
		delta = cant
	case "salida":
		delta = -cant
		if stock+delta < 0 {
			return nil, P.Conflicto("STOCK_INSUFICIENTE", "No hay stock suficiente de «"+nombre+"»: quedan "+strconv.FormatFloat(float64(stock)/100, 'f', -1, 64)+".").
				Con("disponible", float64(stock)/100)
		}
	case "ajuste":
		delta = cant - stock
		if delta == 0 {
			return nil, P.Validacion("El conteo coincide con el stock: no hay nada que ajustar.").Campo("cantidad", "Sin diferencia.")
		}
	}
	saldo := stock + delta
	var mid int64
	if err := tx.QueryRow(ctx, `INSERT INTO almacen_movimiento (edificio_id, producto_id, tipo, delta, saldo, costo_unit_cts, motivo, creado_por)
		VALUES ($1,$2,$3,$4::numeric/100,$5::numeric/100,$6,$7,$8) RETURNING id`, e.ID, pid, tipo, delta, saldo, costoCts, motivo, ses(r).UsuarioID).Scan(&mid); err != nil {
		return nil, err
	}
	// La entrada con costo actualiza el último costo del producto.
	if _, err := tx.Exec(ctx, `UPDATE producto SET stock=$2::numeric/100,
			costo_cts=CASE WHEN $3::boolean AND $4::bigint > 0 THEN $4::bigint ELSE costo_cts END, actualizado_en=now() WHERE id=$1`,
		pid, saldo, tipo == "entrada", costoCts); err != nil {
		return nil, err
	}
	out := map[string]any{"id": mid, "producto_id": pid, "tipo": tipo, "delta": float64(delta) / 100, "saldo": float64(saldo) / 100}
	if minimo > 0 && saldo <= minimo {
		out["alerta"] = map[string]any{"producto": nombre, "stock": float64(saldo) / 100, "stock_minimo": float64(minimo) / 100}
	}
	return out, nil
}

// registrarMovimientoAlmacen: POST /almacen/movimientos {producto_id, tipo, cantidad, costo_unit_cts, motivo}
// El ajuste (conteo físico) pide además almacen.administrar.
func (s *Server) registrarMovimientoAlmacen(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	var in struct {
		ProductoID   int64   `json:"producto_id"`
		Tipo         string  `json:"tipo"`
		Cantidad     float64 `json:"cantidad"`
		CostoUnitCts int64   `json:"costo_unit_cts"`
		Motivo       string  `json:"motivo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	in.Motivo = strings.TrimSpace(in.Motivo)
	ev := P.Validacion("Revisa el movimiento.")
	switch in.Tipo {
	case "entrada", "salida":
		if centesimas(in.Cantidad) <= 0 {
			ev.Campo("cantidad", "Mayor que cero.")
		}
	case "ajuste":
		if !e.Puede("almacen.administrar") {
			P.Fallo(w, r, P.Prohibido("SIN_PERMISO", "Ajustar el inventario lo hace la administración.").Con("permiso", "almacen.administrar"))
			return
		}
		if in.Cantidad < 0 {
			ev.Campo("cantidad", "El conteo no puede ser negativo.")
		}
	default:
		ev.Campo("tipo", "entrada, salida o ajuste.")
	}
	if in.CostoUnitCts < 0 {
		ev.Campo("costo_unit_cts", "Cero o más.")
	}
	if in.Tipo != "entrada" && in.Motivo == "" {
		ev.Campo("motivo", "Indica para qué sale o por qué se ajusta.")
	}
	if len(ev.Campos) > 0 {
		P.Fallo(w, r, ev)
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	mov, err := moverStock(r, tx, in.ProductoID, in.Tipo, centesimas(in.Cantidad), in.CostoUnitCts, in.Motivo)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusCreated, mov)
}

// listarMovimientosAlmacen: GET /almacen/movimientos?producto_id=&pagina=
func (s *Server) listarMovimientosAlmacen(w http.ResponseWriter, r *http.Request) {
	e, ctx := edf(r), r.Context()
	pagina, porPagina := paginacion(r)
	pid, _ := strconv.ParseInt(r.URL.Query().Get("producto_id"), 10, 64)
	var total int64
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM almacen_movimiento WHERE edificio_id=$1 AND ($2=0 OR producto_id=$2)`, e.ID, pid).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	filas, err := db.Filas(ctx, s.DB, `SELECT m.id, m.producto_id, p.nombre AS producto, p.unidad, m.tipo, m.delta, m.saldo, m.costo_unit_cts, m.motivo,
			u.nombre AS usuario, to_char(m.creado_en AT TIME ZONE 'America/Lima','YYYY-MM-DD"T"HH24:MI') AS fecha
		FROM almacen_movimiento m JOIN producto p ON p.id=m.producto_id LEFT JOIN usuario u ON u.id=m.creado_por
		WHERE m.edificio_id=$1 AND ($2=0 OR m.producto_id=$2)
		ORDER BY m.id DESC LIMIT $3 OFFSET $4`, e.ID, pid, porPagina, (pagina-1)*porPagina)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, paginado(filas, total, pagina))
}
