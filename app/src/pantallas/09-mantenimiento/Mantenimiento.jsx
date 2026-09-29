import { useQuery } from '../../lib/nav.jsx';
import { useSesion } from '../../layout/Sesion.jsx';
import { veTableroMantenimiento } from '../../lib/permisos.js';
import Tablero from './Tablero.jsx';
import Reportar from './Reportar.jsx';

/** 09 · Mantenimiento. Tablero kanban (administración, junta, técnico) o reporte con foto (?reportar=1). */
export default function Mantenimiento() {
  const s = useSesion();
  const [q] = useQuery();
  const veTablero = veTableroMantenimiento(s.tiene);
  if (q.get('reportar') || !veTablero) return <Reportar />;
  return <Tablero />;
}
