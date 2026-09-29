import { useEffect, useId, useState } from 'react';
import { ctsATexto, parsearSoles } from '../lib/dinero.js';
import SelectorFecha from './SelectorFecha.jsx';
import Icono from './Icono.jsx';

const BASE_INPUT =
  'w-full rounded-control border bg-superficie px-3 text-base sm:text-sm text-tinta placeholder:text-texto-apoyo ' +
  'transition-colors duration-rapida focus:outline-none focus:ring-2 focus:ring-acento focus:border-acento disabled:bg-superficie-2 disabled:text-texto-apoyo';

/**
 * Campo con etiqueta, ayuda y error.
 * tipo: texto | numero | dinero | fecha | select | textarea | correo | telefono | buscar | clave
 * `dinero` escribe en soles y entrega céntimos enteros (o null) en onCambio.
 * `numero` usa inputMode decimal para el teclado del celular.
 */
export default function Campo({
  etiqueta, ayuda, error, tipo = 'texto', valor, onCambio, opciones = [], id, className = '',
  inputClassName = '', alto = 'h-11 lg:h-9', ocultarEtiqueta = false, ...resto
}) {
  const auto = useId();
  const idCampo = id || `campo-${auto}`;
  const idAyuda = `${idCampo}-ayuda`;
  const idError = `${idCampo}-error`;
  const describe = [ayuda ? idAyuda : null, error ? idError : null].filter(Boolean).join(' ') || undefined;
  const borde = error ? 'border-alerta' : 'border-borde-fuerte';
  const comunes = {
    id: idCampo,
    'aria-invalid': error ? 'true' : undefined,
    'aria-describedby': describe,
    ...resto,
  };

  let control;
  if (tipo === 'select') {
    control = (
      <select className={`${BASE_INPUT} ${borde} ${alto} ${inputClassName}`} value={valor ?? ''} onChange={(e) => onCambio?.(e.target.value)} {...comunes}>
        {opciones.map((o) => {
          const op = typeof o === 'object' ? o : { valor: o, etiqueta: o };
          return (
            <option key={op.valor} value={op.valor} disabled={op.deshabilitado}>
              {op.etiqueta}
            </option>
          );
        })}
      </select>
    );
  } else if (tipo === 'textarea') {
    control = (
      <textarea className={`${BASE_INPUT} ${borde} py-2 min-h-[88px] text-base ${inputClassName}`} value={valor ?? ''} onChange={(e) => onCambio?.(e.target.value)} {...comunes} />
    );
  } else if (tipo === 'fecha') {
    // v2: selector propio en español (dd/mm/aaaa, lunes primero). Entrega «AAAA-MM-DD» como antes.
    control = <SelectorFecha className={`${BASE_INPUT} ${borde} ${alto} ${inputClassName}`} valor={valor} onCambio={onCambio} {...comunes} />;
  } else if (tipo === 'dinero') {
    control = <CampoDinero className={`${BASE_INPUT} ${borde} ${alto} ${inputClassName}`} valor={valor} onCambio={onCambio} {...comunes} />;
  } else {
    const tipos = { texto: 'text', numero: 'text', fecha: 'date', correo: 'email', telefono: 'tel', buscar: 'search', clave: 'password' };
    const extra = tipo === 'numero' ? { inputMode: 'decimal', autoComplete: 'off' } : tipo === 'telefono' ? { inputMode: 'tel' } : {};
    control = (
      <input
        type={tipos[tipo] || 'text'}
        className={`${BASE_INPUT} ${borde} ${alto} tabular-nums ${inputClassName}`}
        value={valor ?? ''}
        onChange={(e) => onCambio?.(e.target.value)}
        {...extra}
        {...comunes}
      />
    );
  }

  return (
    <div className={`flex flex-col gap-1.5 ${className}`}>
      {etiqueta && (
        <label htmlFor={idCampo} className={ocultarEtiqueta ? 'sr-only' : 'text-sm font-semibold text-tinta'}>
          {etiqueta}
        </label>
      )}
      {control}
      {ayuda && !error && (
        <p id={idAyuda} className="text-xs text-texto-apoyo">
          {ayuda}
        </p>
      )}
      {error && (
        <p id={idError} className="text-sm text-alerta flex items-center gap-1" role="alert">
          <Icono nombre="alerta" tam={14} /> {error}
        </p>
      )}
    </div>
  );
}

function CampoDinero({ valor, onCambio, className, ...resto }) {
  const [texto, setTexto] = useState(() => ctsATexto(valor));
  // Si el valor cambia desde fuera (otro cálculo), se refleja salvo que ya coincida con lo escrito.
  useEffect(() => {
    if (parsearSoles(texto) !== (valor ?? null)) setTexto(valor == null ? '' : ctsATexto(valor));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [valor]);
  return (
    <div className="relative">
      <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-texto-apoyo">S/</span>
      <input
        type="text"
        inputMode="decimal"
        autoComplete="off"
        className={`${className} pl-9 tabular-nums`}
        value={texto}
        onChange={(e) => {
          setTexto(e.target.value);
          onCambio?.(parsearSoles(e.target.value));
        }}
        onBlur={() => {
          const cts = parsearSoles(texto);
          if (cts !== null) setTexto(ctsATexto(cts));
        }}
        {...resto}
      />
    </div>
  );
}
