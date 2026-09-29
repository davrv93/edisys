# Genera testdata/py_ref.json ejecutando _ensamblar, _jaccard y _aplicar_filtro de app.py.
# docker run --rm -i --memory 512m --entrypoint python edisys/motor:local - < ref_logica.py > ../testdata/py_ref.json
import json, sys
sys.path.insert(0, "/motor")
import app
frag = [{"id": "f1", "texto": "Horarios de la piscina: de 08:00 a 12:00. Aforo máximo: 20 personas."},
        {"id": "f2", "texto": "La parrilla se reserva desde la app con 48 horas de anticipación."}]
mem = [{"texto": "¿cuánto debo?", "respuesta": "Revisa tu estado de cuenta en la app."}]
largo = "x" * 2500
entradas = [
  ([{"role":"user","content":"hola"}], [], [], {}),
  ([{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"  ¿horario piscina?  "}], frag, mem, {}),
  ([{"role":"user","content":"x"}], [], [], {"filas":[{"unidad":"402","deuda_cts":142000,"monto":1234.5,"nota":"á\"\n<b>","z":None,"ok":True,"f":1e-05,"g":100.0}]}),
  ([{"role":"user","content":"x"}], frag, [], {"golden":[{"pregunta":"¿Cuánto se recaudó este mes?"},{"pregunta":7}]}),
  ([{"role":"user","content":"x"}], [], [], {"filas":[], "golden":[{"pregunta":"g1"}]}),
  ([{"role":"user","content":largo},{"role":"assistant","content":"y"*2000},{"role":"user","content":"z"*1500},{"role":"assistant","content":"w"*1000},{"role":"user","content":"final"}], frag, [], {}),
  ([{"role":"user","content":"x"}], [], [], {"filas":[{"t": "ñ"*3000}]}),
]
casos = [{"mensajes": m, "frag": f, "mem": me, "datos": d, "salida": app._ensamblar(m, f, me, d)} for m, f, me, d in entradas]
jac = [{"a": a, "b": b, "j": app._jaccard(a, b)} for a, b in [
    ("cuánto ha entrado por el alquiler de las áreas comunes este mes", "¿Cuánto se recaudó por alquiler de áreas comunes este mes?"),
    ("deuda del 402", "¿Cuánto debe el dpto 402?"), ("", "x"), ("qué es", "la de"), ("Reservas de la PARRILLA 2026", "reservas parrilla este año")]]
app._filtro[app._clave_filtro(frag)] = {"+1": 0, "-1": 1}
filtro = [{"texto": t, "salida": app._aplicar_filtro(t, frag)} for t in [
    "La piscina abre de 08:00 a 12:00.\nAforo máximo: 20 personas.\nOk.\r\nGracias por escribir, vecino",
    "Nada que ver con el contexto\n\nFin", "Horarios anticipación reserva"]]
print(json.dumps({"ensamblar": casos, "jaccard": jac, "filtro": filtro, "frag": frag}, ensure_ascii=False))
