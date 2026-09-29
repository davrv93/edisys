package e5

// Tokenizador Unigram (XLM-RoBERTa / multilingual-e5) en Go puro, leído del
// mismo e5-tokenizer.json. Replica el pipeline de HuggingFace tokenizers:
//
//	tokens añadidos (<s>, </s>, …) → Precompiled (charsmap de sentencepiece)
//	→ Replace(" {2,}", " ") → Metaspace(▁, prepend always, MergedWithNext)
//	→ Unigram (Viterbi, fuse_unk) → <s> … </s> con truncado a 128.
//
// Por qué no la librería de Rust: con el vocabulario de 250 002 piezas ocupa
// ~300 MB de RAM (un trie de nodos con HashMap); este ocupa ~30 MB y da los
// mismos ids (verificado contra tokenizers de Python: testdata/tokens_ref.json).

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

const kUnkPenalty = 10.0

// Unigram es el tokenizador cargado.
type Unigram struct {
	piezas    map[string]int32
	puntos    []float64
	maxBytes  int
	unkID     int32
	unkPuntos float64
	bosID     int32
	eosID     int32
	anadidos  []anadido // tokens añadidos no normalizados, en orden de id
	charsmap  *precompilado
	maxTokens int
}

type anadido struct {
	texto string
	id    int32
}

type tokenizerJSON struct {
	AddedTokens []struct {
		ID         int32  `json:"id"`
		Content    string `json:"content"`
		Normalized bool   `json:"normalized"`
		Lstrip     bool   `json:"lstrip"`
		Rstrip     bool   `json:"rstrip"`
		SingleWord bool   `json:"single_word"`
	} `json:"added_tokens"`
	Normalizer struct {
		Type        string `json:"type"`
		Normalizers []struct {
			Type    string `json:"type"`
			Charmap string `json:"precompiled_charsmap"`
			Pattern struct {
				Regex string `json:"Regex"`
			} `json:"pattern"`
			Content string `json:"content"`
		} `json:"normalizers"`
	} `json:"normalizer"`
	PreTokenizer struct {
		Type           string `json:"type"`
		Replacement    string `json:"replacement"`
		AddPrefixSpace *bool  `json:"add_prefix_space"`
		PrependScheme  string `json:"prepend_scheme"`
		Split          *bool  `json:"split"`
	} `json:"pre_tokenizer"`
	Model struct {
		Type         string               `json:"type"`
		UnkID        *int32               `json:"unk_id"`
		ByteFallback bool                 `json:"byte_fallback"`
		Vocab        [][2]json.RawMessage `json:"vocab"`
	} `json:"model"`
}

// CargarUnigram lee un tokenizer.json de XLM-R/e5. Rechaza (con error) las
// variantes que este port no replica, en vez de tokenizar distinto en silencio.
func CargarUnigram(ruta string, maxTokens int) (*Unigram, error) {
	data, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var t tokenizerJSON
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("tokenizer.json: %w", err)
	}
	if t.Model.Type != "Unigram" || t.Model.UnkID == nil || t.Model.ByteFallback {
		return nil, errors.New("tokenizer: se esperaba Unigram con unk_id y sin byte_fallback")
	}
	if t.PreTokenizer.Type != "Metaspace" || t.PreTokenizer.Replacement != "▁" ||
		(t.PreTokenizer.PrependScheme != "" && t.PreTokenizer.PrependScheme != "always") ||
		(t.PreTokenizer.AddPrefixSpace != nil && !*t.PreTokenizer.AddPrefixSpace) ||
		(t.PreTokenizer.Split != nil && !*t.PreTokenizer.Split) {
		return nil, errors.New("tokenizer: pre_tokenizer no soportado (se esperaba Metaspace ▁ always)")
	}
	u := &Unigram{piezas: make(map[string]int32, len(t.Model.Vocab)), unkID: *t.Model.UnkID, maxTokens: maxTokens}
	u.puntos = make([]float64, len(t.Model.Vocab))
	minimo := math.Inf(1)
	for i, par := range t.Model.Vocab {
		var pieza string
		var puntos float64
		if err := json.Unmarshal(par[0], &pieza); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(par[1], &puntos); err != nil {
			return nil, err
		}
		u.piezas[pieza] = int32(i) // HashMap::insert de HF: gana la última
		u.puntos[i] = puntos
		minimo = math.Min(minimo, puntos)
		u.maxBytes = max(u.maxBytes, len(pieza))
	}
	u.unkPuntos = minimo - kUnkPenalty
	for _, a := range t.AddedTokens {
		if a.Normalized || a.Lstrip || a.Rstrip || a.SingleWord {
			return nil, fmt.Errorf("tokenizer: token añadido %q con opciones no soportadas", a.Content)
		}
		u.anadidos = append(u.anadidos, anadido{a.Content, a.ID})
		switch a.Content {
		case "<s>":
			u.bosID = a.ID
		case "</s>":
			u.eosID = a.ID
		}
	}
	if t.Normalizer.Type != "Sequence" || len(t.Normalizer.Normalizers) != 2 ||
		t.Normalizer.Normalizers[0].Type != "Precompiled" ||
		t.Normalizer.Normalizers[1].Type != "Replace" || t.Normalizer.Normalizers[1].Pattern.Regex != " {2,}" ||
		t.Normalizer.Normalizers[1].Content != " " {
		return nil, errors.New("tokenizer: normalizador no soportado (se esperaba Precompiled + Replace)")
	}
	blob, err := base64.StdEncoding.DecodeString(t.Normalizer.Normalizers[0].Charmap)
	if err != nil {
		return nil, err
	}
	if u.charsmap, err = nuevoPrecompilado(blob); err != nil {
		return nil, err
	}
	return u, nil
}

