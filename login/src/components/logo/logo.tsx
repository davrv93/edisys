import { component$ } from "@builder.io/qwik";

/** Logotipo: cuadrado petróleo con tres pisos escalonados + «EDISYS» en Fraunces. */
export const Logo = component$((props: { tam: number; texto: string }) => (
  <span class="flex items-center gap-3">
    <svg width={props.tam} height={props.tam} viewBox="0 0 32 32" aria-hidden="true">
      <rect width="32" height="32" rx="8" class="fill-acento" />
      <path
        d="M8 24h16M10.5 19h11M13 14h6M16 9v0.01"
        stroke="#FFFFFF"
        stroke-width="2.4"
        stroke-linecap="round"
        fill="none"
      />
    </svg>
    <span class={["font-titulo font-semibold tracking-[0.02em] text-white", props.texto]}>EDISYS</span>
  </span>
));
