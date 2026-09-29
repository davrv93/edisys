#!/usr/bin/env python3
"""F0 · Evaluador del benchmark del motor de EDISYS. Solo biblioteca estándar.

Por cada pregunta: pide /v1/chat/completions, mide latencia y tokens, revisa que
las cifras de la respuesta sean las esperadas (los DÍGITOS deben coincidir; el
formato S/ 4.800,00 puede venir separado distinto) y anota banderas de mala salida
(letra rara ï¿½, respuesta evasiva, respuesta en inglés).

Deja un JSON por candidato y reconstruye resultados/RESUMEN.md con todos.
"""
import argparse
import json
import re
import statistics
import time
import urllib.request
from datetime import datetime, timezone
from pathlib import Path

EVASIVAS = ("no lo sé", "no lo se", "no tengo información", "no puedo ayudar",
            "no dispongo", "no cuento con")
PALABRAS_ES = ("que", "de ", "la ", "el ", "para", "con")  # heurística mínima de español


def http(base, ruta, cuerpo=None, tiempo=180):
    datos = json.dumps(cuerpo).encode() if cuerpo is not None else None
    peticion = urllib.request.Request(
        base + ruta, data=datos,
        headers={"Content-Type": "application/json"} if datos else {})
    with urllib.request.urlopen(peticion, timeout=tiempo) as r:
        return json.loads(r.read().decode())


def digitos(texto):
    return re.sub(r"\D", "", texto or "")


def cifras_ok(respuesta, esperado):
    """Los dígitos de lo esperado deben aparecer en la respuesta (§2 del plan:
    cero alucinación de cifros en las preguntas con cifra)."""
    if not esperado:
        return True
    return digitos(esperado) in digitos(respuesta)


def banderas(respuesta):
    b = []
    bajo = respuesta.lower()
    if "\ufffd" in respuesta or "ï¿½" in respuesta:
        b.append("mojibake")
    if any(e in bajo for e in EVASIVAS):
        b.append("evasiva")
    if respuesta and not any(p in bajo for p in PALABRAS_ES):
        b.append("no_español?")
    return b


def metricas(base):
    """Valores de /metrics de llama-server; si el build no los publica, n/d."""
    try:
        with urllib.request.urlopen(base + "/metrics", timeout=10) as r:
            texto = r.read().decode()
    except Exception:
        return {}
    def toma(nombre):
        m = re.search(rf"^{re.escape(nombre)}\s+([0-9.]+)", texto, re.M)
        return float(m.group(1)) if m else None
    return {
        "tok_s_prompt": toma("llamacpp:prompt_tokens_seconds"),
        "tok_s_generacion": toma("llamacpp:predicted_tokens_seconds"),
        "kv_cache": toma("llamacpp:kv_cache_tokens_current"),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", required=True)
    ap.add_argument("--clave", required=True)
    ap.add_argument("--gguf", required=True)
    ap.add_argument("--rss", default="")
    ap.add_argument("--salida", required=True)
    ap.add_argument("--resumen", required=True)
    args = ap.parse_args()

    preguntas = [json.loads(l) for l in
                 Path("/benchmark/preguntas.jsonl").read_text(encoding="utf-8").splitlines() if l.strip()]

    filas, latencias = [], []
    for p in preguntas:
        t0 = time.monotonic()
        try:
            r = http(args.base, "/v1/chat/completions", {
                "messages": [{"role": "user", "content": p["pregunta"]}],
                "temperature": 0.3, "max_tokens": 300,
            })
            texto = r["choices"][0]["message"]["content"].strip()
            usos = r.get("usage", {})
        except Exception as e:  # el modelo se colgó o devolvió basura: se cuenta como fallo
            texto, usos = f"ERROR: {e}", {}
        ms = round((time.monotonic() - t0) * 1000)
        latencias.append(ms)
        ok = (not texto.startswith("ERROR:")) and cifras_ok(texto, p.get("esperado"))
        filas.append({
            "id": p["id"], "categoria": p["categoria"], "ms": ms, "ok": ok,
            "esperado": p.get("esperado", ""), "banderas": banderas(texto),
            "tokens_generados": usos.get("completion_tokens"),
            "pregunta": p["pregunta"], "respuesta": texto[:400],
        })
        marca = "OK  " if ok else "FALLA"
        print(f"  {marca} {p['id']:<16} {ms:>6} ms  {p['pregunta'][:52]}")

    m = metricas(args.base)
    ok_n = sum(1 for f in filas if f["ok"])
    resumen = {
        "clave": args.clave, "gguf": args.gguf,
        "rss_llama": args.rss, "metricas": m,
        "latencia_media_ms": round(statistics.mean(latencias)),
        "latencia_p95_ms": sorted(latencias)[max(0, round(0.95 * len(latencias)) - 1)],
        "aprobados": ok_n, "total": len(filas),
        "tasa_ok": round(ok_n / len(filas), 3),
        "tok_s_generacion": m.get("tok_s_generacion"),
        "fecha": datetime.now(timezone.utc).isoformat(timespec="seconds"),
        "filas": filas,
    }
    Path(args.salida).write_text(json.dumps(resumen, ensure_ascii=False, indent=1), encoding="utf-8")
    escribe_resumen(args.resumen)
    print(f"\n  {args.clave}: {ok_n}/{len(filas)} · media {resumen['latencia_media_ms']} ms · "
          f"p95 {resumen['latencia_p95_ms']} ms · RSS llama {args.rss or 'n/d'}")


def escribe_resumen(ruta):
    filas = []
    for f in sorted(Path("/resultados").glob("*.json")):
        d = json.loads(f.read_text(encoding="utf-8"))
        filas.append((d["clave"], d))
    filas.sort(key=lambda kv: (-kv[1]["tasa_ok"], kv[1]["latencia_p95_ms"]))
    lineas = [
        "# Benchmark del motor · EC2 de 3 GB",
        "",
        f"_Actualizado: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')} · "
        f"criterio del plan: ≥ 8 tok/s y tasa de cifras correcta._",
        "",
        "| modelo | tasa ok | p95 | tok/s (server) | RSS llama |",
        "|---|---|---|---|---|",
    ]
    for clave, d in filas:
        tok = d.get("tok_s_generacion")
        tok_s = f"{tok:.1f}" if isinstance(tok, float) else "n/d"
        lineas.append(f"| {clave} | {d['aprobados']}/{d['total']} | "
                      f"{d['latencia_p95_ms']} ms | {tok_s} | {d['rss_llama'] or 'n/d'} |")
    Path(ruta).write_text("\n".join(lineas) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
