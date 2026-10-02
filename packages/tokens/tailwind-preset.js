// packages/tokens/tailwind-preset.js
// Preset compartido por app (React), login (Qwik) y landing (Astro).
// Los valores viven en tokens.css; aquí solo se nombran.
// Regla de revisión: las pantallas usan estos nombres, nunca clases de color crudas
// ni valores sueltos de radio, sombra o duración (v2: el lint de la app lo comprueba).
//
// Compatibilidad: los nombres por defecto de Tailwind (rounded-lg, rounded-xl, shadow-md,
// duration-200…) siguen funcionando, pero ahora resuelven a los tokens, para que el código
// que aún los use no se salga del sistema.

const radioControl = 'var(--radio-control)';
const radioTarjeta = 'var(--radio-tarjeta)';
const flotante = 'var(--sombra-flotante)';

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
        curso: {
          DEFAULT: 'var(--color-curso)', // indigo-600
          suave: 'var(--color-curso-suave)', // indigo-50
          borde: 'var(--color-curso-borde)', // indigo-200
          texto: 'var(--color-curso-texto)', // indigo-800
        },
        hecho: {
          DEFAULT: 'var(--color-hecho)', // slate-500
          suave: 'var(--color-hecho-suave)', // slate-100
          borde: 'var(--color-hecho-borde)', // slate-200
          texto: 'var(--color-hecho-texto)', // slate-600
        },
        serie: {
          1: 'var(--color-serie-1)',
          2: 'var(--color-serie-2)',
          3: 'var(--color-serie-3)',
          4: 'var(--color-serie-4)',
          5: 'var(--color-serie-5)',
        },
        ingreso: 'var(--color-ingreso)',
        egreso: 'var(--color-egreso)',
      },
      fontFamily: {
        titulo: ['Fraunces', 'Georgia', 'serif'],
        sans: ['"Plus Jakarta Sans"', 'system-ui', 'sans-serif'],
      },
      fontSize: {
        cuerpo: ['var(--texto-cuerpo)', { lineHeight: '1.45' }],
        kpi: ['var(--texto-kpi)', { lineHeight: '1.1', letterSpacing: '-0.01em' }],
        'titulo-pantalla': ['var(--texto-titulo-pantalla)', { lineHeight: '1.2' }],
      },
      spacing: {
        sidebar: 'var(--ancho-lateral)', // 220 px (v1: 248)
        'sidebar-iconos': 'var(--ancho-lateral-iconos)', // 64 px
        topbar: 'var(--alto-encabezado)', // 56 px (v1: 72)
        control: 'var(--alto-control)', // 36 px
        'control-tactil': 'var(--alto-control-tactil)', // 44 px
        fila: 'var(--alto-fila)', // 40 px
        tarjeta: 'var(--pad-tarjeta)', // 16 px
        13: '52px',
      },
      maxWidth: { contenido: '1280px' },
      borderRadius: {
        mini: 'var(--radio-mini)',
        control: radioControl,
        tarjeta: radioTarjeta,
        chip: 'var(--radio-chip)',
        // Nombres por defecto → tokens (compatibilidad).
        sm: 'var(--radio-mini)',
        DEFAULT: 'var(--radio-mini)',
        md: radioControl,
        lg: radioControl,
        xl: radioTarjeta,
        '2xl': radioTarjeta,
        '3xl': radioTarjeta,
      },
      boxShadow: {
        flotante: flotante,
        sm: flotante,
        DEFAULT: flotante,
        md: flotante,
        lg: flotante,
        xl: flotante,
        '2xl': flotante,
      },
      transitionDuration: {
        DEFAULT: 'var(--dur-rapida)',
        rapida: 'var(--dur-rapida)',
        media: 'var(--dur-media)',
        lenta: 'var(--dur-lenta)',
        75: 'var(--dur-rapida)',
        100: 'var(--dur-rapida)',
        150: 'var(--dur-rapida)',
        200: 'var(--dur-media)',
        300: 'var(--dur-lenta)',
        500: 'var(--dur-lenta)',
        700: 'var(--dur-lenta)',
        1000: 'var(--dur-lenta)',
      },
      transitionTimingFunction: {
        DEFAULT: 'var(--ease-salida)',
        salida: 'var(--ease-salida)',
        entrada: 'var(--ease-entrada)',
      },
      scale: { 97: '.97', 98: '.98' },
      keyframes: {
        aparecer: { from: { opacity: '0', transform: 'translateY(6px)' }, to: { opacity: '1', transform: 'none' } },
        desplegar: { from: { opacity: '0', transform: 'translateY(-4px) scale(.98)' }, to: { opacity: '1', transform: 'none' } },
        brillo: { from: { backgroundPosition: '150% 0' }, to: { backgroundPosition: '-50% 0' } },
        'latido-suave': { '0%, 100%': { opacity: '1', transform: 'scale(1)' }, '50%': { opacity: '.45', transform: 'scale(.85)' } },
        asentar: { '0%': { transform: 'translateY(-6px) scale(1.02)' }, '60%': { transform: 'translateY(1px) scale(.995)' }, '100%': { transform: 'none' } },
        temblor: { '0%, 100%': { transform: 'none' }, '20%, 60%': { transform: 'translateX(-4px)' }, '40%, 80%': { transform: 'translateX(4px)' } },
        fundir: { from: { opacity: '0' }, to: { opacity: '1' } },
        'fundir-salida': { from: { opacity: '1' }, to: { opacity: '0' } },
        'escala-entrar': { from: { opacity: '0', transform: 'scale(.97)' }, to: { opacity: '1', transform: 'none' } },
        'escala-salir': { from: { opacity: '1', transform: 'none' }, to: { opacity: '0', transform: 'scale(.97)' } },
        'entrar-derecha': { from: { opacity: '0', transform: 'translateX(16px)' }, to: { opacity: '1', transform: 'none' } },
        'entrar-izquierda': { from: { opacity: '0', transform: 'translateX(-16px)' }, to: { opacity: '1', transform: 'none' } },
        'entrar-abajo': { from: { opacity: '0', transform: 'translateY(16px)' }, to: { opacity: '1', transform: 'none' } },
        'crecer-y': { from: { transform: 'scaleY(0)' }, to: { transform: 'none' } },
        'crecer-x': { from: { transform: 'scaleX(0)' }, to: { transform: 'none' } },
        trazar: { from: { strokeDashoffset: '1' }, to: { strokeDashoffset: '0' } },
        consumir: { from: { transform: 'scaleX(1)' }, to: { transform: 'scaleX(0)' } },
        escribiendo: { '0%, 80%, 100%': { opacity: '.3', transform: 'translateY(0)' }, '40%': { opacity: '1', transform: 'translateY(-3px)' } },
      },
      animation: {
        aparecer: 'aparecer var(--dur-media) var(--ease-salida) both',
        desplegar: 'desplegar var(--dur-media) var(--ease-salida) both',
        brillo: 'brillo 1.4s linear infinite',
        'latido-suave': 'latido-suave 1.8s ease-in-out infinite',
        asentar: 'asentar var(--dur-lenta) var(--ease-salida) both',
        temblor: 'temblor var(--dur-lenta) var(--ease-salida) both',
        fundir: 'fundir var(--dur-media) var(--ease-salida) both',
        'fundir-salida': 'fundir-salida var(--dur-rapida) var(--ease-entrada) both',
        'escala-entrar': 'escala-entrar var(--dur-media) var(--ease-salida) both',
        'escala-salir': 'escala-salir var(--dur-rapida) var(--ease-entrada) both',
        'entrar-derecha': 'entrar-derecha var(--dur-media) var(--ease-salida) both',
        'entrar-izquierda': 'entrar-izquierda var(--dur-media) var(--ease-salida) both',
        'entrar-abajo': 'entrar-abajo var(--dur-media) var(--ease-salida) both',
        'crecer-y': 'crecer-y var(--dur-lenta) var(--ease-salida) both',
        'crecer-x': 'crecer-x var(--dur-lenta) var(--ease-salida) both',
        trazar: 'trazar var(--dur-lenta) var(--ease-salida) both',
        consumir: 'consumir 4s linear both',
        escribiendo: 'escribiendo 1.2s ease-in-out infinite',
        // v1 usaba animate-pulse: ahora es el barrido del esqueleto.
        pulse: 'brillo 1.4s linear infinite',
      },
    },
  },
};
