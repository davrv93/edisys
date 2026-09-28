import { useId, useRef, useState } from 'react';
import Icono from './Icono.jsx';
import { validarArchivo } from '../lib/imagen.js';

/**
 * PDF, imagen o video para vouchers e informes. Tope 10 MB (PDF/imagen) y 50 MB (video).
 * Valida el tipo ANTES de subir. onArchivo(file | null).
 */
export default function SubirArchivo({ etiqueta = 'Adjuntar archivo', ayuda = 'PDF o imagen, hasta 10 MB', aceptar = 'application/pdf,image/*', tipos = ['application/pdf', 'image/'], extensiones = [], archivo, onArchivo, error: errorExterno }) {
  const id = useId();
  const ref = useRef(null);
  const [error, setError] = useState(null);
  const elegir = (f) => {
    if (!f) return;
    const e = validarArchivo(f, { tipos, extensiones });
    if (e) {
      setError(e);
      onArchivo?.(null);
      return;
    }
    setError(null);
    onArchivo?.(f);
  };
  const err = error || errorExterno;
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-sm font-semibold text-tinta">{etiqueta}</span>
      <label
        htmlFor={id}
        onDragOver={(e) => e.preventDefault()}
        onDrop={(e) => {
          e.preventDefault();
          elegir(e.dataTransfer.files?.[0]);
        }}
        className={`flex min-h-[56px] cursor-pointer items-center gap-3 rounded-lg border border-dashed px-3 py-3 ${err ? 'border-alerta' : 'border-borde-fuerte'} bg-superficie hover:bg-fondo`}
      >
        <Icono nombre={archivo ? 'documento' : 'subir'} className="text-acento" />
        <span className="min-w-0 flex-1 text-sm">
          {archivo ? (
            <>
              <span className="block truncate font-semibold">{archivo.name}</span>
              <span className="text-texto-apoyo">{(archivo.size / 1024).toFixed(0)} KB · toca para cambiar</span>
            </>
          ) : (
            <>
              <span className="block font-semibold text-acento">Elegir archivo</span>
              <span className="text-texto-apoyo">{ayuda}</span>
            </>
          )}
        </span>
        {archivo && (
          <button
            type="button"
            aria-label="Quitar archivo"
            className="flex h-11 w-11 items-center justify-center rounded-lg hover:bg-superficie-2"
            onClick={(e) => {
              e.preventDefault();
              onArchivo?.(null);
            }}
          >
            <Icono nombre="cerrar" tam={16} />
          </button>
        )}
      </label>
      <input ref={ref} id={id} type="file" accept={aceptar} className="sr-only" onChange={(e) => elegir(e.target.files?.[0])} />
      {err && (
        <p className="text-sm text-alerta" role="alert">
          {err}
        </p>
      )}
    </div>
  );
}
