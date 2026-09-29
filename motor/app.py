#!/usr/bin/env python3
"""EDISYS · motor conversacional. Español de Perú: edificios, cuotas, reservas, mantenimiento.

Fases del plan que viven aquí:
  F1 · chat: ensambla contexto y pregunta a llama-server (:8080, OpenAI-compatible)
  F3 · embeddings e5-small ONNX: recupera FAQ/indexa y marca la intención
  F5 · memoria: correcciones confirmadas por el admin vuelven como contexto
  F6 · KTO-lite online: 👍 refuerza, 👎 penaliza los fragmentos que coincidieron
  F7 · sugerencias: banco curado + similaridad e5 (3 por respuesta)

Todo el estado vive en archivos locales (indice/faq.json, indice/memoria.json);
sin base de datos propia: EDISYS ya tiene Postgres y respaldo, y el motor es efímero.
"""
import json
import math
import re
import threading
import time
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

import numpy as np
import onnxruntime as ort
from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel
from tokenizers import Tokenizer

# ---------- configuración ----------
LLAMA = "http://llama:8080"
CHAT_TIMEOUT = 120.0
E5_RUTA = "/models/e5-model_quantized.onnx"
E5_TOK = "/models/e5-tokenizer.json"
DIM = 384

RECUPERADOS = 2          # fragmentos de FAQ en el contexto
UMBRAL_FAQ = 0.55        # similaridad mínima para traer un fragmento
UMBRAL_MEM = 0.80        # similaridad para usar una corrección de memoria
UMBRAL_ALTO_SIM = 0.93   # respuesta ≈ literal del contexto → posible evasión
UMBRAL_ALTO_VECES = 2    # …si pasa ≥2 veces en el historial → "no lo sé" (F5)
MAX_CORRECCIONES = 50    # tope de la memoria de correcciones
PREFIJO_Q, PREFIJO_P = "query: ", "passage: "   # convención de e5
MAX_HISTORIAL_CHARS = 6000
SUGERENCIAS_N = 3

DIR = Path(__file__).parent
INDICE = DIR / "indice"

app = FastAPI(title="EDISYS · motor", version="1.1.0")

# ---------- estado en memoria ----------
_candado = threading.Lock()
_log = []          # últimas interacciones (para /v1/registro)
_filtro = {}       # clave de intents recuperadas -> {"+1": n, "-1": n} (KTO-lite)
_propuestas = []   # correcciones pendientes de confirmación del admin


# ---------- embeddings (F3) ----------
_sesion = None
_tok = None


def _cargar_e5():
    global _sesion, _tok
    if _sesion is None:
        _sesion = ort.InferenceSession(E5_RUTA, providers=["CPUExecutionProvider"])
        _tok = Tokenizer.from_file(E5_TOK)
        _tok.enable_truncation(max_length=128)
        _tok.enable_padding()  # encode_batch exige tensores rectangulares (longitudes iguales)
    return _sesion, _tok


def _e5(textos: list[str]) -> np.ndarray:
    """Vectores e5 con media y normalización L2 (coseno = producto punto)."""
    sesion, tok = _cargar_e5()
    cod = tok.encode_batch(textos)
    ids = np.array([c.ids for c in cod], dtype=np.int64)
    mask = np.array([c.attention_mask for c in cod], dtype=np.int64)
    tipo = np.zeros_like(mask)  # e5 (BERT) pide también token_type_ids, en cero
    salida = sesion.run(None, {"input_ids": ids, "attention_mask": mask, "token_type_ids": tipo})[0]
    maskf = mask[:, :, None].astype(np.float32)
    vec = (salida * maskf).sum(1) / np.clip(maskf.sum(1), 1e-9, None)
    return vec / np.clip(np.linalg.norm(vec, axis=1, keepdims=True), 1e-9, None)


def _cargar_json(p: Path, defecto):
    if p.exists():
        return json.loads(p.read_text(encoding="utf-8"))
    return defecto


def _guardar_json(p: Path, dato):
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(dato, ensure_ascii=False, indent=1), encoding="utf-8")


def _indexar():
    """Completa los vectores que falten en FAQ y memoria y persiste el índice."""
    faq = _cargar_json(INDICE / "faq.json", [])
    memoria = _cargar_json(INDICE / "memoria.json", [])
    faltan = [f for f in faq if not f.get("vector")] + \
             [m for m in memoria if not m.get("vector")]
    if faltan:
        vecs = _e5([PREFIJO_P + x["texto"] for x in faltan])
        for x, v in zip(faltan, vecs):
            x["vector"] = v.tolist()
        _guardar_json(INDICE / "faq.json", faq)
        _guardar_json(INDICE / "memoria.json", memoria)
    return faq, memoria


