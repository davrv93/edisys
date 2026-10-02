import { component$ } from "@builder.io/qwik";

/** Logotipo EDISYS: marca cuadrada con degradado y tres «pisos» sobre el wordmark en Fraunces. */
export const Logo = component$((props: { tam: number; texto: string }) => (
  <span class="inline-flex items-center gap-2.5">
    <svg width={props.tam} height={props.tam} viewBox="0 0 32 32" aria-hidden="true">
      <defs>
        <linearGradient id="edisysIso" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stop-color="#0e9bb5" />
          <stop offset="1" stop-color="#155e75" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="9" fill="url(#edisysIso)" />
      <rect x="8" y="19.4" width="16" height="3.5" rx="1.75" fill="#FFFFFF" opacity="0.98" />
      <rect x="10" y="14" width="12" height="3.5" rx="1.75" fill="#FFFFFF" opacity="0.82" />
      <rect x="12" y="8.6" width="8" height="3.5" rx="1.75" fill="#FFFFFF" opacity="0.62" />
    </svg>
    <span class={["font-titulo text-xl font-semibold tracking-tight text-white", props.texto]}>EDISYS</span>
  </span>
));
