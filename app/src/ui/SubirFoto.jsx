import { useEffect, useId, useRef, useState } from 'react';
import Icono from './Icono.jsx';
import { Spinner } from './Boton.jsx';
import { comprimirImagen } from '../lib/imagen.js';
import { formatearFechaHora } from '../lib/fechas.js';

/**
 * Abre la cámara trasera (<input capture="environment">), previsualiza y comprime (JPEG ≤ 1600 px, ~300 KB).
 * onFoto({ archivo, tomadaEn, url }) — la subida la hace la pantalla (con `progreso` para la barra).
 */
export default function SubirFoto({ foto, onFoto, etiqueta = 'Tomar foto', obligatoria = false, progreso = null, alto = 'h-56', oscuro = true, children }) {
  const id = useId();
  const input = useRef(null);
  const [procesando, setProcesando] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => () => foto?.url?.startsWith('blob:') && URL.revokeObjectURL(foto.url), [foto]);

  const alElegir = async (e) => {
    const f = e.target.files?.[0];
    e.target.value = '';
    if (!f) return;
    if (!f.type.startsWith('image/')) {
      setError('Eso no es una foto.');
      return;
    }
    setError(null);
    setProcesando(true);
    try {
      const r = await comprimirImagen(f);
      onFoto?.({ ...r, url: URL.createObjectURL(r.archivo) });
    } catch {
      setError('No pudimos leer la foto. Intenta tomarla otra vez.');
    } finally {
      setProcesando(false);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      <input ref={input} id={id} type="file" accept="image/*" capture="environment" className="sr-only" onChange={alElegir} aria-label={etiqueta} />
      {foto ? (
        <div className={`relative overflow-hidden rounded-tarjeta ${oscuro ? 'bg-superficie-oscura' : 'bg-superficie-2'} ${alto}`}>
          <img src={foto.url} alt="Foto tomada" className="h-full w-full object-cover" />
          {children}
          <span className="absolute left-3 top-3 rounded-full bg-acento px-2.5 py-1 text-xs font-semibold text-white">
            {obligatoria ? 'Foto obligatoria · tomada' : 'Foto tomada'}
          </span>
          {foto.tomadaEn && (
            <span className="absolute bottom-3 left-3 rounded bg-tinta/70 px-2 py-0.5 text-xs text-white">{formatearFechaHora(foto.tomadaEn + '-05:00')}</span>
          )}
          <button
            type="button"
            onClick={() => input.current?.click()}
            className="absolute bottom-3 right-3 h-11 rounded-control bg-superficie px-3 text-sm font-semibold text-tinta shadow"
          >
            Repetir foto
          </button>
          {progreso != null && progreso < 100 && (
            <div className="absolute inset-x-0 bottom-0 h-1.5 bg-tinta/40" role="progressbar" aria-valuenow={progreso} aria-valuemin={0} aria-valuemax={100} aria-label="Subiendo foto">
              <div className="h-full bg-acento-oscuro transition-[width] duration-media" style={{ width: `${progreso}%` }} />
            </div>
          )}
        </div>
      ) : (
        <button
          type="button"
          onClick={() => input.current?.click()}
          disabled={procesando}
          className={`flex ${alto} w-full flex-col items-center justify-center gap-3 rounded-tarjeta border-2 border-dashed border-acento-borde bg-acento-suave text-acento hover:border-acento focus-visible:outline focus-visible:outline-2 focus-visible:outline-acento`}
        >
          {procesando ? <Spinner className="h-8 w-8" /> : <Icono nombre="camara" tam={40} grosor={1.6} />}
          <span className="text-lg font-semibold">{procesando ? 'Preparando la foto…' : etiqueta}</span>
          {obligatoria && <span className="text-sm text-acento-hover">La foto es obligatoria</span>}
        </button>
      )}
      {error && (
        <p className="text-sm text-alerta" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
