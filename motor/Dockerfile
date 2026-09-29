# EDISYS · motor conversacional. Imagen pequeña; los modelos van por volumen, no en la imagen.
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
WORKDIR /motor

RUN useradd --system --home /motor edisys

COPY requirements.txt .
RUN pip install -q -r requirements.txt

COPY app.py .
COPY indice/ indice/
RUN mkdir -p indice && chown -R edisys:edisys /motor

USER edisys
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=20s --retries=3 \
  CMD python -c "import urllib.request;urllib.request.urlopen('http://127.0.0.1:8080/v1/salud',timeout=3)"
CMD ["python", "-m", "uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080"]
