package app

import (
	"net/http"
	"strconv"
	"strings"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
	"edisys/api/internal/sunat"
)

// Comercio · punto de venta del edificio. Bloque 1: catálogo (productos y servicios)
// y clientes. La caja y las ventas van en su propio bloque, sobre estas mismas tablas.

const sqlProducto = `SELECT p.id, p.codigo, p.nombre, p.descripcion, p.unidad, p.precio_cts, p.costo_cts,
	p.afectacion, p.controla_stock, p.stock, p.activo, p.categoria_id, c.nombre AS categoria, p.actualizado_en
	FROM producto p LEFT JOIN producto_categoria c ON c.id = p.categoria_id`

// ---------- productos ----------

func (s *Server) listarProductos(w http.ResponseWriter, r *http.Request) {
	ctx, e := r.Context(), edf(r)
	pagina, porPagina := paginacion(r)
	buscar := strings.TrimSpace(r.URL.Query().Get("buscar"))
	var categoria *int64
	if v, err := strconv.ParseInt(r.URL.Query().Get("categoria"), 10, 64); err == nil && v > 0 {
		categoria = &v
	}
	// El catálogo del punto de venta muestra solo lo activo; la pantalla lo pide explícito.
	activos := r.URL.Query().Get("todos") != "1"

	var cond []string
	args := []any{e.ID}
	if activos {
		cond = append(cond, "p.activo")
	}
	if categoria != nil {
		args = append(args, *categoria)
		cond = append(cond, "p.categoria_id = $"+strconv.Itoa(len(args)))
	}
	if buscar != "" {
		args = append(args, "%"+buscar+"%")
		cond = append(cond, "(p.nombre ILIKE $"+strconv.Itoa(len(args))+" OR p.codigo ILIKE $"+strconv.Itoa(len(args))+")")
	}
	where := ""
	if len(cond) > 0 {
		where = " WHERE " + strings.Join(cond, " AND ")
	}

	var total int64
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM producto p`+where, args...).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	args = append(args, porPagina, (pagina-1)*porPagina)
	filas, err := db.Filas(ctx, s.DB, sqlProducto+where+` ORDER BY p.activo DESC, p.nombre LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, paginado(filas, total, pagina))
}

func (s *Server) verProducto(w http.ResponseWriter, r *http.Request) {
	pid, err := idRuta(r, "pid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	f, err := db.Fila(r.Context(), s.DB, sqlProducto+` WHERE p.id=$1 AND p.edificio_id=$2`, pid, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el producto"))
		return
	}
	P.JSON(w, http.StatusOK, f)
}

type productoIn struct {
	Nombre        *string  `json:"nombre"`
	Codigo        *string  `json:"codigo"`
	Descripcion   *string  `json:"descripcion"`
	CategoriaID   *int64   `json:"categoria_id"`
	Unidad        *string  `json:"unidad"`
	PrecioCts     *int64   `json:"precio_cts"`
	CostoCts      *int64   `json:"costo_cts"`
	Afectacion    *string  `json:"afectacion"`
	ControlaStock *bool    `json:"controla_stock"`
	Stock         *float64 `json:"stock"`
	Activo        *bool    `json:"activo"`
}

var unidadesProducto = map[string]bool{"unidad": true, "mes": true, "dia": true, "hora": true, "kg": true, "m2": true}
var afectaciones = map[string]bool{"gravado": true, "exonerado": true, "inafecto": true}

var codigoProducto = strings.NewReplacer(" ", "", "\t", "")