// Encode devuelve los ids con <s> … </s>, truncados a maxTokens (el contenido
// se corta por la derecha a maxTokens-2, como TruncationParams por defecto).
func (u *Unigram) Encode(texto string) []uint32 {
	var ids []uint32
	for _, seg := range u.partirAnadidos(texto) {
		if seg.id >= 0 {
			ids = append(ids, uint32(seg.id))
			continue
		}
		norm := u.normalizar(seg.texto)
		for _, palabra := range metaspace(norm) {
			for _, id := range u.viterbi(palabra) {
				ids = append(ids, uint32(id))
			}
		}
	}
	if u.maxTokens > 2 && len(ids) > u.maxTokens-2 {
		ids = ids[:u.maxTokens-2]
	}
	fuera := make([]uint32, 0, len(ids)+2)
	fuera = append(fuera, uint32(u.bosID))
	fuera = append(fuera, ids...)
	return append(fuera, uint32(u.eosID))
}

type segmento struct {
	texto string
	id    int32 // -1 = texto normal
}

// partirAnadidos separa los tokens añadidos (coincidencia más a la izquierda
// y, a igual inicio, la más larga, como el aho-corasick de HF).
func (u *Unigram) partirAnadidos(s string) []segmento {
	var fuera []segmento
	for len(s) > 0 {
		mejor, largo, id := -1, 0, int32(-1)
		for _, a := range u.anadidos {
			if a.texto == "" {
				continue
			}
			if i := strings.Index(s, a.texto); i >= 0 && (mejor < 0 || i < mejor || (i == mejor && len(a.texto) > largo)) {
				mejor, largo, id = i, len(a.texto), a.id
			}
		}
		if mejor < 0 {
			fuera = append(fuera, segmento{s, -1})
			break
		}
		if mejor > 0 {
			fuera = append(fuera, segmento{s[:mejor], -1})
		}
		fuera = append(fuera, segmento{"", id})
		s = s[mejor+largo:]
	}
	return fuera
}

