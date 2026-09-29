package e5

import (
	"os"
	"regexp"
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

var reRSS = regexp.MustCompile(`(VmHWM|RssAnon):\s+\d+ kB`)

func rss() string {
	b, _ := os.ReadFile("/proc/self/status")
	return string(regexp.MustCompile(`\s+`).ReplaceAll([]byte(reRSS.FindAllString(string(b), -1)[0]+" "+reRSS.FindAllString(string(b), -1)[1]), []byte(" ")))
}

// Diagnóstico de RAM por pieza (MEDIR=1): tokenizer vs sesión ONNX.
func TestMedirMemoria(t *testing.T) {
	if os.Getenv("MEDIR") == "" {
		t.Skip("MEDIR no definido")
	}
	t.Logf("inicio:     %s", rss())
	tok, err := CargarUnigram(os.Getenv("E5_TOK"), MaxTokens)
	if err != nil {
		t.Fatal(err)
	}
	_ = tok.Encode("hola")
	t.Logf("tokenizer:  %s", rss())
	liberarHeap()
	t.Logf("+trim:      %s", rss())
	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(os.Getenv("ORT_LIB"))
		if err := ort.InitializeEnvironment(); err != nil {
			t.Fatal(err)
		}
	}
	opts, _ := ort.NewSessionOptions()
	_ = opts.SetCpuMemArena(false)
	if os.Getenv("SIN_PATRON") != "" {
		_ = opts.SetMemPattern(false)
	}
	s, err := ort.NewDynamicAdvancedSession(os.Getenv("E5_RUTA"), []string{"input_ids", "attention_mask", "token_type_ids"}, []string{"last_hidden_state"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Destroy()
	t.Logf("sesión:     %s", rss())
	liberarHeap()
	t.Logf("+trim:      %s", rss())
}
