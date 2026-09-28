// Logotipo: cuadrado petróleo con tres pisos escalonados + «EDISYS» en Fraunces.
export function Isotipo({ tam = 32 }) {
  return (
    <svg width={tam} height={tam} viewBox="0 0 32 32" aria-hidden="true" className="shrink-0">
      <rect width="32" height="32" rx="8" className="fill-acento" />
      <path d="M8 24h16M10.5 19h11M13 14h6M16 9v0.01" stroke="#FFFFFF" strokeWidth="2.4" strokeLinecap="round" />
    </svg>
  );
}

export default function Logo({ tam = 32, claro = false }) {
  return (
    <span className="inline-flex items-center gap-3">
      <Isotipo tam={tam} />
      <span className={`font-titulo text-2xl font-semibold tracking-wide ${claro ? 'text-white' : 'text-tinta'}`}>EDISYS</span>
    </span>
  );
}
