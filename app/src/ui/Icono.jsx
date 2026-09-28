// Iconos de trazo (24 × 24). Siempre acompañan a un texto: nunca son el único portador del significado.
const RUTAS = {
  inicio: 'M3 11l9-7 9 7M5 10v10h5v-6h4v6h5V10',
  balance: 'M12 3v18M5 7h14M7 7l-3 7a3 3 0 006 0L7 7zm10 0l-3 7a3 3 0 006 0l-3-7z',
  recibo: 'M6 3h12v18l-3-2-3 2-3-2-3 2V3zM9 8h6M9 12h6M9 16h3',
  edificio: 'M4 21V5l8-2v18M12 7l8 2v12M7 8h2M7 12h2M7 16h2M15 12h2M15 16h2M2 21h20',
  calendario: 'M4 6h16v15H4zM4 10h16M8 3v4M16 3v4',
  medidor: 'M12 21a8 8 0 100-16 8 8 0 000 16zM12 13l3-4M12 3v2',
  herramienta: 'M14.7 6.3a4 4 0 00-5.4 5.4L3 18l3 3 6.3-6.3a4 4 0 005.4-5.4l-2.6 2.6-2.4-.6-.6-2.4 2.6-2.6z',
  camara: 'M4 8h3l2-3h6l2 3h3v12H4zM12 17a4 4 0 100-8 4 4 0 000 8z',
  mensaje: 'M4 5h16v11H9l-5 4V5zM8 9h8M8 12h5',
  grafico: 'M4 20V4M4 20h16M8 16v-4M12 16V8M16 16v-6',
  llave: 'M15 7a4 4 0 11-3.5 6L4 20v-3h3v-3h3l1.5-1.5A4 4 0 0115 7z',
  mas: 'M5 12h.01M12 12h.01M19 12h.01',
  menu: 'M4 6h16M4 12h16M4 18h16',
  salir: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
  volver: 'M15 5l-7 7 7 7',
  izq: 'M15 5l-7 7 7 7',
  der: 'M9 5l7 7-7 7',
  abajo: 'M5 9l7 7 7-7',
  arriba: 'M5 15l7-7 7 7',
  cerrar: 'M6 6l12 12M18 6L6 18',
  check: 'M5 12l5 5 9-10',
  alerta: 'M12 3l10 18H2L12 3zM12 10v4M12 17h.01',
  info: 'M12 22a10 10 0 100-20 10 10 0 000 20zM12 11v6M12 7h.01',
  buscar: 'M11 18a7 7 0 100-14 7 7 0 000 14zM21 21l-5-5',
  filtro: 'M4 5h16l-6 8v6l-4-2v-4L4 5z',
  subir: 'M12 16V4M7 9l5-5 5 5M4 20h16',
  descargar: 'M12 4v12M7 11l5 5 5-5M4 20h16',
  enviar: 'M4 12l16-8-6 16-2-6-8-2z',
  candado: 'M6 11h12v10H6zM8 11V7a4 4 0 118 0v4',
  documento: 'M6 3h8l4 4v14H6zM14 3v4h4M9 13h6M9 17h6',
  usuario: 'M12 12a4 4 0 100-8 4 4 0 000 8zM4 21a8 8 0 0116 0',
  mas_signo: 'M12 5v14M5 12h14',
  arrastrar: 'M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01',
  reloj: 'M12 22a10 10 0 100-20 10 10 0 000 20zM12 7v5l3 2',
  whatsapp: 'M4 20l1.3-4A8 8 0 1112 20a8 8 0 01-4-1l-4 1zM9 9c0 3 3 6 6 6l1-1.5-2-1-1 1c-1-.5-2-1.5-2.5-2.5l1-1-1-2L9 9z',
  robot: 'M6 8h12v11H6zM12 4v4M9 13h.01M15 13h.01M9 16h6M3 12v3M21 12v3',
  engranaje: 'M12 15a3 3 0 100-6 3 3 0 000 6zM19 12l2-1-1-3-2 .5-1.5-1.5.5-2-3-1-1 2h-2l-1-2-3 1 .5 2L6 7.5 4 7 3 10l2 1v2l-2 1 1 3 2-.5L7.5 18 7 20l3 1 1-2h2l1 2 3-1-.5-2 1.5-1.5 2 .5 1-3-2-1v-2z',
};

export default function Icono({ nombre, tam = 20, className = '', grosor = 2, titulo }) {
  const d = RUTAS[nombre] || RUTAS.info;
  return (
    <svg
      width={tam}
      height={tam}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={grosor}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={'shrink-0 ' + className}
      aria-hidden={titulo ? undefined : 'true'}
      role={titulo ? 'img' : undefined}
    >
      {titulo && <title>{titulo}</title>}
      <path d={d} />
    </svg>
  );
}
