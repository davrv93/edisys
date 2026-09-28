// Copia local del preset compartido (packages/tokens/tailwind-preset.js, §2.1).
// Va dentro de login/ para que el contexto de `docker build` sea solo esta carpeta.
export default {
  theme: {
    extend: {
      colors: {
        fondo: "var(--color-fondo)",
        superficie: "var(--color-superficie)",
        tinta: "var(--color-tinta)",
        "texto-suave": "var(--color-texto-suave)",
        "texto-apoyo": "var(--color-texto-apoyo)",
        "texto-oscuro": "var(--color-texto-oscuro)",
        "texto-oscuro-apoyo": "var(--color-texto-oscuro-apoyo)",
        "superficie-oscura": {
          DEFAULT: "var(--color-superficie-oscura)",
          2: "var(--color-superficie-oscura-2)",
        },
        borde: { DEFAULT: "var(--color-borde)", fuerte: "var(--color-borde-fuerte)" },
        acento: {
          DEFAULT: "var(--color-acento)",
          hover: "var(--color-acento-hover)",
          suave: "var(--color-acento-suave)",
          borde: "var(--color-acento-borde)",
          oscuro: "var(--color-acento-oscuro)",
        },
        alerta: { DEFAULT: "var(--color-alerta)", suave: "var(--color-alerta-suave)", borde: "var(--color-alerta-borde)" },
        aviso: { DEFAULT: "var(--color-aviso)", suave: "var(--color-aviso-suave)", borde: "var(--color-aviso-borde)" },
        ingreso: "var(--color-ingreso)",
        egreso: "var(--color-egreso)",
      },
      fontFamily: {
        titulo: ["Fraunces", "Georgia", "serif"],
        sans: ['"Public Sans"', "system-ui", "sans-serif"],
      },
      spacing: { sidebar: "248px", topbar: "72px", 13: "52px" },
    },
  },
};
