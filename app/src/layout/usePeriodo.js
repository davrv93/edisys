import { useQuery } from '../lib/nav.jsx';
import { esPeriodo, periodoActual } from '../lib/fechas.js';
import { useSesion } from './Sesion.jsx';

/** Periodo de la pantalla: ?periodo=AAAA-MM o, por defecto, el periodo abierto del edificio. */
export function usePeriodo() {
  const s = useSesion();
  const [q, setQuery] = useQuery();
  const porDefecto = s.edificio.periodo_abierto || periodoActual();
  const p = q.get('periodo');
  const periodo = esPeriodo(p) ? p : porDefecto;
  const setPeriodo = (nuevo) => setQuery({ periodo: nuevo === porDefecto ? null : nuevo }, { reemplazar: true });
  return [periodo, setPeriodo, porDefecto];
}
