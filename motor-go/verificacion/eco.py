# llama-server falso: devuelve un eco determinista de lo que recibe.
import hashlib, json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b'{"status":"ok"}')
    def do_POST(self):
        cuerpo = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        firma = hashlib.sha256(json.dumps(cuerpo, ensure_ascii=False, sort_keys=True).encode()).hexdigest()[:16]
        sistema = cuerpo["messages"][0]["content"]
        ctx = sistema.split("Contexto del edificio (puede estar vacío o no servir):\n")[1].split("\n---\n")[0][:200] if "Contexto del edificio" in sistema else "sin contexto"
        texto = f"  eco {firma} · {len(cuerpo['messages'])} mensajes\n{ctx}\n{cuerpo['messages'][-1]['content']}  "
        out = {"choices": [{"message": {"role": "assistant", "content": texto}}],
               "usage": {"completion_tokens": len(sistema), "prompt_tokens": 1}}
        b = json.dumps(out).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers(); self.wfile.write(b)
ThreadingHTTPServer(("0.0.0.0", 8080), H).serve_forever()
