import { useQuery } from '../../lib/nav.jsx';
import { useEid, useSesion } from '../../layout/Sesion.jsx';
import Encabezado, { Contenido } from '../../layout/Encabezado.jsx';
import { SinPermiso } from '../../ui/index.js';
import FacturacionElectronica from './FacturacionElectronica.jsx';

const PESTANAS = [['facturacion', 'Facturación electrónica', 'facturacion.configurar']];

/** Configuración del edificio, por pestañas (?tab=). Hoy: Facturación electrónica (SUNAT). */
export default function Configuracion() {
  const eid = useEid();
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const visibles = PESTANAS.filter(([, , p]) => s.tiene(p));
  const tab = visibles.find(([id]) => id === q.get('tab'))?.[0] || visibles[0]?.[0];
  return (
    <>
      <Encabezado titulo="Configuración">
        <div className="flex gap-2 overflow-x-auto px-4 lg:px-8" role="tablist">
          {visibles.map(([id, etiqueta]) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={tab === id}
              onClick={() => setQuery({ tab: id })}
              className={`h-12 whitespace-nowrap border-b-2 px-3 text-sm font-semibold ${tab === id ? 'border-acento text-acento' : 'border-transparent text-texto-suave hover:text-tinta'}`}
            >
              {etiqueta}
            </button>
          ))}
        </div>
      </Encabezado>
      <Contenido>{tab === 'facturacion' ? <FacturacionElectronica eid={eid} /> : <SinPermiso permiso="facturacion.configurar" />}</Contenido>
    </>
  );
}