// normalizar = Precompiled + Replace(" {2,}", " ").
func (u *Unigram) normalizar(s string) string {
	s = u.charsmap.normalizar(s)
	if !strings.Contains(s, "  ") {
		return s
	}
	var b strings.Builder
	espacio := false
	for _, r := range s {
		if r == ' ' {
			if espacio {
				continue
			}
			espacio = true
		} else {
			espacio = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// metaspace: ' ' → '▁', antepone '▁' si no empieza por él y parte con
// MergedWithNext (un '▁' que no sigue a otro '▁' abre pieza nueva).
func metaspace(s string) []string {
	s = strings.ReplaceAll(s, " ", "▁")
	if !strings.HasPrefix(s, "▁") {
		s = "▁" + s
	}
	var fuera []string
	ini, previo := 0, false
	for i, r := range s {
		es := r == '▁'
		if es && !previo && i > ini {
			fuera = append(fuera, s[ini:i])
			ini = i
		}
		previo = es
	}
	return append(fuera, s[ini:])
}

type nodo struct {
	id     int32
	puntos float64
	inicio int // -1 = sin camino
}

// viterbi = Unigram::encode_optimized de HF (fuse_unk = true).
func (u *Unigram) viterbi(s string) []int32 {
	if s == "" {
		return nil
	}
	n := len(s)
	mejor := make([]nodo, n+1)
	for i := range mejor {
		mejor[i].inicio = -1
	}
	mejor[0].inicio = 0
	for ini := 0; ini < n; {
		base := mejor[ini].puntos
		_, mblen := utf8.DecodeRuneInString(s[ini:])
		unaSola := false
		// Prefijos en orden creciente de longitud (como common_prefix_search).
		for fin := ini + 1; fin <= n && fin-ini <= u.maxBytes; fin++ {
			if fin < n && !utf8.RuneStart(s[fin]) {
				continue
			}
			id, ok := u.piezas[s[ini:fin]]
			if !ok {
				continue
			}
			cand := u.puntos[id] + base
			if mejor[fin].inicio < 0 || cand > mejor[fin].puntos {
				mejor[fin] = nodo{id, cand, ini}
			}
			if fin-ini == mblen {
				unaSola = true
			}
		}
		if !unaSola {
			fin := ini + mblen
			cand := u.unkPuntos + base
			if mejor[fin].inicio < 0 || cand > mejor[fin].puntos {
				mejor[fin] = nodo{u.unkID, cand, ini}
			}
		}
		ini += mblen
	}
	// Retroceso; los unk consecutivos se funden en una pieza (que, al no estar
	// en el vocabulario, vuelve a ser unk).
	var piezas []string
	var unk []string
	for fin := n; fin > 0; {
		nd := mejor[fin]
		trozo := s[nd.inicio:fin]
		if nd.id == u.unkID {
			unk = append(unk, trozo)
		} else {
			if len(unk) > 0 {
				piezas = append(piezas, juntarAlReves(unk))
				unk = nil
			}
			piezas = append(piezas, trozo)
		}
		fin = nd.inicio
	}
	if len(unk) > 0 {
		piezas = append(piezas, juntarAlReves(unk))
	}
	ids := make([]int32, len(piezas))
	for i := range piezas {
		p := piezas[len(piezas)-1-i]
		if id, ok := u.piezas[p]; ok {
			ids[i] = id
		} else {
			ids[i] = u.unkID
		}
	}
	return ids
}

func juntarAlReves(xs []string) string {
	var b strings.Builder
	for i := len(xs) - 1; i >= 0; i-- {
		b.WriteString(xs[i])
	}
	return b.String()
}

// ---------- Precompiled (charsmap de sentencepiece: darts double-array) ----------

type precompilado struct {
	trie []uint32
	norm []byte
}

func nuevoPrecompilado(blob []byte) (*precompilado, error) {
	if len(blob) < 4 {
		return nil, errors.New("charsmap corto")
	}
	tam := binary.LittleEndian.Uint32(blob[:4])
	if int(tam)+4 > len(blob) || tam%4 != 0 {
		return nil, errors.New("charsmap inválido")
	}
	trie := make([]uint32, tam/4)
	for i := range trie {
		trie[i] = binary.LittleEndian.Uint32(blob[4+4*i:])
	}
	return &precompilado{trie: trie, norm: blob[4+tam:]}, nil
}

func hasLeaf(u uint32) bool    { return (u>>8)&1 == 1 }
func valor(u uint32) uint32    { return u & ((1 << 31) - 1) }
func etiqueta(u uint32) uint32 { return u & ((1 << 31) | 0xFF) }
func desplaz(u uint32) uint32  { return (u >> 10) << ((u & (1 << 9)) >> 6) }

// primerPrefijo = common_prefix_search(key)[0] (el prefijo MÁS CORTO).
func (p *precompilado) primerPrefijo(key string) (uint32, bool) {
	pos := uint32(0)
	unidad := p.trie[pos]
	pos ^= desplaz(unidad)
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == 0 {
			break
		}
		pos ^= uint32(c)
		if int(pos) >= len(p.trie) {
			return 0, false
		}
		unidad = p.trie[pos]
		if etiqueta(unidad) != uint32(c) {
			return 0, false
		}
		pos ^= desplaz(unidad)
		if int(pos) >= len(p.trie) {
			return 0, false
		}
		if hasLeaf(unidad) {
			return valor(p.trie[pos]), true
		}
	}
	return 0, false
}

func (p *precompilado) transformar(trozo string) (string, bool) {
	i, ok := p.primerPrefijo(trozo)
	if !ok || int(i) > len(p.norm) {
		return "", false
	}
	j := int(i)
	for j < len(p.norm) && p.norm[j] != 0 {
		j++
	}
	return string(p.norm[i:j]), true
}

// normalizar replica Precompiled::normalize de HF: por grafema (< 6 bytes) y,
// si no hay regla para el grafema entero, carácter a carácter.
func (p *precompilado) normalizar(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		graf := g.Str()
		if len(graf) < 6 {
			if t, ok := p.transformar(graf); ok {
				b.WriteString(t)
				continue
			}
		}
		for _, r := range graf {
			parte := string(r)
			if t, ok := p.transformar(parte); ok {
				b.WriteString(t)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}