// validarProducto mira lo que el API no puede dejar pasar. Devuelve el error ya listo para responder.
func validarProducto(in productoIn, esNuevo bool) error {
	if in.Nombre != nil {
		if strings.TrimSpace(*in.Nombre) == "" {
			return P.Validacion("Falta el nombre del producto.").Campo("nombre", "Obligatorio.")
		}
		if len(*in.Nombre) > 120 {
			return P.Validacion("El nombre es demasiado largo.").Campo("nombre", "Máximo 120 caracteres.")
		}
	}
	if esNuevo && (in.Nombre == nil || in.PrecioCts == nil) {
		return P.Validacion("El producto necesita nombre y precio.")
	}
	if in.PrecioCts != nil && *in.PrecioCts < 0 {
		return P.Validacion("El precio no puede ser negativo.").Campo("precio_cts", "Cero o más.")
	}
	if in.CostoCts != nil && *in.CostoCts < 0 {
		return P.Validacion("El costo no puede ser negativo.").Campo("costo_cts", "Cero o más.")
	}
	if in.Unidad != nil && !unidadesProducto[*in.Unidad] {
		return P.Validacion("Esa unidad de venta no existe.").Campo("unidad", "Usa una de: unidad, mes, día, hora, kg, m2.")
	}
	if in.Afectacion != nil && !afectaciones[*in.Afectacion] {
		return P.Validacion("Esa afectación no existe.").Campo("afectacion", "Usa gravado, exonerado o inafecto.")
	}
	if in.Codigo != nil {
		c := codigoProducto.Replace(strings.ToUpper(strings.TrimSpace(*in.Codigo)))
		if len(c) > 20 {
			return P.Validacion("El código es demasiado largo.").Campo("codigo", "Máximo 20 caracteres.")
		}
		for _, ch := range c {
			if !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '-' && ch != '.' && ch != '_' {
				return P.Validacion("El código solo admite letras, números, punto y guion.").Campo("codigo", "Sin espacios ni símbolos.")
			}
		}
	}
	if in.Stock != nil && *in.Stock < 0 {
		return P.Validacion("El stock no puede ser negativo.").Campo("stock", "Cero o más.")
	}
	return nil
}

