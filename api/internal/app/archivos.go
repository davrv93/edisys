package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"edisys/api/internal/db"
	P "edisys/api/internal/plataforma"
)

// MaxArchivo: 10 MB para imágenes y PDF (§2.2).
const MaxArchivo = 10 << 20

var reNombre = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// Subido es un archivo leído de un formulario.
type Subido struct {
	Nombre string
	Mime   string
	Datos  []byte
}

// esMultipart dice si la petición trae un formulario multipart.
func esMultipart(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
}

func leerMultipart(r *http.Request) error {
	if r.MultipartForm != nil {
		return nil
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return P.Validacion("No pude leer el formulario: " + err.Error())
	}
	return nil
}

// archivosDeForm lee los archivos de uno o varios campos (p. ej. "fotos", "fotos[]", "foto").
func archivosDeForm(r *http.Request, campos ...string) ([]Subido, error) {
	if !esMultipart(r) {
		return nil, nil
	}
	if err := leerMultipart(r); err != nil {
		return nil, err
	}
	var out []Subido
	for _, c := range campos {
		for _, fh := range r.MultipartForm.File[c] {
			s, err := leerSubido(fh)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func leerSubido(fh *multipart.FileHeader) (Subido, error) {
	if fh.Size > MaxArchivo {
		return Subido{}, P.Err(http.StatusRequestEntityTooLarge, "ARCHIVO_GRANDE", "El archivo pasa de 10 MB.")
	}
	f, err := fh.Open()
	if err != nil {
		return Subido{}, err
	}
	defer f.Close()
	datos, err := io.ReadAll(io.LimitReader(f, MaxArchivo+1))
	if err != nil {
		return Subido{}, err
	}
	mime := http.DetectContentType(datos)
	if !(strings.HasPrefix(mime, "image/") || mime == "application/pdf") {
		return Subido{}, P.Err(http.StatusUnprocessableEntity, "TIPO_NO_PERMITIDO", "Solo se aceptan fotos (JPG, PNG, WebP) o PDF.").Campo(fh.Filename, "Tipo "+mime+" no permitido.")
	}
	return Subido{Nombre: fh.Filename, Mime: mime, Datos: datos}, nil
}

// campo lee un valor de formulario (multipart o urlencoded).
func campo(r *http.Request, nombre string) string {
	if r.MultipartForm != nil {
		if v := r.MultipartForm.Value[nombre]; len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	return strings.TrimSpace(r.FormValue(nombre))
}

// guardarArchivo sube el objeto al S3 y registra la fila en archivo.
func (s *Server) guardarArchivo(ctx context.Context, q db.Q, eid int64, uid *int64, a Subido) (int64, error) {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	nombre := reNombre.ReplaceAllString(path.Base(a.Nombre), "_")
	if nombre == "" || nombre == "." {
		nombre = "archivo"
	}
	clave := fmt.Sprintf("e%d/%s/%s-%s", eid, time.Now().UTC().Format("2006/01"), hex.EncodeToString(b), nombre)
	if err := s.Almacen.Subir(ctx, clave, a.Datos, a.Mime); err != nil {
		return 0, P.Err(http.StatusServiceUnavailable, "ALMACEN_NO_DISPONIBLE", "No pudimos guardar el archivo. Intenta otra vez.")
	}
	var id int64
	err := q.QueryRow(ctx, `INSERT INTO archivo (edificio_id, clave, nombre, tipo_mime, tamano, subido_por) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		eid, clave, a.Nombre, a.Mime, len(a.Datos), uid).Scan(&id)
	return id, err
}

// url firmada de un archivo (o "" si no hay).
func (s *Server) url(id *int64) any {
	if id == nil || *id == 0 {
		return nil
	}
	return s.Firma.URL(*id)
}

// servirArchivo: GET /archivos/{id}?exp=&firma= — valida la firma (10 min) y sirve el objeto.
func (s *Server) servirArchivo(w http.ResponseWriter, r *http.Request) {
	id, err := idRuta(r, "id")
	if err != nil || !s.Firma.Valida(id, r.URL.Query().Get("exp"), r.URL.Query().Get("firma")) {
		P.Fallo(w, r, P.Prohibido("URL_VENCIDA", "El enlace venció. Vuelve a abrir el documento."))
		return
	}
	var clave, nombre, mime string
	if err := s.DB.QueryRow(r.Context(), `SELECT clave, nombre, tipo_mime FROM archivo WHERE id=$1`, id).Scan(&clave, &nombre, &mime); err != nil {
		P.Fallo(w, r, P.NoEncontrado("el archivo"))
		return
	}
	datos, err := s.Almacen.Leer(r.Context(), clave)
	if err != nil {
		P.Fallo(w, r, P.NoEncontrado("el archivo en el almacén"))
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", reNombre.ReplaceAllString(nombre, "_")))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(datos)
}

type multipartFileHeader = multipart.FileHeader
