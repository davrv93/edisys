import { api, lista } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { ruta } from '../../lib/nav.jsx';
import { useSesion } from '../../layout/Sesion.jsx';
import { Icono } from '../../ui/index.js';

/**
 * Bloques E2 y E5 en el portal: los últimos comunicados publicados y el acceso a
 * preguntas frecuentes, academia y beneficios. Si no hay nada que mostrar, no ocupa sitio.
 */
export default function ComunicadosPortal({ eid }) {
  const s = useSesion();
  const veAnuncios = s.tiene?.('anuncios.ver');
  const veAyuda = s.tiene?.('contenido.ver');
  const { datos } = useCarga(() => api.get(`/edificios/${eid}/anuncios`, { limite: 3 }), [eid], { activo: !!veAnuncios });
  const recientes = lista(datos);
  if (!veAnuncios && !veAyuda) return null;
  if (!recientes.length && !veAyuda) return null;
  return (
    <section className="flex flex-col gap-2 bg-superficie px-4 py-4 lg:rounded-tarjeta lg:border lg:border-borde lg:p-5">
      {recientes.length > 0 && (
        <>
          <div className="flex items-center justify-between">
            <h2 className="text-base font-semibold">Comunicados</h2>
            <a href={ruta('anuncios')} className="text-sm font-semibold text-acento">Ver todos</a>
          </div>
          <ul className="divide-y divide-borde">
            {recientes.map((a) => (
              <li key={a.id} className="flex flex-col gap-0.5 py-2">
                <span className="text-sm font-medium text-tinta">{a.titulo}</span>
                <span className="line-clamp-2 text-xs text-texto-apoyo">{formatearFecha(a.fecha)} · {a.cuerpo}</span>
              </li>
            ))}
          </ul>
        </>
      )}
      {veAyuda && (
        <div className="flex flex-wrap gap-3 pt-1 text-sm font-semibold">
          <a href={ruta('ayuda', { vista: 'faq' })} className="inline-flex items-center gap-1 text-acento"><Icono nombre="info" tam={14} />Preguntas frecuentes</a>
          <a href={ruta('ayuda', { vista: 'academia' })} className="inline-flex items-center gap-1 text-acento"><Icono nombre="documento" tam={14} />Academia</a>
          <a href={ruta('ayuda', { vista: 'beneficios' })} className="inline-flex items-center gap-1 text-acento"><Icono nombre="pagado" tam={14} />Beneficios</a>
        </div>
      )}
    </section>
  );
}
