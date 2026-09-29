import Boton from './Boton.jsx';
import Icono from './Icono.jsx';
import { Isotipo } from './Logo.jsx';

/** Estado vacío con título, texto y acciones. */
export function Vacio({ titulo = 'Aún no hay nada aquí', texto, icono = 'info', children, compacto = false }) {
  return (
    <div className={`flex flex-col items-center justify-center gap-3 text-center ${compacto ? 'py-8' : 'py-16'} px-4`}>
      <span className="flex h-12 w-12 items-center justify-center rounded-full bg-acento-suave text-acento">
        <Icono nombre={icono} tam={24} />
      </span>
      <h2 className="text-lg font-semibold text-tinta">{titulo}</h2>
      {texto && <p className="max-w-md text-base text-texto-suave">{texto}</p>}
      {children && <div className="mt-2 flex flex-wrap justify-center gap-2">{children}</div>}
    </div>
  );
}

/** Error de carga con «Reintentar» y el id de la petición, si el API lo dio. */
export function ErrorCarga({ error, onReintentar, compacto = false }) {
  const msg = error?.message || 'No pudimos cargar los datos.';
  return (
    <div role="alert" className={`flex flex-col items-center gap-3 text-center ${compacto ? 'py-6' : 'py-16'} px-4`}>
      <span className="flex h-12 w-12 items-center justify-center rounded-full bg-alerta-suave text-alerta">
        <Icono nombre="alerta" tam={24} />
      </span>
      <h2 className="text-lg font-semibold text-tinta">No se pudo cargar</h2>
      <p className="max-w-md text-base text-texto-suave">{msg}</p>
      {error?.idPeticion && <p className="text-xs text-texto-apoyo">Código para soporte: {error.idPeticion}</p>}
      {onReintentar && (
        <Boton variante="secundario" icono="reloj" onClick={onReintentar}>
          Reintentar
        </Boton>
      )}
    </div>
  );
}

const QUIEN = {
  administrador: 'la administración del edificio',
  junta: 'la junta directiva o la administración',
};

/** Sin permiso: explica qué rol hace falta y a quién pedirlo. Nunca pantalla en blanco. */
export function SinPermiso({ permiso, rol = 'administrador' }) {
  return (
    <div className="flex flex-col items-center gap-3 px-4 py-16 text-center">
      <span className="flex h-12 w-12 items-center justify-center rounded-full bg-superficie-2 text-texto-suave">
        <Icono nombre="candado" tam={24} />
      </span>
      <h2 className="text-lg font-semibold text-tinta">Esta sección no está habilitada para ti</h2>
      <p className="max-w-md text-base text-texto-suave">
        Hace falta el permiso <code className="rounded bg-superficie-2 px-1.5 py-0.5 text-sm">{permiso || 'correspondiente'}</code>. Pídeselo a {QUIEN[rol] || 'la administración del edificio'}.
      </p>
    </div>
  );
}

/** Bloque gris animado para estados de carga. */
export function Esqueleto({ className = 'h-4 w-full' }) {
  return <div className={`esqueleto rounded-control ${className}`} aria-hidden="true" />;
}

/** Pantalla de carga con el logo (arranque de la sesión). */
export function CargandoApp({ texto = 'Cargando EDISYS…' }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-fondo" role="status">
      <div className="animate-latido-suave">
        <Isotipo tam={48} />
      </div>
      <p className="text-sm text-texto-apoyo">{texto}</p>
    </div>
  );
}
