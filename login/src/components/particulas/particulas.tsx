import { component$ } from "@builder.io/qwik";

/** Aleatorio determinista: el SSR y el cliente pintan las mismas partículas (sin salto de hidratación). */
function azar(semilla: number) {
  let s = semilla >>> 0;
  return () => {
    s = (s * 1664525 + 1013904223) >>> 0;
    return s / 4294967296;
  };
}

interface Particula {
  izq: string;
  abajo: string;
  lado: string;
  fondo: string;
  duracion: string;
  retraso: string;
  deriva: string;
  opacidad: string;
}

/** Campo de partículas flotantes (solo CSS): puntitos blancos y cian que suben y parpadean.
 *  `cantidad` según el lienzo (panel 26, cabecera móvil 12). Decorativo: aria-hidden. */
export const Particulas = component$(({ cantidad = 24 }: { cantidad?: number }) => {
  const r = azar(20260929);
  const ps: Particula[] = Array.from({ length: cantidad }, (_, i) => {
    const tam = 2 + r() * 3.5;
    return {
      izq: `${(r() * 100).toFixed(2)}%`,
      abajo: `${(r() * 100).toFixed(2)}%`,
      lado: `${tam.toFixed(1)}px`,
      fondo: i % 3 === 2 ? "var(--color-acento-oscuro)" : "#ffffff",
      duracion: `${(7 + r() * 8).toFixed(2)}s`,
      retraso: `${(-r() * 15).toFixed(2)}s`,
      deriva: `${((r() - 0.5) * 56).toFixed(1)}px`,
      opacidad: (0.1 + r() * 0.28).toFixed(2),
    };
  });
  return (
    <div aria-hidden="true" class="pointer-events-none absolute inset-0 overflow-hidden">
      {ps.map((p, i) => (
        <span
          key={i}
          class="particula"
          style={{
            left: p.izq,
            bottom: p.abajo,
            width: p.lado,
            height: p.lado,
            background: p.fondo,
            animationDuration: p.duracion,
            animationDelay: p.retraso,
            "--deriva": p.deriva,
            "--op": p.opacidad,
          }}
        />
      ))}
    </div>
  );
});
