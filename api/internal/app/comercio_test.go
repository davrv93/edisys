package app_test

import (
	"context"
	"strconv"
	"testing"
)

// Comercio · bloque 1: catálogo de productos y clientes del punto de venta.

func TestComercioCatalogo(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")

	// La semilla deja el catálogo del edificio: alquileres, servicios, espacios y extras.
	st, d := e.pedir("GET", "/api/v1/edificios/1/productos", tok, nil)
	if st != 200 {
		t.Fatalf("listar productos: %d %v", st, d)
	}
	if num(d["total"]) != 18 {
		t.Fatalf("la semilla debe dejar 18 productos, hay %d", num(d["total"]))
	}

	// Buscar y filtrar por categoría.
	st, d = e.pedir("GET", "/api/v1/edificios/1/productos?buscar=alquiler", tok, nil)
	if st != 200 || num(d["total"]) != 4 {
		t.Fatalf("buscar alquiler: %d %d %v", st, num(d["total"]), d)
	}

	var extras int64
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM producto_categoria WHERE nombre='Extras' AND edificio_id=1`).Scan(&extras); err != nil {
		t.Fatal(err)
	}
	st, d = e.pedir("GET", "/api/v1/edificios/1/productos?categoria="+strconv.FormatInt(extras, 10), tok, nil)
	if st != 200 || num(d["total"]) != 5 {
		t.Fatalf("filtrar por categoría: %d %d %v", st, num(d["total"]), d)
	}

	// Alta de producto con código en minúsculas: se guarda en mayúsculas y no se repite.
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{
		"nombre": "Alquiler de sala de maquinas", "codigo": "srv-luz", "unidad": "mes", "precio_cts": 120000, "afectacion": "gravado",
	})
	if st != 201 {
		t.Fatalf("crear producto: %d %v", st, d)
	}
	pid := num(d["id"])
	if d["codigo"] != "SRV-LUZ" || d["unidad"] != "mes" || num(d["precio_cts"]) != 120000 || d["afectacion"] != "gravado" {
		t.Fatalf("producto creado raro: %v", d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{
		"nombre": "Otro producto", "codigo": "srv-luz", "precio_cts": 1000,
	})
	if st != 409 || codigo(d) != "DUPLICADO" {
		t.Fatalf("código repetido: %d %v", st, d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{"nombre": "Repetido", "precio_cts": 1000})
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{"nombre": " Repetido ", "precio_cts": 1000})
	if st != 409 || codigo(d) != "DUPLICADO" {
		t.Fatalf("nombre repetido: %d %v", st, d)
	}

	// Validaciones con 422 y campo señalado.
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{"nombre": " ", "precio_cts": 100})
	if st != 422 || codigo(d) != "VALIDACION" {
		t.Fatalf("nombre vacío: %d %v", st, d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{"nombre": "Precio negativo", "precio_cts": -5})
	if st != 422 {
		t.Fatalf("precio negativo: %d %v", st, d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{"nombre": "Unidad rara", "precio_cts": 100, "unidad": "galón"})
	if st != 422 {
		t.Fatalf("unidad inválida: %d %v", st, d)
	}
	st, d = e.pedir("POST", "/api/v1/edificios/1/productos", tok, map[string]any{"nombre": "Falta precio"})
	if st != 422 {
		t.Fatalf("sin precio: %d %v", st, d)
	}

	// El PUT es parcial: lo que no viene no cambia.
	st, d = e.pedir("PUT", "/api/v1/edificios/1/productos/"+strconv.FormatInt(pid, 10), tok, map[string]any{"precio_cts": 135000})
	if st != 200 || num(d["precio_cts"]) != 135000 || d["nombre"] != "Alquiler de sala de maquinas" {
		t.Fatalf("editar precio: %d %v", st, d)
	}

	// Desactivar saca del catálogo pero no borra la fila.
	st, d = e.pedir("DELETE", "/api/v1/edificios/1/productos/"+strconv.FormatInt(pid, 10), tok, nil)
	if st != 200 {
		t.Fatalf("desactivar: %d %v", st, d)
	}
	st, d = e.pedir("GET", "/api/v1/edificios/1/productos", tok, nil)
	if num(d["total"]) != 19 {
		t.Fatalf("el producto desactivado no debe contar en el catálogo: %d", num(d["total"]))
	}
	st, d = e.pedir("GET", "/api/v1/edificios/1/productos?todos=1", tok, nil)
	if num(d["total"]) != 20 {
		t.Fatalf("con inactivos deben verse 20: %d", num(d["total"]))
	}
	var borrados int64
	_ = e.pool.QueryRow(context.Background(), `SELECT count(*) FROM producto WHERE id=$1`, pid).Scan(&borrados)
	if borrados != 1 {
		t.Fatal("desactivar no debe borrar el producto")
	}

	// Categorías: se crean, se listan con su conteo y se desactivan.
	st, d = e.pedir("POST", "/api/v1/edificios/1/categorias", tok, map[string]any{"nombre": "Bodega", "orden": 5})
	if st != 201 {
		t.Fatalf("crear categoría: %d %v", st, d)
	}
	cid := num(d["id"])
	st, d = e.pedir("POST", "/api/v1/edificios/1/categorias", tok, map[string]any{"nombre": "bodega"})
	if st != 409 {
		t.Fatalf("categoría repetida: %d %v", st, d)
	}
	st, d = e.pedir("GET", "/api/v1/edificios/1/categorias", tok, nil)
	if st != 200 || num(d["datos"].([]any)[0].(map[string]any)["productos"]) != 4 {
		t.Fatalf("listar categorías: %d %v", st, d)
	}
	st, d = e.pedir("DELETE", "/api/v1/edificios/1/categorias/"+strconv.FormatInt(cid, 10), tok, nil)
	if st != 200 {
		t.Fatalf("desactivar categoría: %d %v", st, d)
	}
	var activa bool
	_ = e.pool.QueryRow(context.Background(), `SELECT activo FROM producto_categoria WHERE id=$1`, cid).Scan(&activa)
	if activa {
		t.Fatal("la categoría debe quedar inactiva")
	}
}

func TestComercioPermisos(t *testing.T) {
	e := nuevo(t)

	// El operario atiende el mostrador: ve el catálogo y los clientes, no crea productos.
	operario := e.login("operario@demo.pe")
	if st, d := e.pedir("GET", "/api/v1/edificios/1/productos", operario, nil); st != 200 {
		t.Fatalf("operario ve el catálogo: %d %v", st, d)
	}
	if st, d := e.pedir("POST", "/api/v1/edificios/1/productos", operario, map[string]any{"nombre": "X", "precio_cts": 100}); st != 403 || codigo(d) != "SIN_PERMISO" {
		t.Fatalf("operario no crea productos: %d %v", st, d)
	}
	if st, d := e.pedir("GET", "/api/v1/edificios/1/clientes", operario, nil); st != 200 {
		t.Fatalf("operario ve clientes: %d %v", st, d)
	}

	// La junta ve el edificio, no el mostrador.
	junta := e.login("junta@demo.pe")
	if st, d := e.pedir("GET", "/api/v1/edificios/1/productos", junta, nil); st != 403 || codigo(d) != "SIN_PERMISO" {
		t.Fatalf("junta no ve el catálogo: %d %v", st, d)
	}
	if st, d := e.pedir("GET", "/api/v1/edificios/1/clientes", junta, nil); st != 403 {
		t.Fatalf("junta no ve clientes: %d %v", st, d)
	}
}

func TestComercioClientes(t *testing.T) {
	e := nuevo(t)
	tok := e.login("admin@demo.pe")

	st, d := e.pedir("GET", "/api/v1/edificios/1/clientes?buscar=pinos", tok, nil)
	if st != 200 || num(d["total"]) != 1 {
		t.Fatalf("buscar cliente: %d %d %v", st, num(d["total"]), d)
	}
	if d["datos"].([]any)[0].(map[string]any)["tipo_doc"] != "6" {
		t.Fatalf("el RUC debe quedar como tipo 6: %v", d)
	}

	// Un documento no se repite en el mismo edificio.
	st, d = e.pedir("POST", "/api/v1/edificios/1/clientes", tok, map[string]any{"nombre": "Otro con el mismo RUC", "num_doc": "20600000048"})
	if st != 409 || codigo(d) != "DUPLICADO" {
		t.Fatalf("documento repetido: %d %v", st, d)
	}

	// Sin tipo explícito, el documento manda: 8 dígitos es DNI (tipo 1), RUC válido es 6.
	st, d = e.pedir("POST", "/api/v1/edificios/1/clientes", tok, map[string]any{"nombre": "Nuevo inquilino", "num_doc": "41234567"})
	if st != 201 || d["tipo_doc"] != "1" || d["num_doc"] != "41234567" {
		t.Fatalf("crear cliente: %d %v", st, d)
	}
	cid := num(d["id"])

	st, d = e.pedir("POST", "/api/v1/edificios/1/clientes", tok, map[string]any{"nombre": "Con correo raro", "correo": "no-es-correo"})
	if st != 422 {
		t.Fatalf("correo inválido: %d %v", st, d)
	}

	st, d = e.pedir("PUT", "/api/v1/edificios/1/clientes/"+strconv.FormatInt(cid, 10), tok, map[string]any{"telefono": "51999000333"})
	if st != 200 || d["telefono"] != "51999000333" || d["nombre"] != "Nuevo inquilino" {
		t.Fatalf("editar cliente: %d %v", st, d)
	}

	// Un cliente de otro edificio no se toca: el TenantMiddleware resuelve el edificio, no el id.
	var otroEdificio, clienteAjeno int64
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO edificio (administradora_id, nombre) SELECT id, 'Otro' FROM administradora LIMIT 1 RETURNING id`).Scan(&otroEdificio); err != nil {
		t.Fatal(err)
	}
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO cliente (edificio_id, nombre) VALUES ($1, 'Cliente ajeno') RETURNING id`, otroEdificio).Scan(&clienteAjeno); err != nil {
		t.Fatal(err)
	}
	st, _ = e.pedir("PUT", "/api/v1/edificios/1/clientes/"+strconv.FormatInt(clienteAjeno, 10), tok, map[string]any{"telefono": "1"})
	if st == 200 {
		t.Fatalf("no se debe editar un cliente de otro edificio: %d", st)
	}
}
