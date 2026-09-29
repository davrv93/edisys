#!/usr/bin/env python3
"""F2 · Construye el dataset JSONL para el QLoRA a partir de la FAQ del motor.

Entrada:  motor/indice/faq.json (la misma que sirve el RAG en producción)
Salida:   motor/entrenar/dataset/edisys_chat.jsonl (formato chat de mlx-lm:
          {"messages": [{"role": "...", "content": "..."}]})

Cada fragmento genera 4 ejemplos: pregunta directa, con rodeo, con typo y con
«por favor». El system prompt es el MISMO que usa app.py en producción: así el
afinado refuerza el formato que el motor ya pide (español-PE, tuteo, S/ con
punto de miles, «no tengo ese dato» cuando no está). Se reparte 90/10
entrenamiento/validación con semilla fija para poder repetir la corrida.

Uso:  python3 construir_dataset.py [ruta_faq.json]
"""
import json
import random
import sys
from pathlib import Path

DIR = Path(__file__).parent
FAQ = Path(sys.argv[1]) if len(sys.argv) > 1 else DIR.parent / "indice" / "faq.json"
SALIDA = DIR / "dataset"
SEMILLA = 20260928

SYSTEM = (
    "Eres el asistente de EDISYS, software de administración de edificios en Perú. "
    "Responde SIEMPRE en español de Perú, con tuteo, breve (máximo 6 líneas). "
    "Dinero como «S/ 4.800,00»: punto de miles y coma decimal. "
    "Usa SOLO los datos del contexto y la conversación; si algo no está, di "
    "«No tengo ese dato, escríbele a la administración» y nada más. "
    "Nunca inventes cifras, fechas ni nombres. Nunca muestres DNI ni teléfonos de terceros."
)

# Paráfrasis por id: la primera persona pregunta distinto para cada fragmento.
RODEOS = {
    "faq_horarios_piscina": ["¿La piscina está abierta ahora?", "¿Qué horario maneja la piscina?"],
    "faq_reservar_parrilla": ["Quiero la parrilla este finde, ¿cómo hago?", "¿Me reservas la parrilla?"],
    "faq_reportar_incidencia": ["Se está saliendo el agua del lavadero, ¿a quién aviso?", "¿Cómo denuncio una falla del ascensor?"],
    "faq_aforo_piscina": ["¿Cuántos cabemos en la piscina?", "¿Hay límite de gente en la piscina?"],
    "faq_pago_yape": ["¿Cuál es el Yape de la administración?", "¿Cómo le hago para pagar mi cuota?"],
    "faq_morosidad": ["¿Cómo va la morosidad?", "¿Cuánta gente debe cuotas este mes?"],
    "faq_saldo_banco": ["¿Cuánto hay de dinero en el banco del edificio?", "¿Cuál es el saldo de la cuenta?"],
    "faq_reparto_medidores": ["¿Por qué me cobran agua si mi medidor marca menos?", "Explícame cómo calculan mi consumo de agua"],
    "faq_horario_admin": ["¿A qué hora abre la administración?", "¿Cuándo atienden la oficina?"],
    "faq_cuota_ordinaria": ["¿Qué es eso de cuota extraordinaria?", "¿Qué cubre mi cuota mensual?"],
    "faq_deuda_402": ["¿Cuánto le debe el 402?", "El dpto 402 ¿está moroso?"],
    "faq_pago_201": ["El 201, ¿ya pagó setiembre?", "¿Cómo va el pago del Dpto 201?"],
    "faq_recibo_contenido": ["¿Qué partidas trae mi recibo?", "¿De dónde sale el total de mi recibo?"],
    "faq_horario_sum": ["¿Hasta qué hora funciona el SUM?", "¿El SUM se puede de noche?"],
    "faq_quien_puede_reservar": ["Mi inquilino, ¿puede reservar la parrilla?", "Si estoy moroso, ¿puedo reservar igual?"],
}
TYPOS = {"piscina": "piscna", "reservar": "reservar,", "cuota": "cuota,", "parrilla": "parrila",
         "morosidad": "moroosidad", "medidores": "medidores ", "debo": "ddebo", "recibo": "recivbo"}


def variantes(pid: str, texto: str) -> list[str]:
    """4 formas de preguntar lo mismo. La respuesta siempre sale del fragmento."""
    base = pid.replace("faq_", "").replace("_", " ")
    primera = texto.split(". ")[0].rstrip(".")
    directa = f"¿Me puedes explicar sobre {base}?"
    rodeo = RODEOS.get(pid, [directa])[0]
    con_typo = rodeo
    for mal, bien in TYPOS.items():
        if mal.rstrip(",") in rodeo.lower():
            con_typo = rodeo.lower().replace(mal.rstrip(","), mal, 1)
            break
    cortesia = f"Por favor, {rodeo[0].lower() + rodeo[1:]}" if rodeo else rodeo
    return [directa, rodeo, con_typo, cortesia]


def respuesta_de(texto: str) -> str:
    """Respuesta corta: 1–3 primeras oraciones del fragmento (el máximo de 6 líneas del prompt)."""
    oraciones = [o.strip() for o in texto.split(". ") if o.strip()]
    return ". ".join(oraciones[:3]).rstrip(".") + "."


def main():
    faq = json.loads(FAQ.read_text(encoding="utf-8"))
    ejemplos = []
    for f in faq:
        resp = respuesta_de(f["texto"])
        for pregunta in variantes(f["id"], f["texto"]):
            ejemplos.append({"messages": [
                {"role": "system", "content": SYSTEM},
                {"role": "user", "content": pregunta},
                {"role": "assistant", "content": resp},
            ]})

    azar = random.Random(SEMILLA)
    azar.shuffle(ejemplos)
    corte = max(1, int(len(ejemplos) * 0.9))
    SALIDA.mkdir(parents=True, exist_ok=True)
    (SALIDA / "edisys_chat.jsonl").write_text(
        "\n".join(json.dumps(e, ensure_ascii=False) for e in ejemplos[:corte]) + "\n", encoding="utf-8")
    (SALIDA / "edisys_chat_val.jsonl").write_text(
        "\n".join(json.dumps(e, ensure_ascii=False) for e in ejemplos[corte:]) + "\n", encoding="utf-8")
    print(f"F2 · {len(ejemplos)} ejemplos desde {len(faq)} fragmentos "
          f"(entrena {corte}, valida {len(ejemplos) - corte}) en {SALIDA}/")
    print("Ojo: son paráfrasis mecánicas. Antes de la F4 de verdad, revisa el 10 % "
          "y añade preguntas reales de WhatsApp: eso vale más que doblar el dataset.")


if __name__ == "__main__":
    main()
