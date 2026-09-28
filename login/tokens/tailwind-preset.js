// packages/tokens/tailwind-preset.js
// Preset compartido por app (React), login (Qwik) y landing (Astro).
// Los valores viven en tokens.css; aquí solo se nombran.
// Regla de revisión: las pantallas usan estos nombres, nunca clases de color crudas.
export default {
  theme: {
    extend: {
      colors: {
        fondo: 'var(--color-fondo)', // slate-50
        superficie: {
          DEFAULT: 'var(--color-superficie)', // white
          2: 'var(--color-superficie-2)', // slate-100
          oscura: 'var(--color-superficie-oscura)', // slate-800
          'oscura-2': 'var(--color-superficie-oscura-2)', // slate-700
        },
        tinta: 'var(--color-tinta)', // slate-900
        'texto-suave': 'var(--color-texto-suave)', // slate-600
        'texto-apoyo': 'var(--color-texto-apoyo)', // slate-500
        'texto-tenue': 'var(--color-texto-tenue)', // slate-400
        'texto-claro': 'var(--color-texto-claro)', // slate-300
        borde: { DEFAULT: 'var(--color-borde)', fuerte: 'var(--color-borde-fuerte)' },
        acento: {
          DEFAULT: 'var(--color-acento)', // cyan-800 #155E75
          hover: 'var(--color-acento-hover)', // cyan-900
          suave: 'var(--color-acento-suave)', // cyan-50
          borde: 'var(--color-acento-borde)', // cyan-100
          oscuro: 'var(--color-acento-oscuro)', // cyan-300
        },
        alerta: {
          DEFAULT: 'var(--color-alerta)', // red-700
          suave: 'var(--color-alerta-suave)', // red-50
          borde: 'var(--color-alerta-borde)', // red-200
          texto: 'var(--color-alerta-texto)', // red-800
          pista: 'var(--color-alerta-pista)', // red-100
        },
        aviso: {
          DEFAULT: 'var(--color-aviso)', // amber-700
          suave: 'var(--color-aviso-suave)', // amber-50
          borde: 'var(--color-aviso-borde)', // amber-200
          texto: 'var(--color-aviso-texto)', // amber-800
        },
        ingreso: 'var(--color-ingreso)',
        egreso: 'var(--color-egreso)',
      },
      fontFamily: {
        titulo: ['Fraunces', 'Georgia', 'serif'],
        sans: ['"Public Sans"', 'system-ui', 'sans-serif'],
      },
      spacing: { sidebar: '248px', topbar: '72px', 13: '52px' },
      maxWidth: { contenido: '1280px' },
    },
  },
};
