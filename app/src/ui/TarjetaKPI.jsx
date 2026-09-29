import Icono from './Icono.jsx';
import { Esqueleto } from './EstadosPantalla.jsx';
import { Variacion } from './FranjaKPI.jsx';

const TONOS = {
  neutro: { caja: 'bg-superficie border-borde', titulo: 'text-texto-apoyo', valor: 'text-tinta', nota: 'text-texto-apoyo' },
  acento: { caja: 'bg-acento-suave border-acento-borde', titulo: 'text-acento-hover', valor: 'text-acento', nota: 'text-acento-hover' },
  alerta: { caja: 'bg-alerta-suave border-alerta-borde', titulo: 'text-alerta-texto', valor: 'text-alerta', nota: 'text-alerta-texto' },
  aviso: { caja: 'bg-aviso-suave border-aviso-borde', titulo: 'text-aviso-texto', valor: 'text-aviso', nota: 'text-aviso-texto' },
};

/**
 * KPI: título, valor grande (Fraunces, tabular), nota, variación con TEXTO (no solo color), barra opcional y enlace.
 * variacion: { texto: '+12 % vs. agosto', buena: true|false }
 */
export default function TarjetaKPI({ titulo, valor, nota, tono = 'neutro', variacion, icono, to, barra, cargando, tamValor = 'text-xl lg:text-kpi', verDetalle }) {
  const t = TONOS[tono] || TONOS.neutro;
  const cuerpo = (
    <>
      <span className={`flex items-center gap-2 text-xs ${t.titulo}`}>
        {icono && <Icono nombre={icono} tam={16} />}
        {titulo}
      </span>
      {cargando ? (
        <Esqueleto className="h-8 w-3/4" />
      ) : (
        <span className={`whitespace-nowrap font-titulo font-semibold tabular-nums leading-tight ${tamValor} ${t.valor}`}>{valor}</span>
      )}
      {barra != null && (
        <div className="h-1.5 rounded-full bg-borde" role="progressbar" aria-valuenow={Math.round(barra)} aria-valuemin={0} aria-valuemax={100}>
          <div className="h-1.5 rounded-full bg-acento" style={{ width: `${Math.min(100, Math.max(0, barra))}%` }} />
        </div>
      )}
      {nota && <span className={`text-xs ${t.nota}`}>{nota}</span>}
      <Variacion variacion={variacion} />
      {to && verDetalle && <span className="text-xs font-semibold text-acento">{verDetalle} →</span>}
    </>
  );
  const clases = `flex min-w-0 flex-col gap-1 rounded-tarjeta border p-tarjeta ${t.caja}`;
  if (to) {
    return (
      <a href={to} className={`${clases} text-tinta transition-colors duration-rapida hover:border-acento hover:text-tinta`}>
        {cuerpo}
      </a>
    );
  }
  return <div className={clases}>{cuerpo}</div>;
}
