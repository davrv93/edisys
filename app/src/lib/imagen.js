// Compresión de fotos antes de subir (JPEG ≤ 1600 px, ~300 KB) y hora de la toma (EXIF).

/** Lee DateTimeOriginal (o DateTime) del EXIF de un JPEG. Devuelve ISO sin zona o null. */
export function leerFechaExif(buffer) {
  try {
    const v = new DataView(buffer);
    if (v.getUint16(0) !== 0xffd8) return null;
    let o = 2;
    while (o < v.byteLength - 4) {
      const marca = v.getUint16(o);
      const largo = v.getUint16(o + 2);
      if (marca === 0xffe1 && v.getUint32(o + 4) === 0x45786966) {
        const tiff = o + 10;
        const le = v.getUint16(tiff) === 0x4949;
        const u16 = (p) => v.getUint16(p, le);
        const u32 = (p) => v.getUint32(p, le);
        const leerIfd = (ifd, buscados) => {
          const n = u16(ifd);
          const res = {};
          for (let i = 0; i < n; i++) {
            const e = ifd + 2 + i * 12;
            const tag = u16(e);
            if (buscados.includes(tag)) res[tag] = u32(e + 8);
          }
          return res;
        };
        const texto = (p) => {
          let s = '';
          for (let i = 0; i < 19; i++) s += String.fromCharCode(v.getUint8(tiff + p + i));
          return s;
        };
        const ifd0 = leerIfd(tiff + u32(tiff + 4), [0x8769, 0x0132]);
        let crudo = null;
        if (ifd0[0x8769]) {
          const exif = leerIfd(tiff + ifd0[0x8769], [0x9003]);
          if (exif[0x9003]) crudo = texto(exif[0x9003]);
        }
        if (!crudo && ifd0[0x0132]) crudo = texto(ifd0[0x0132]);
        if (!crudo) return null;
        const m = crudo.match(/^(\d{4}):(\d{2}):(\d{2}) (\d{2}):(\d{2}):(\d{2})/);
        return m ? `${m[1]}-${m[2]}-${m[3]}T${m[4]}:${m[5]}:${m[6]}` : null;
      }
      if ((marca & 0xff00) !== 0xff00) break;
      o += 2 + largo;
    }
  } catch {
    return null;
  }
  return null;
}

/** Medidas destino manteniendo proporción, con el lado mayor ≤ max. */
export function medidasDestino(ancho, alto, max = 1600) {
  const lado = Math.max(ancho, alto);
  if (lado <= max) return { ancho, alto };
  const f = max / lado;
  return { ancho: Math.round(ancho * f), alto: Math.round(alto * f) };
}

/** Comprime una imagen a JPEG. Baja la calidad hasta quedar cerca de `objetivo` bytes. */
export async function comprimirImagen(archivo, { max = 1600, objetivo = 300 * 1024 } = {}) {
  let tomadaEn = null;
  try {
    tomadaEn = leerFechaExif(await archivo.arrayBuffer());
  } catch {
    tomadaEn = null;
  }
  let fuente;
  try {
    fuente = await createImageBitmap(archivo, { imageOrientation: 'from-image' });
  } catch {
    // Navegadores sin createImageBitmap para este tipo: se sube tal cual.
    return { archivo, tomadaEn, comprimida: false };
  }
  const { ancho, alto } = medidasDestino(fuente.width, fuente.height, max);
  const lienzo = document.createElement('canvas');
  lienzo.width = ancho;
  lienzo.height = alto;
  lienzo.getContext('2d').drawImage(fuente, 0, 0, ancho, alto);
  fuente.close?.();
  let calidad = 0.82;
  let blob = await new Promise((r) => lienzo.toBlob(r, 'image/jpeg', calidad));
  while (blob && blob.size > objetivo && calidad > 0.45) {
    calidad -= 0.08;
    blob = await new Promise((r) => lienzo.toBlob(r, 'image/jpeg', calidad));
  }
  if (!blob) return { archivo, tomadaEn, comprimida: false };
  const nombre = (archivo.name || 'foto').replace(/\.[^.]+$/, '') + '.jpg';
  return { archivo: new File([blob], nombre, { type: 'image/jpeg' }), tomadaEn, comprimida: true };
}

/** Tope de tamaño por tipo (§2.2 SubirArchivo). */
export function validarArchivo(archivo, { tipos = ['application/pdf', 'image/'], extensiones = [], maxMb = 10, maxMbVideo = 50 } = {}) {
  if (!archivo) return 'Elige un archivo.';
  const tipo = archivo.type || '';
  const nombre = String(archivo.name || '').toLowerCase();
  // Algunos navegadores no informan el tipo de un .xlsx: entonces decide la extensión.
  const ok = tipos.some((t) => (t.endsWith('/') ? tipo.startsWith(t) : tipo === t)) || extensiones.some((e) => nombre.endsWith(e));
  if (!ok) return 'Ese tipo de archivo no se acepta aquí.';
  const tope = tipo.startsWith('video/') ? maxMbVideo : maxMb;
  if (archivo.size > tope * 1024 * 1024) return `El archivo pesa más de ${tope} MB.`;
  return null;
}
