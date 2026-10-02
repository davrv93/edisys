// Logotipo EDISYS: marca cuadrada con acabado en degradado y tres «pisos» que se apagan,
// sobre el wordmark en Fraunces. Un solo idioma visual para app, login y landing.
export function Isotipo({ tam = 32 }) {
  return (
    <svg width={tam} height={tam} viewBox="0 0 32 32" aria-hidden="true" className="shrink-0">
      <defs>
        <linearGradient id="edisysIso" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#0e9bb5" />
          <stop offset="1" stopColor="#155e75" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="9" fill="url(#edisysIso)" />
      <rect x="8" y="19.4" width="16" height="3.5" rx="1.75" fill="#FFFFFF" opacity="0.98" />
      <rect x="10" y="14" width="12" height="3.5" rx="1.75" fill="#FFFFFF" opacity="0.82" />
      <rect x="12" y="8.6" width="8" height="3.5" rx="1.75" fill="#FFFFFF" opacity="0.62" />
    </svg>
  );
}

export default function Logo({ tam = 32, claro = false }) {
  return (
    <span className="inline-flex items-center gap-2.5">
      <Isotipo tam={tam} />
      <span className={`font-titulo text-xl font-semibold tracking-tight ${claro ? 'text-white' : 'text-tinta'}`}>EDISYS</span>
    </span>
  );
}
