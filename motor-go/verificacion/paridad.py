import json, re, sys, time, urllib.request, pathlib
PY, GO = "http://127.0.0.1:4767", "http://127.0.0.1:4768"
S = pathlib.Path(sys.argv[1])
golden = [l.strip() for l in open(S / "golden_preguntas.txt") if l.strip()]
cands = [{"id": i + 1, "pregunta": p, "sql": f"SELECT {i+1} WHERE edificio_id=:edificio_id"} for i, p in enumerate(golden)]
PREG = ["cuánto ha entrado por el alquiler de las áreas comunes este mes",
        "¿cuánto debo?", "¿cuánto debe el dpto 402?", "quiero reservar la parrilla el sábado",
        "¿qué horarios tiene la piscina?", "¿se puede llevar mascotas a la piscina? normas del edificio",
        "¿cuál es la morosidad del edificio?", "¿cómo pago por yape?", "hay una fuga de agua en mi baño",
        "¿cuánto hay en la cuenta del banco?"]

def req(base, metodo, ruta, cuerpo=None):
    datos = None if cuerpo is None else (cuerpo if isinstance(cuerpo, bytes) else json.dumps(cuerpo, ensure_ascii=False).encode())
    r = urllib.request.Request(base + ruta, data=datos, method=metodo, headers={"Content-Type": "application/json"})
    t0 = time.perf_counter()
    try:
        with urllib.request.urlopen(r, timeout=180) as x:
            b, c = x.read(), x.status
    except urllib.error.HTTPError as e:
        b, c = e.read(), e.code
    return c, b.decode(), time.perf_counter() - t0

sin_cuando = lambda s: re.sub(r'"cuando":"[^"]*"', '"cuando":"…"', s)
ok = fallos = 0
def comparar(nombre, metodo, ruta, cuerpo=None, normal=lambda s: s):
    global ok, fallos
    cp, bp, tp = req(PY, metodo, ruta, cuerpo)
    cg, bg, tg = req(GO, metodo, ruta, cuerpo)
    igual = cp == cg and normal(bp) == normal(bg)
    if igual: ok += 1
    else:
        fallos += 1
        print(f"✗ {nombre}\n   py {cp} {bp[:600]}\n   go {cg} {bg[:600]}")
    return bp, bg

comparar("salud", "GET", "/v1/salud")
comparar("registro vacío", "GET", "/v1/registro")
for q in PREG:
    bp, _ = comparar(f"recuperar: {q}", "POST", "/v1/recuperar", {"consulta": q, "candidatos": cands})
    orden = json.loads(bp)["golden"]
    ejemplos = [{"pregunta": g["pregunta"]} for g in orden if g["sim"] >= 0.60]
    comparar(f"chat: {q}", "POST", "/v1/chat", {"mensajes": [{"role": "user", "content": q}]})
    comparar(f"chat+golden: {q}", "POST", "/v1/chat", {"mensajes": [{"role": "user", "content": q}], "datos": {"golden": ejemplos}, "pedir_sugerencias": False})
    comparar(f"sugerencias: {q}", "POST", "/v1/sugerencias", {"consulta": q, "usadas": ["¿Cuánto debo?"]})
# conversación con historial largo y filas
hist = [{"role": "user", "content": "hola " * 400}, {"role": "assistant", "content": "¡Hola! " * 300}, {"role": "user", "content": "¿cuánto debe el dpto 402?"}]
comparar("chat historial+filas", "POST", "/v1/chat", {"mensajes": hist, "datos": {"filas": [{"unidad": "402", "deuda_cts": 142000, "meses": 3, "prom": 473.33}], "golden": [{"pregunta": "x"}]}})
# memoria: el admin confirma, luego se pregunta parecido
comparar("feedback admin", "POST", "/v1/feedback", {"pregunta": "¿Dónde dejo la basura reciclable?", "respuesta": "En el cuarto de reciclaje del sótano 1.", "respondio_bien": True, "admin": True})
mp = (S / "paridad/indice_py/memoria.json").read_text(); mg = (S / "paridad/indice_go/memoria.json").read_text()
if sin_cuando(mp.replace(" ", "")) == sin_cuando(mg.replace(" ", "")) and len(mp) == len(mg): ok += 1; print("✓ memoria.json idéntico (salvo «cuando»), vector incluido")
else: fallos += 1; print("✗ memoria.json difiere", len(mp), len(mg))
comparar("chat usa memoria", "POST", "/v1/chat", {"mensajes": [{"role": "user", "content": "¿dónde dejo la basura reciclable?"}]})
# 👎 y 👍, filtro, registro
comparar("feedback 👎", "POST", "/v1/feedback", {"pregunta": "¿qué horarios tiene la piscina?", "respuesta": "mal", "respondio_bien": False})
comparar("feedback 👎 sin chat", "POST", "/v1/feedback", {"pregunta": "nunca", "respuesta": "x", "respondio_bien": False})
comparar("feedback 👍", "POST", "/v1/feedback", {"pregunta": "x", "respuesta": "y", "respondio_bien": True})
comparar("chat tras 👎 (filtro)", "POST", "/v1/chat", {"mensajes": [{"role": "user", "content": "¿qué horarios tiene la piscina?"}]})
comparar("registro", "GET", "/v1/registro", normal=sin_cuando)
# errores
for nombre, ruta, cuerpo in [("chat vacío", "/v1/chat", {"mensajes": [{"role": "user", "content": "  "}]}),
                             ("chat assistant", "/v1/chat", {"mensajes": [{"role": "assistant", "content": "x"}]}),
                             ("chat largo", "/v1/chat", {"mensajes": [{"role": "user", "content": "ñ" * 2001}]}),
                             ("chat sin mensajes", "/v1/chat", {}),
                             ("chat sin content", "/v1/chat", {"mensajes": [{"role": "user"}]}),
                             ("recuperar sin candidatos", "/v1/recuperar", {"consulta": "x"}),
                             ("feedback sin bool", "/v1/feedback", {"pregunta": "x", "respuesta": "y"})]:
    comparar(f"422 {nombre}", "POST", ruta, cuerpo)
comparar("404", "GET", "/no/existe")
comparar("405", "GET", "/v1/chat")
fp = (S / "paridad/indice_py/faq.json").read_bytes(); fg = (S / "paridad/indice_go/faq.json").read_bytes()
if fp == fg: ok += 1; print("✓ faq.json idéntico tras la sesión")
else: fallos += 1; print("✗ faq.json difiere")
print(f"\n{ok} iguales, {fallos} distintos")
