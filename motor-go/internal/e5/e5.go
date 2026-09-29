// Package e5 calcula embeddings multilingual-e5-small con los MISMOS archivos
// que usaba el motor en Python (e5-model_quantized.onnx + e5-tokenizer.json):
// onnxruntime 1.19.2 (la versión del Python; C API vía github.com/yalue/onnxruntime_go)
// y un tokenizador Unigram en Go puro (unigram.go). Media con máscara y
// normalización L2, igual que _e5() de app.py, para que los vectores guardados
// en faq.json sigan valiendo.
//
// Requiere libonnxruntime.so en tiempo de ejecución (ORT_LIB); se abre con dlopen.
package e5

import (
	"fmt"
	"math"
	"runtime/debug"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// MaxTokens = enable_truncation(max_length=128) de app.py.
const MaxTokens = 128

// Dim del vector de salida.
const Dim = 384

// Embebedor carga modelo y tokenizador una vez y es seguro para uso concurrente
// (serializa las inferencias: el servidor comparte 2 vCPU con llama-server).
type Embebedor struct {
	mu      sync.Mutex
	tok     *Unigram
	sesion  *ort.DynamicAdvancedSession
	entrada []string
}

var iniciarORT sync.Once
var errORT error

// Nuevo carga el ONNX y el tokenizer. libORT es la ruta de libonnxruntime.
func Nuevo(modelo, tokenizer, libORT string, hilos int) (*Embebedor, error) {
	iniciarORT.Do(func() {
		if libORT != "" {
			ort.SetSharedLibraryPath(libORT)
		}
		errORT = ort.InitializeEnvironment()
		if errORT == nil {
			_ = ort.DisableTelemetry()
		}
	})
	if errORT != nil {
		return nil, fmt.Errorf("onnxruntime: %w", errORT)
	}
	tok, err := CargarUnigram(tokenizer, MaxTokens)
	if err != nil {
		return nil, err
	}
	debug.FreeOSMemory() // el JSON de 17 MB ya parseado no debe sumar al pico de ONNX
	// Nombres fijos, como app.py (input_ids, attention_mask, token_type_ids →
	// last_hidden_state). No se usa GetInputOutputInfo: abre una sesión aparte
	// y carga el modelo dos veces, y ese pico no cabía en 400 MiB.
	nombres := []string{"input_ids", "attention_mask", "token_type_ids"}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if hilos > 0 {
		_ = opts.SetIntraOpNumThreads(hilos)
		_ = opts.SetInterOpNumThreads(1)
	}
	// Sin arena: el modelo es pequeño y así la RAM vuelve al sistema entre
	// peticiones (tope del contenedor: 400 MiB).
	_ = opts.SetCpuMemArena(false)
	sesion, err := ort.NewDynamicAdvancedSession(modelo, nombres, []string{"last_hidden_state"}, opts)
	if err != nil {
		return nil, fmt.Errorf("sesión onnx: %w", err)
	}
	liberarHeap() // los temporales de la carga del modelo vuelven al sistema
	return &Embebedor{tok: tok, sesion: sesion, entrada: nombres}, nil
}

// Cerrar libera los recursos nativos.
func (e *Embebedor) Cerrar() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sesion != nil {
		_ = e.sesion.Destroy()
	}
}

// Tokens devuelve los ids (con <s> … </s>, truncados a 128) de un texto.
func (e *Embebedor) Tokens(texto string) []uint32 {
	return e.tok.Encode(texto)
}

// Embeber replica _e5() de app.py: un lote con relleno a la longitud mayor
// (enable_padding() por defecto: pad_id 0, máscara 0, a la derecha), media con
// máscara y normalización L2. El lote importa: la cuantización dinámica del
// ONNX calcula su escala sobre todo el tensor, así que el mismo texto da un
// vector algo distinto (coseno ~0,997) solo que en lote que suelto. Se llama con
// los mismos lotes que Python para obtener los mismos vectores.
func (e *Embebedor) Embeber(textos []string) ([][]float32, error) {
	if len(textos) == 0 {
		return nil, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	cods := make([][]uint32, len(textos))
	largo := 0
	for i, t := range textos {
		cods[i] = e.Tokens(t)
		largo = max(largo, len(cods[i]))
	}
	b, n := int64(len(textos)), int64(largo)
	ids := make([]int64, b*n)
	mask := make([]int64, b*n)
	tipo := make([]int64, b*n) // token_type_ids en cero, como en app.py
	for i, c := range cods {
		for j, x := range c {
			ids[int64(i)*n+int64(j)] = int64(x)
			mask[int64(i)*n+int64(j)] = 1
		}
	}
	forma := ort.NewShape(b, n)
	porNombre := map[string][]int64{"input_ids": ids, "attention_mask": mask, "token_type_ids": tipo}
	valores := make([]ort.Value, 0, len(e.entrada))
	defer func() {
		for _, v := range valores {
			_ = v.Destroy()
		}
	}()
	for _, nombre := range e.entrada {
		datos, ok := porNombre[nombre]
		if !ok {
			return nil, fmt.Errorf("entrada del modelo desconocida: %s", nombre)
		}
		t, err := ort.NewTensor(forma, datos)
		if err != nil {
			return nil, err
		}
		valores = append(valores, t)
	}
	salidas := []ort.Value{nil}
	if err := e.sesion.Run(valores, salidas); err != nil {
		return nil, fmt.Errorf("inferencia e5: %w", err)
	}
	defer salidas[0].Destroy()
	t, ok := salidas[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("salida e5 de tipo inesperado %T", salidas[0])
	}
	datos := t.GetData() // [b, n, dim]
	dim := len(datos) / int(b*n)
	fuera := make([][]float32, b)
	for i := range fuera {
		vec := make([]float32, dim)
		var cuenta float32
		for p := 0; p < int(n); p++ {
			if mask[int64(i)*n+int64(p)] == 0 {
				continue
			}
			cuenta++
			fila := datos[(i*int(n)+p)*dim : (i*int(n)+p+1)*dim]
			for j := range vec {
				vec[j] += fila[j]
			}
		}
		cuenta = max(cuenta, 1e-9)
		for j := range vec {
			vec[j] /= cuenta
		}
		// np.linalg.norm en float32: suma por pares de numpy y sqrt en float32.
		cuad := make([]float32, dim)
		for j, x := range vec {
			cuad[j] = x * x
		}
		norma := max(float32(math.Sqrt(float64(sumaPares(cuad)))), 1e-9)
		for j := range vec {
			vec[j] /= norma
		}
		fuera[i] = vec
	}
	return fuera, nil
}

// sumaPares = pairwise_sum de numpy para float32 (add.reduce sobre un eje
// contiguo): bloques de 8 acumuladores hasta 128 elementos, mitades por encima.
// Replicarla deja los vectores idénticos bit a bit a los de app.py.
func sumaPares(a []float32) float32 {
	n := len(a)
	switch {
	case n < 8:
		var r float32
		for _, x := range a {
			r += x
		}
		return r
	case n <= 128:
		var r [8]float32
		copy(r[:], a[:8])
		i := 8
		for ; i < n-n%8; i += 8 {
			for j := 0; j < 8; j++ {
				r[j] += a[i+j]
			}
		}
		res := ((r[0] + r[1]) + (r[2] + r[3])) + ((r[4] + r[5]) + (r[6] + r[7]))
		for ; i < n; i++ {
			res += a[i]
		}
		return res
	default:
		n2 := n / 2
		n2 -= n2 % 8
		return sumaPares(a[:n2]) + sumaPares(a[n2:])
	}
}