func (s *Server) crearProducto(w http.ResponseWriter, r *http.Request) {
	var in productoIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarProducto(in, true); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx, e := r.Context(), edf(r)
	se := ses(r)
	nombre := strings.TrimSpace(*in.Nombre)
	codigo := ""
	if in.Codigo != nil {
		codigo = codigoProducto.Replace(strings.ToUpper(strings.TrimSpace(*in.Codigo)))
	}
	precio := *in.PrecioCts
	costo := int64(0)
	if in.CostoCts != nil {
		costo = *in.CostoCts
	}
	unidad, afectacion, desc := "unidad", "inafecto", ""
	if in.Unidad != nil {
		unidad = *in.Unidad
	}
	if in.Afectacion != nil {
		afectacion = *in.Afectacion
	}
	if in.Descripcion != nil {
		desc = strings.TrimSpace(*in.Descripcion)
	}
	controla := in.ControlaStock != nil && *in.ControlaStock
	stock := 0.0
	if in.Stock != nil {
		stock = *in.Stock
	}

	var pid int64
	err := s.DB.QueryRow(ctx, `INSERT INTO producto (edificio_id, categoria_id, codigo, nombre, descripcion, unidad,
			precio_cts, costo_cts, afectacion, controla_stock, stock, creado_por)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		e.ID, in.CategoriaID, codigo, nombre, desc, unidad, precio, costo, afectacion, controla, stock, se.UsuarioID).Scan(&pid)
	if err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "crear", "producto", pid, nil, map[string]any{"nombre": nombre, "precio_cts": precio})
	f, _ := db.Fila(ctx, s.DB, sqlProducto+` WHERE p.id=$1`, pid)
	P.JSON(w, http.StatusCreated, f)
}

func (s *Server) editarProducto(w http.ResponseWriter, r *http.Request) {
	pid, err := idRuta(r, "pid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in productoIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarProducto(in, false); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx, e := r.Context(), edf(r)
	antes, err := db.Fila(ctx, s.DB, `SELECT nombre, codigo, precio_cts, activo FROM producto WHERE id=$1 AND edificio_id=$2`, pid, e.ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el producto"))
		return
	}
	if in.Codigo != nil {
		*in.Codigo = codigoProducto.Replace(strings.ToUpper(strings.TrimSpace(*in.Codigo)))
	}
	if in.Nombre != nil {
		*in.Nombre = strings.TrimSpace(*in.Nombre)
	}
	if in.Descripcion != nil {
		*in.Descripcion = strings.TrimSpace(*in.Descripcion)
	}
	// COALESCE deja igual lo que no viene: el PUT es parcial (la app manda solo lo editado).
	err = s.DB.QueryRow(ctx, `UPDATE producto SET categoria_id=COALESCE($1,categoria_id),
			codigo=COALESCE($2,codigo), nombre=COALESCE($3,nombre), descripcion=COALESCE($4,descripcion),
			unidad=COALESCE($5,unidad), precio_cts=COALESCE($6,precio_cts), costo_cts=COALESCE($7,costo_cts),
			afectacion=COALESCE($8,afectacion), controla_stock=COALESCE($9,controla_stock),
			stock=COALESCE($10,stock), activo=COALESCE($11,activo), actualizado_en=now()
		WHERE id=$12 AND edificio_id=$13 RETURNING id`,
		in.CategoriaID, in.Codigo, in.Nombre, in.Descripcion, in.Unidad, in.PrecioCts, in.CostoCts,
		in.Afectacion, in.ControlaStock, in.Stock, in.Activo, pid, e.ID).Scan(&pid)
	if err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "editar", "producto", pid, antes, in)
	f, _ := db.Fila(ctx, s.DB, sqlProducto+` WHERE p.id=$1`, pid)
	P.JSON(w, http.StatusOK, f)
}

// desactivarProducto retira el producto del catálogo sin borrarlo: las ventas viejas lo nombran.
func (s *Server) desactivarProducto(w http.ResponseWriter, r *http.Request) {
	pid, err := idRuta(r, "pid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	antes, err := db.Fila(ctx, s.DB, `SELECT nombre, activo FROM producto WHERE id=$1 AND edificio_id=$2`, pid, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el producto"))
		return
	}
	if _, err := s.DB.Exec(ctx, `UPDATE producto SET activo=false, actualizado_en=now() WHERE id=$1 AND edificio_id=$2`, pid, edf(r).ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "desactivar", "producto", pid, antes, map[string]any{"activo": false})
	P.JSON(w, http.StatusOK, map[string]any{"id": pid, "activo": false})
}

// ---------- categorías ----------

func (s *Server) listarCategorias(w http.ResponseWriter, r *http.Request) {
	filas, err := db.Filas(r.Context(), s.DB, `SELECT c.id, c.nombre, c.orden, c.activo,
			(SELECT count(*) FROM producto p WHERE p.categoria_id=c.id AND p.activo) AS productos
		FROM producto_categoria c WHERE c.edificio_id=$1 ORDER BY c.orden, c.nombre`, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"datos": filas})
}

func (s *Server) crearCategoria(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Nombre string `json:"nombre"`
		Orden  *int   `json:"orden"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if strings.TrimSpace(in.Nombre) == "" {
		P.Fallo(w, r, P.Validacion("Falta el nombre de la categoría.").Campo("nombre", "Obligatorio."))
		return
	}
	ctx := r.Context()
	var cid int64
	if err := s.DB.QueryRow(ctx, `INSERT INTO producto_categoria (edificio_id, nombre, orden)
		VALUES ($1,$2,COALESCE($3,0)) RETURNING id`, edf(r).ID, strings.TrimSpace(in.Nombre), in.Orden).Scan(&cid); err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "crear", "categoria", cid, nil, map[string]any{"nombre": in.Nombre})
	P.JSON(w, http.StatusCreated, map[string]any{"id": cid, "nombre": strings.TrimSpace(in.Nombre)})
}

