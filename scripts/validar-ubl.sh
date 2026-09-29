#!/usr/bin/env bash
# Valida los comprobantes que genera EDISYS (boleta, factura y nota de crédito):
#   1. contra los XSD oficiales de UBL 2.1 (OASIS) con xmllint;
#   2. la firma XMLDSig con xmlsec1 (en un contenedor Alpine), un verificador independiente del nuestro.
# Los XSD se bajan una vez a ~/.cache/edisys/ubl (58 MB). No se envía nada a SUNAT.
set -euo pipefail
cd "$(dirname "$0")/../api"
CACHE="${UBL_CACHE:-$HOME/.cache/edisys}"
if [ ! -f "$CACHE/ubl/xsd/maindoc/UBL-Invoice-2.1.xsd" ]; then
  mkdir -p "$CACHE"
  echo "Bajando los XSD de UBL 2.1…"
  curl -sL -o "$CACHE/ubl.zip" http://docs.oasis-open.org/ubl/os-UBL-2.1/UBL-2.1.zip
  unzip -q -o "$CACHE/ubl.zip" -d "$CACHE/ubl"
fi
SALIDA="$(mktemp -d)"; trap 'rm -rf "$SALIDA"' EXIT
UBL_XSD="$CACHE/ubl/xsd" SUNAT_SALIDA="$SALIDA" go test -count=1 -run 'TestEsquemaUBL|TestFirmaYVerificacion' ./internal/sunat/
echo "XSD de UBL 2.1: OK (boleta, factura y nota de crédito)"
docker run --rm -v "$SALIDA":/x alpine:3.20 sh -c 'apk add -q xmlsec >/dev/null 2>&1
  for f in 01 03 07; do xmlsec1 --verify --insecure --lax-key-search /x/$f.xml >/dev/null 2>&1 && echo "firma $f: OK" || { echo "firma $f: FALLA"; exit 1; }; done'
