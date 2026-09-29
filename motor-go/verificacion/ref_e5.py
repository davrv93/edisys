# Genera testdata/e5_ref.json: ids y vectores de app._e5, sueltos y en lote.
# docker run --rm -i --memory 1g -v <models>:/models:ro --entrypoint python edisys/motor:local - < ref_e5.py > ../testdata/e5_ref.json
import json, sys
sys.path.insert(0, "/motor")
import app, numpy as np
textos = [
 "query: cuánto ha entrado por el alquiler de las áreas comunes este mes",
 "query: ¿Cuánto debo?",
 "passage: Quiero reservar la parrilla",
 "query: " + "¿Cuál es la morosidad del edificio y cuánto hay en la cuenta del banco? " * 12,
 "query: ñandú ÁÉÍÓÚ 🙂 S/ 4.800,00 — «hola»",
]
tok = app._cargar_e5()[1]
lote = app._e5(textos)
out = []
for t, v in zip(textos, lote):
    solo = app._e5([t])[0]
    out.append({"texto": t, "ids": tok.encode(t).ids, "vector": solo.tolist(), "vector_lote": v.tolist(),
                "coseno_solo_vs_lote": float(np.dot(solo, v))})
print(json.dumps(out, ensure_ascii=False))
