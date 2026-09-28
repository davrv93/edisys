// Package archivo guarda fotos, vouchers y PDF en un cubo S3 privado (MinIO o Garage) y firma
// URLs que caducan en 10 minutos. Las URLs apuntan al propio API (/api/v1/archivos/{id}),
// que valida la firma y sirve el objeto por la red interna: el cubo nunca se publica.
package archivo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Almacen es lo que el API necesita del S3.
type Almacen interface {
	Subir(ctx context.Context, clave string, datos []byte, mime string) error
	Leer(ctx context.Context, clave string) ([]byte, error)
	Ping(ctx context.Context) error
}

// S3 sobre minio-go (sirve para MinIO, Garage, R2…).
type S3 struct {
	cli    *minio.Client
	bucket string
	region string
}

// NuevoS3 conecta con el endpoint S3.
func NuevoS3(endpoint, access, secret, bucket, region string, ssl bool) (*S3, error) {
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: ssl,
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3{cli: cli, bucket: bucket, region: region}, nil
}

// AsegurarCubo crea el cubo privado si no existe.
func (s *S3) AsegurarCubo(ctx context.Context) error {
	ok, err := s.cli.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return s.cli.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: s.region})
}

// Subir guarda el objeto.
func (s *S3) Subir(ctx context.Context, clave string, datos []byte, mime string) error {
	_, err := s.cli.PutObject(ctx, s.bucket, clave, bytes.NewReader(datos), int64(len(datos)), minio.PutObjectOptions{ContentType: mime})
	return err
}

// Leer trae el objeto completo (fotos ≤ 10 MB).
func (s *S3) Leer(ctx context.Context, clave string) ([]byte, error) {
	obj, err := s.cli.GetObject(ctx, s.bucket, clave, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

// Ping comprueba que el cubo responde.
func (s *S3) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := s.cli.BucketExists(ctx, s.bucket)
	return err
}

// Memoria es un almacén en RAM para pruebas.
type Memoria struct {
	mu   sync.Mutex
	objs map[string][]byte
}

// NuevaMemoria crea el almacén de pruebas.
func NuevaMemoria() *Memoria { return &Memoria{objs: map[string][]byte{}} }

func (m *Memoria) Subir(_ context.Context, clave string, datos []byte, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objs[clave] = append([]byte(nil), datos...)
	return nil
}

func (m *Memoria) Leer(_ context.Context, clave string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.objs[clave]
	if !ok {
		return nil, errors.New("no existe")
	}
	return d, nil
}

func (m *Memoria) Ping(context.Context) error { return nil }

// Firmador crea y valida URLs firmadas con HMAC-SHA256.
type Firmador struct{ Clave []byte }

// Duracion de una URL firmada (Ley 29733: 10 minutos).
const Duracion = 10 * time.Minute

func (f Firmador) firma(id int64, exp int64) string {
	m := hmac.New(sha256.New, f.Clave)
	fmt.Fprintf(m, "archivo:%d:%d", id, exp)
	return hex.EncodeToString(m.Sum(nil))[:32]
}

// URL devuelve la ruta firmada del archivo (relativa al dominio).
func (f Firmador) URL(id int64) string {
	if id == 0 {
		return ""
	}
	exp := time.Now().Add(Duracion).Unix()
	return fmt.Sprintf("/api/v1/archivos/%d?exp=%d&firma=%s", id, exp, f.firma(id, exp))
}

// Valida comprueba firma y vencimiento.
func (f Firmador) Valida(id int64, expTxt, firma string) bool {
	exp, err := strconv.ParseInt(expTxt, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(f.firma(id, exp)), []byte(firma))
}