def _buscar(consulta: str, banco: list, k: int, umbral: float):
    q = _e5([PREFIJO_Q + consulta])[0]
    pares = []
    for x in banco:
        if not x.get("vector"):
            continue
        sim = float(np.dot(q, np.array(x["vector"], dtype=np.float32)))
        if sim >= umbral:
            pares.append((sim, x))
    pares.sort(key=lambda p: -p[0])
    return pares[:k]


# ---------- llama-server (F1) ----------
def _llama(mensajes: list[dict], max_tokens: int = 280) -> tuple[str, dict]:
    cuerpo = json.dumps({
        "messages": mensajes,
        "temperature": 0.3,
        "max_tokens": max_tokens,
        "cache_prompt": True,
    }).encode()
    peticion = urllib.request.Request(
        LLAMA + "/v1/chat/completions", data=cuerpo,
        headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(peticion, timeout=CHAT_TIMEOUT) as r:
        dato = json.loads(r.read().decode())
    return (dato["choices"][0]["message"]["content"].strip(),
            dato.get("usage", {}))


def _recortar(texto: str, n: int) -> str:
    r = list(texto)
    return texto if len(r) <= n else "".join(r[: n - 1]) + "…"


# ---------- KTO-lite online (F6) ----------
def _clave_filtro(fragmentos: list) -> str:
    return "|".join(sorted(str(f.get("id", "")) for f in fragmentos))


def _aplicar_filtro(texto: str, fragmentos: list) -> str:
    """KTO-lite: si un fragmento fue castigado (👍/👎), quita sus líneas de la respuesta."""
    with _candado:
        pesos = dict(_filtro.get(_clave_filtro(fragmentos), {}))
    if pesos.get("-1", 0) <= pesos.get("+1", 0):
        return texto
    fuente = " ".join(f["texto"] for f in fragmentos).lower()
    lineas = [ln for ln in texto.splitlines()
              if not any(frag in fuente for frag in _pedazos(ln))
              or len(ln.strip()) < 8]
    return "\n".join(lineas).strip() or texto


def _pedazos(linea: str) -> list[str]:
    palabras = [p for p in re.split(r"[\s,;:]+", linea.lower()) if len(p) >= 6]
    return palabras or [linea.lower().strip()]


def _votar_filtro(fragmentos: list, valor: int):
    with _candado:
        clave = _clave_filtro(fragmentos)
        _filtro.setdefault(clave, {"+1": 0, "-1": 0})
        _filtro[clave]["+1" if valor > 0 else "-1"] += 1


# ---------- sugerencias (F7) ----------
BANCO_SUGERENCIAS = [
    "¿Cuánto debo?",
    "¿Mi recibo de este mes ya está pagado?",
    "¿Cómo pago por Yape?",
    "Quiero reservar la parrilla",
    "¿Qué horarios tiene la piscina?",
    "¿Cuál es el aforo de la piscina?",
    "Hay una fuga de agua en mi piso",
    "¿Cuál es la morosidad del edificio?",
    "¿Cuánto hay en la cuenta del banco?",
    "Explícame el reparto de medidores",
]
SUGERENCIAS_INICIO = ["Hola", "¿Cuánto debo?", "Quiero reservar la parrilla"]


def _sugerencias(consulta: str, ya_usadas: list[str]) -> list[str]:
    if not consulta:
        return SUGERENCIAS_INICIO
    # Solo preguntas curadas: los textos de la FAQ son respuestas, no preguntas,
    # y como sugerencias se leerían raro.
    banco = [{"texto": s} for s in BANCO_SUGERENCIAS]
    vecs_banco = _e5([PREFIJO_P + b["texto"] for b in banco])
    q = _e5([PREFIJO_Q + consulta])[0]
    sims = vecs_banco @ q
    orden = sorted(range(len(banco)), key=lambda i: -sims[i])
    fuera = {u.strip().lower() for u in ya_usadas}
    salida = []
    for i in orden:
        t = banco[i]["texto"]
        if t.lower() not in fuera:
            salida.append(t)
        if len(salida) >= SUGERENCIAS_N:
            break
    return salida


# ---------- memoria de correcciones (F5) ----------
def _memoria_buena(consulta: str) -> list:
    _, memoria = _indexar()
    confirmadas = [m for m in memoria if m.get("confirmada")]
    return [m for _, m in _buscar(consulta, confirmadas, 1, UMBRAL_MEM)]


def _guardar_correccion(pregunta: str, respuesta: str, confirmada: bool):
    _, memoria = _indexar()
    baja = pregunta.strip().lower()
    memoria = [m for m in memoria if m["texto"].strip().lower() != baja]
    memoria.append({
        "texto": pregunta.strip(),
        "respuesta": respuesta.strip(),
        "confirmada": confirmada,
        "cuando": datetime.now(timezone.utc).isoformat(timespec="seconds"),
    })
    if len(memoria) > MAX_CORRECCIONES:
        memoria = memoria[-MAX_CORRECCIONES:]
    _guardar_json(INDICE / "memoria.json", memoria)  # _indexar() completa el vector


# ---------- recuperación few-shot de golden (Go trae candidatos, Python ordena por e5) ----------
_vector_golden = {}   # id → vector (caché; los golden cambian poco)


def _vec_golden(gid: int, texto: str) -> np.ndarray:
    with _candado:
        v = _vector_golden.get(gid)
    if v is None:
        v = _e5([PREFIJO_P + texto])[0]
        with _candado:
            _vector_golden[gid] = v
    return v


def _recuperar_golden(consulta: str, candidatos: list[dict], k: int = 3) -> list[dict]:
    """Ordena los golden por similaridad e5 con la pregunta. Devuelve [{id, pregunta, sql, sim}]."""
    if not candidatos:
        return []
    q = _e5([PREFIJO_Q + consulta])[0]
    pares = [(float(np.dot(q, _vec_golden(g["id"], g["pregunta"]))), g) for g in candidatos]
    pares.sort(key=lambda x: -x[0])
    return [{"id": g["id"], "pregunta": g["pregunta"], "sql": g.get("sql", ""),
             "sim": round(s, 3)} for s, g in pares[:k] if s >= 0.45]


# ---------- system prompt ----------
SYSTEM = (
    "Eres el asistente de EDISYS, software de administración de edificios en Perú. "
    "Responde SIEMPRE en español de Perú, con tuteo, breve (máximo 6 líneas). "
    "Dinero como «S/ 4.800,00»: punto de miles y coma decimal. "
    "Usa SOLO los datos del contexto y la conversación; si algo no está, di "
    "«No tengo ese dato, escríbele a la administración» y nada más. "
    "Nunca inventes cifras, fechas ni nombres. Nunca muestres DNI ni teléfonos de terceros."
)


def _ensamblar(mensajes: list[dict], frag: list, mem: list, datos: dict | None = None) -> list[dict]:
    partes = [SYSTEM]
    datos = datos or {}
    if frag:
        partes.append("Contexto del edificio (puede estar vacío o no servir):\n" +
                      "\n---\n".join(f["texto"] for f in frag))
    if mem:
        partes.append("Correcciones aprendidas de la administración:\n" +
                      "\n".join(f"P: {m['texto']}\nR: {m['respuesta']}" for m in mem))
    ejemplos = datos.get("golden") or []
    if ejemplos:
        # Few-shot de dominio: patrón pregunta→SQL verificado del edificio. El modelo
        # NO debe inventar cifras: si hay filas, vienen aparte y son las de verdad.
        partes.append("Ejemplos de preguntas que SÍ sabes responder (con su consulta interna):\n" +
                      "\n".join(f"P: {g['pregunta']}" for g in ejemplos) +
                      "\nSi la pregunta del usuario se parece a alguna, respóndela con datos del contexto o di que la administración la ve en la app.")
    filas = datos.get("filas")
    if filas:
        partes.append("Datos reales ya calculados para esta pregunta (usa SOLO estos números, en soles con coma decimal):\n" +
                      json.dumps(filas, ensure_ascii=False)[:1800])
    fuera = sum(len(p) for p in partes)
    historial = []
    for m in reversed(mensajes[:-1]):
        if fuera + len(m.get("content", "")) > MAX_HISTORIAL_CHARS:
            break
        historial.insert(0, m)
        fuera += len(m.get("content", ""))
    return [{"role": "system", "content": "\n\n".join(partes)}] + \
        historial + [{"role": "user", "content": mensajes[-1]["content"]}]


# ---------- esquemas ----------
class Mensaje(BaseModel):
    role: str
    content: str


class PeticionChat(BaseModel):
    mensajes: list[Mensaje]
    pedir_sugerencias: bool = True
    memoria: bool = True
    datos: dict = {}   # Go manda filas reales (golden ejecutado) y/o ejemplos few-shot


class Correccion(BaseModel):
    pregunta: str
    respuesta: str
    respondio_bien: bool
    admin: bool = False


class PeticionSugerencias(BaseModel):
    consulta: str = ""
    usadas: list[str] = []


class GoldenCandidato(BaseModel):
    id: int
    pregunta: str
    sql: str = ""


class PeticionRecuperar(BaseModel):
    consulta: str
    candidatos: list[GoldenCandidato]


# ---------- endpoints ----------
@app.get("/v1/salud")
def salud():
    try:
        with urllib.request.urlopen(LLAMA + "/health", timeout=5) as r:
            llama = {"estado": "ok" if r.status == 200 else f"http {r.status}"}
    except Exception as e:
        llama = {"estado": "caido", "error": str(e)[:120]}
    return {"ok": True, "motor": "1.1.0", "llama": llama}


@app.post("/v1/chat")
def chat(p: PeticionChat, request: Request):
    if not p.mensajes or p.mensajes[-1].role != "user":
        raise HTTPException(422, "El último mensaje debe ser del usuario.")
    consulta = p.mensajes[-1].content.strip()
    if not consulta:
        raise HTTPException(422, "Mensaje vacío.")
    if len(consulta) > 2000:
        raise HTTPException(422, "Mensaje demasiado largo (máximo 2000 caracteres).")

    faq, _ = _indexar()
    frag = [x for _, x in _buscar(consulta, faq, RECUPERADOS, UMBRAL_FAQ)]
    mem = _memoria_buena(consulta) if p.memoria else []

    entradas = [{"role": m.role, "content": m.content} for m in p.mensajes]
    try:
        texto, uso = _llama(_ensamblar(entradas, frag, mem, p.datos))
    except Exception as e:
        raise HTTPException(502, f"llama-server no respondió: {e}")

    texto = _aplicar_filtro(texto, frag)

    # F5: evasión aprendida — si el contexto decía la respuesta y el modelo la
    # repite casi literal varias veces, responde directamente del contexto.
    if frag:
        vec_r = _e5([PREFIJO_P + texto])[0]
        similares = sum(
            1 for f in frag
            if float(np.dot(vec_r, np.array(f["vector"], dtype=np.float32))) > UMBRAL_ALTO_SIM)
        if similares >= UMBRAL_ALTO_VECES:
            texto = frag[0]["texto"]

    sugerencias = _sugerencias(consulta, [m.content for m in p.mensajes if m.role == "user"]) \
        if p.pedir_sugerencias else []
    intencion = "golden" if p.datos.get("filas") else ("falla" if frag else "conversa")

    with _candado:
        _log.append({"cuando": datetime.now(timezone.utc).isoformat(timespec="seconds"),
                     "pregunta": consulta, "respuesta": texto[:400],
                     "intencion": intencion, "claves": _clave_filtro(frag),
                     "tokens": uso.get("completion_tokens")})
        del _log[:-100]

    qvec = _e5([PREFIJO_Q + consulta])[0]
    contexto_usado = [{"id": f.get("id"),
                       "sim": round(float(np.dot(qvec, np.array(f["vector"], dtype=np.float32))), 3)}
                      for f in frag]

    return {"respuesta": texto, "intencion": intencion,
            "sugerencias": sugerencias, "contexto_usado": contexto_usado,
            "tokens_generados": uso.get("completion_tokens")}


@app.post("/v1/feedback")
def feedback(c: Correccion):
    if c.admin and c.respondio_bien and c.pregunta and c.respuesta:
        # El admin confirmó la pareja: entra a la memoria (F5) con vector.
        _guardar_correccion(c.pregunta, c.respuesta, confirmada=True)
        _indexar()
        return {"ok": True, "memoria": "confirmada"}
    if not c.respondio_bien:
        # KTO-lite (F6): castiga la combinación de fragmentos usados, si hubo.
        with _candado:
            reciente = next((x for x in reversed(_log) if x["pregunta"] == c.pregunta), None)
        if reciente and reciente["claves"]:
            ids = [i for i in reciente["claves"].split("|") if i]
            _votar_filtro([{"id": i} for i in ids], -1)
            _propuestas.append({"pregunta": c.pregunta, "respuesta": c.respuesta,
                                "cuando": datetime.now(timezone.utc).isoformat(timespec="seconds")})
            del _propuestas[:-50]
        return {"ok": True, "memoria": "propuesta_pendiente"}
    _votar_filtro([{"id": ""}], +1)  # 👍 sin contexto: refuerzo genérico
    return {"ok": True, "memoria": "reforzado"}


@app.post("/v1/recuperar")
def recuperar(p: PeticionRecuperar):
    """Ordena golden candidatos por similaridad e5 con la pregunta (Go decide si ejecutar)."""
    return {"golden": _recuperar_golden(p.consulta.strip(), [c.model_dump() for c in p.candidatos])}


@app.post("/v1/sugerencias")
def sugerencias(p: PeticionSugerencias):
    return {"sugerencias": _sugerencias(p.consulta.strip(), p.usadas)}


@app.get("/v1/registro")
def registro():
    with _candado:
        return {"interacciones": list(_log), "propuestas": list(_propuestas),
                "filtro": dict(_filtro)}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8080)
