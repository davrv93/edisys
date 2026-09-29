import { migasPara } from '../lib/migas.js';

/** Migas Grupo › Sección › Subnivel, leídas de la URL actual. En móvil, las dos últimas. */
export default function Migas() {
  if (typeof window === 'undefined') return null;
  const migas = migasPara(window.location.pathname, window.location.search);
  if (migas.length < 2) return null;
  return (
    <nav aria-label="Migas" className="min-w-0">
      <ol className="flex min-w-0 items-center gap-1 text-xs text-texto-apoyo">
        {migas.map((m, i) => {
          const ultima = i === migas.length - 1;
          const esconder = i < migas.length - 2 ? 'hidden sm:inline' : '';
          return (
            <li key={`${m.etiqueta}-${i}`} className={`flex min-w-0 items-center gap-1 ${esconder}`} aria-current={ultima ? 'page' : undefined}>
              {i > 0 && <span aria-hidden="true" className="text-borde-fuerte">›</span>}
              {m.href && !ultima ? (
                <a href={m.href} className="truncate hover:text-tinta hover:underline">{m.etiqueta}</a>
              ) : (
                <span className={ultima ? 'truncate font-semibold text-texto-suave' : 'truncate'}>{m.etiqueta}</span>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