func (s *Server) editarCategoria(w http.ResponseWriter, r *http.Request) {
	cid, err := idRuta(r, "cid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in struct {
		Nombre *string `json:"nombre"`
		Orden  *int    `json:"orden"`
		Activo *bool   `json:"activo"`
	}
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if in.Nombre != nil && strings.TrimSpace(*in.Nombre) == "" {
		P.Fallo(w, r, P.Validacion("El nombre no puede quedar vacío.").Campo("nombre", "Obligatorio."))
		return
	}
	if in.Nombre != nil {
		*in.Nombre = strings.TrimSpace(*in.Nombre)
	}
	ctx := r.Context()
	if _, err := s.DB.Exec(ctx, `UPDATE producto_categoria SET nombre=COALESCE($1,nombre), orden=COALESCE($2,orden), activo=COALESCE($3,activo)
		WHERE id=$4 AND edificio_id=$5`, in.Nombre, in.Orden, in.Activo, cid, edf(r).ID); err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "editar", "categoria", cid, nil, in)
	P.JSON(w, http.StatusOK, map[string]any{"id": cid})
}

func (s *Server) desactivarCategoria(w http.ResponseWriter, r *http.Request) {
	cid, err := idRuta(r, "cid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	// Desactivar, no borrar: los productos que la usan la siguen viendo en sus ventas.
	if _, err := s.DB.Exec(r.Context(), `UPDATE producto_categoria SET activo=false WHERE id=$1 AND edificio_id=$2`, cid, edf(r).ID); err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, map[string]any{"id": cid, "activo": false})
}

// ---------- clientes ----------

const sqlCliente = `SELECT c.id, c.nombre, c.tipo_doc, c.num_doc, c.direccion, c.telefono, c.correo,
	c.unidad_id, u.codigo AS unidad, c.activo, c.actualizado_en
	FROM cliente c LEFT JOIN unidad u ON u.id = c.unidad_id`

func (s *Server) listarClientes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	buscar := strings.TrimSpace(r.URL.Query().Get("buscar"))
	pagina, porPagina := paginacion(r)
	where := " WHERE c.edificio_id=$1 AND c.activo"
	args := []any{edf(r).ID}
	if buscar != "" {
		args = append(args, "%"+buscar+"%")
		where += " AND (c.nombre ILIKE $" + strconv.Itoa(len(args)) + " OR c.num_doc ILIKE $" + strconv.Itoa(len(args)) + " OR c.telefono ILIKE $" + strconv.Itoa(len(args)) + ")"
	}
	var total int64
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM cliente c`+where, args...).Scan(&total); err != nil {
		P.Fallo(w, r, err)
		return
	}
	args = append(args, porPagina, (pagina-1)*porPagina)
	filas, err := db.Filas(ctx, s.DB, sqlCliente+where+` ORDER BY c.nombre LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	P.JSON(w, http.StatusOK, paginado(filas, total, pagina))
}

type clienteIn struct {
	Nombre    *string `json:"nombre"`
	TipoDoc   *string `json:"tipo_doc"`
	NumDoc    *string `json:"num_doc"`
	Direccion *string `json:"direccion"`
	Telefono  *string `json:"telefono"`
	Correo    *string `json:"correo"`
	UnidadID  *int64  `json:"unidad_id"`
	Activo    *bool   `json:"activo"`
}

func validarCliente(in clienteIn, esNuevo bool) error {
	if in.Nombre != nil && strings.TrimSpace(*in.Nombre) == "" {
		return P.Validacion("El nombre no puede quedar vacío.").Campo("nombre", "Obligatorio.")
	}
	if esNuevo && in.Nombre == nil {
		return P.Validacion("Falta el nombre del cliente.")
	}
	if in.TipoDoc != nil && *in.TipoDoc != "" && !map[string]bool{"0": true, "1": true, "4": true, "6": true, "7": true}[*in.TipoDoc] {
		return P.Validacion("Ese tipo de documento no existe.").Campo("tipo_doc", "Usa DNI, RUC, pasaporte o carnet.")
	}
	if in.Correo != nil && *in.Correo != "" && !strings.Contains(*in.Correo, "@") {
		return P.Validacion("El correo no parece válido.").Campo("correo", "Revisa el correo.")
	}
	return nil
}

