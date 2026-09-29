import { useQuery } from '../../lib/nav.jsx';
import { ruta } from '../../lib/nav.jsx';
import { useSesion, Guarda } from '../../layout/Sesion.jsx';
import { veTableroMantenimiento } from '../../lib/permisos.js';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { Boton } from '../../ui/index.js';
import Tablero from './Tablero.jsx';
import Plan from './Plan.jsx';
import Reportar from './Reportar.jsx';

/** 09 · Mantenimiento. Tablero kanban o plan de trabajo (?tab=plan); reporte con foto (?reportar=1). */
export default function Mantenimiento() {
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const veTablero = veTableroMantenimiento(s.tiene);
  if (q.get('reportar') || !veTablero) return <Reportar />;
  const tab = q.get('tab') === 'plan' ? 'plan' : 'tablero';
  const acciones = (
    <Guarda permiso="incidencias.reportar">
      <Boton icono="camara" href={ruta('mantenimiento', { reportar: 1 })}>
        Registrar incidencia
      </Boton>
    </Guarda>
  );
  return (
    <>
      <Encabezado titulo="Mantenimiento e incidencias" acciones={acciones}>
        <div role="tablist" aria-label="Vistas de mantenimiento" className="flex gap-1 overflow-x-auto px-4 lg:px-8">
          {[
            { id: 'tablero', etiqueta: 'Tablero' },
            { id: 'plan', etiqueta: 'Plan de trabajo' },
          ].map((t) => (
            <button
              key={t.id}
              type="button"
              role="tab"
              aria-selected={tab === t.id}
              onClick={() => setQuery({ tab: t.id === 'tablero' ? null : t.id })}
              className={`h-11 whitespace-nowrap border-b-2 px-3 text-sm font-semibold transition-colors duration-rapida ${tab === t.id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}
            >
              {t.etiqueta}
            </button>
          ))}
        </div>
      </Encabezado>
      <Contenido className="lg:max-w-none">{tab === 'plan' ? <Plan /> : <Tablero sinMarco />}</Contenido>
    </>
  );
}
