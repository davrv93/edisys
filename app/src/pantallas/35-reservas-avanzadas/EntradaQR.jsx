import { useEffect, useState } from 'react';
import { api } from '../../lib/api.js';
import { diaLima, etiquetaDia, formatearHora } from '../../lib/fechas.js';
import { Boton, Esqueleto, Modal } from '../../ui/index.js';

/**
 * Bloque H1 · la entrada de la reserva (como la del cine): el QR que el conserje escanea el día del evento.
 * El QR lo genera el API (PNG en data URI); aquí solo se muestra. `reserva` = { id, codigo }.
 */
export default function EntradaQR({ eid, reserva, onCerrar }) {
  const [datos, setDatos] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!reserva) return undefined;
    let vivo = true;
    setDatos(null);
    setError(null);
    api
      .get(`/edificios/${eid}/reservas/${reserva.id}/qr`)
      .then((d) => vivo && setDatos(d))
      .catch((err) => vivo && setError(err));
    return () => {
      vivo = false;
    };
  }, [eid, reserva]);

  const yaEntro = datos?.checkin_valido === true;

  return (
    <Modal
      abierto={!!reserva}
      onCerrar={onCerrar}
      titulo={`Entrada ${reserva?.codigo || ''}`.trim()}
      ancho="max-w-sm"
      pie={
        <Boton variante="secundario" onClick={onCerrar}>
          Cerrar
        </Boton>
      }
    >
      {error ? (
        <p className="text-sm text-texto-suave">{error.message}</p>
      ) : !datos ? (
        <Esqueleto className="mx-auto h-64 w-64" />
      ) : (
        <div className="flex flex-col items-center gap-3 text-center">
          <img src={datos.png} alt={`Código QR de la reserva ${datos.codigo}`} width={256} height={256} className={`h-64 w-64 rounded-control border border-borde bg-white p-2 ${yaEntro ? 'opacity-40' : ''}`} />
          <div className="flex flex-col gap-0.5 text-sm">
            <b className="text-base">
              {datos.area} · {datos.recurso}
            </b>
            <span className="text-texto-suave">
              {etiquetaDia(diaLima(datos.inicio))} · {formatearHora(datos.inicio)}–{formatearHora(datos.fin)} · Dpto {datos.unidad}
            </span>
            {datos.titulo && <span className="text-texto-suave">«{datos.titulo}»</span>}
          </div>
          {yaEntro ? (
            <span className="text-sm font-semibold text-acento-hover">Ingreso registrado a las {formatearHora(datos.checkin_en)}.</span>
          ) : (
            <span className="text-xs text-texto-apoyo">Muéstralo al conserje el día del evento. Para entrar, la unidad debe seguir al día en sus pagos.</span>
          )}
        </div>
      )}
    </Modal>
  );
}