func (s *Server) crearCliente(w http.ResponseWriter, r *http.Request) {
	var in clienteIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarCliente(in, true); err != nil {
		P.Fallo(w, r, err)
		return
	}
	nombre := strings.TrimSpace(*in.Nombre)
	tipo, doc := "", ""
	if in.NumDoc != nil {
		doc = strings.TrimSpace(*in.NumDoc)
	}
	if in.TipoDoc != nil {
		tipo = *in.TipoDoc
	}
	if tipo == "" && doc != "" {
		// El documento manda: RUC es factura, DNI es boleta (misma regla que el comprobante).
		_, tipo = sunat.ClienteDe(doc, nombre)
	}
	ctx := r.Context()
	var cid int64
	err := s.DB.QueryRow(ctx, `INSERT INTO cliente (edificio_id, nombre, tipo_doc, num_doc, direccion, telefono, correo, unidad_id, creado_por)
		VALUES ($1,$2,$3,$4,COALESCE($5,''),COALESCE($6,''),COALESCE($7,''),$8,$9) RETURNING id`,
		edf(r).ID, nombre, tipo, doc, in.Direccion, in.Telefono, in.Correo, in.UnidadID, ses(r).UsuarioID).Scan(&cid)
	if err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "crear", "cliente", cid, nil, map[string]any{"nombre": nombre, "num_doc": doc})
	f, _ := db.Fila(ctx, s.DB, sqlCliente+` WHERE c.id=$1`, cid)
	P.JSON(w, http.StatusCreated, f)
}

func (s *Server) editarCliente(w http.ResponseWriter, r *http.Request) {
	cid, err := idRuta(r, "cid")
	if err != nil {
		P.Fallo(w, r, err)
		return
	}
	var in clienteIn
	if err := P.Leer(r, &in); err != nil {
		P.Fallo(w, r, err)
		return
	}
	if err := validarCliente(in, false); err != nil {
		P.Fallo(w, r, err)
		return
	}
	ctx := r.Context()
	antes, err := db.Fila(ctx, s.DB, `SELECT nombre, num_doc, correo, telefono, activo FROM cliente WHERE id=$1 AND edificio_id=$2`, cid, edf(r).ID)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el cliente"))
		return
	}
	if in.Nombre != nil {
		*in.Nombre = strings.TrimSpace(*in.Nombre)
	}
	if in.NumDoc != nil {
		*in.NumDoc = strings.TrimSpace(*in.NumDoc)
	}
	if _, err := s.DB.Exec(ctx, `UPDATE cliente SET nombre=COALESCE($1,nombre), tipo_doc=COALESCE($2,tipo_doc),
			num_doc=COALESCE($3,num_doc), direccion=COALESCE($4,direccion), telefono=COALESCE($5,telefono),
			correo=COALESCE($6,correo), unidad_id=COALESCE($7,unidad_id), activo=COALESCE($8,activo), actualizado_en=now()
		WHERE id=$9 AND edificio_id=$10`,
		in.Nombre, in.TipoDoc, in.NumDoc, in.Direccion, in.Telefono, in.Correo, in.UnidadID, in.Activo, cid, edf(r).ID); err != nil {
		P.Fallo(w, r, P.Traducir(err))
		return
	}
	s.auditarCambio(ctx, s.DB, r, "comercio", "editar", "cliente", cid, antes, in)
	f, _ := db.Fila(ctx, s.DB, sqlCliente+` WHERE c.id=$1`, cid)
	P.JSON(w, http.StatusOK, f)
}
