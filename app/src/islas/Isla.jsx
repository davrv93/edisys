import { lazy, Suspense, useEffect } from 'react';
import { SesionProvider, useSesion } from '../layout/Sesion.jsx';
import Armazon from '../layout/Armazon.jsx';
import Pwa from '../Pwa.jsx';
import { ToastProvider, Esqueleto, SinPermiso, CargandoApp } from '../ui/index.js';
import { destinoPorRol } from '../lib/permisos.js';
import { navegar, ruta } from '../lib/nav.jsx';

// Una entrada por página Astro. Cada pantalla se carga en su propio chunk.
const PANTALLAS = {
  inicio: { permiso: 'dashboard.ver', C: lazy(() => import('../pantallas/03-dashboard/Dashboard.jsx')) },
  balance: { permiso: 'balance.ver', C: lazy(() => import('../pantallas/04-balance/Balance.jsx')) },
  recibos: { permiso: ['recibos.ver', 'portal.ver'], C: lazy(() => import('../pantallas/05-recibos/Recibos.jsx')) },
  unidades: { permiso: 'unidades.ver', C: lazy(() => import('../pantallas/06-unidades/Unidades.jsx')) },
  reservas: { permiso: ['reservas.ver', 'reservas.crear'], C: lazy(() => import('../pantallas/07-reservas/Reservas.jsx')) },
  medidores: { permiso: ['lecturas.registrar', 'lecturas.ver'], C: lazy(() => import('../pantallas/08-medidores/Medidores.jsx')) },
  mantenimiento: { permiso: ['incidencias.ver', 'incidencias.reportar'], C: lazy(() => import('../pantallas/09-mantenimiento/Mantenimiento.jsx')) },
  portal: { permiso: 'portal.ver', C: lazy(() => import('../pantallas/10-portal/Portal.jsx')) },
  roles: { permiso: 'roles.administrar', C: lazy(() => import('../pantallas/11-roles/Roles.jsx')) },
  whatsapp: { permiso: 'whatsapp.ver', C: lazy(() => import('../pantallas/12-whatsapp/WhatsApp.jsx')) },
  chatbot: { permiso: 'chatbot.probar', C: lazy(() => import('../pantallas/12-whatsapp/Chatbot.jsx')) },
  motor: { permiso: 'motor.administrar', C: lazy(() => import('../pantallas/12-whatsapp/Motor.jsx')) },
  analitica: { permiso: 'analitica.ver', C: lazy(() => import('../pantallas/13-analitica/Analitica.jsx')) },
  conciliacion: { permiso: 'balance.conciliar', C: lazy(() => import('../pantallas/14-conciliacion/Conciliacion.jsx')) },
  proveedores: { permiso: ['proveedores.ver', 'cuentas_pagar.ver'], C: lazy(() => import('../pantallas/15-proveedores/Proveedores.jsx')) },
  fondos: { permiso: 'fondos.ver', C: lazy(() => import('../pantallas/16-fondos/Trazabilidad.jsx')) },
  informes: { permiso: 'balance.ver', C: lazy(() => import('../pantallas/17-informes/Informes.jsx')) },
  externos: { permiso: 'externos.ver', C: lazy(() => import('../pantallas/18-externos/Externos.jsx')) },
  vouchers: { permiso: ['pagos.registrar', 'pagos.informar'], C: lazy(() => import('../pantallas/19-vouchers/Vouchers.jsx')) },
  cobranzas: { permiso: 'cobranzas.ver', C: lazy(() => import('../pantallas/20-cobranzas/Cobranzas.jsx')) },
  documentos: { permiso: 'documentos.ver', C: lazy(() => import('../pantallas/21-documentos/Documentos.jsx')) },
  // marca: I1/I2/I5 · marca blanca y plantilla de recibo; configuración del edificio
  marca: { permiso: ['marca.configurar', 'recibos.plantilla'], C: lazy(() => import('../pantallas/36-marca/Marca.jsx')) },
  ajustes: { permiso: 'configuracion.ver', C: lazy(() => import('../pantallas/37-ajustes/Ajustes.jsx')) },
  // recaudacion: A1, A2
  recaudadora: { permiso: 'recaudacion.ver', C: lazy(() => import('../pantallas/38-recaudadora/Recaudadora.jsx')) },
  'cobranza-masiva': { permiso: 'recaudacion.ver', C: lazy(() => import('../pantallas/39-cobranza-masiva/CobranzaMasiva.jsx')) },
  configuracion: { permiso: 'facturacion.configurar', C: lazy(() => import('../pantallas/configuracion/Configuracion.jsx')) },
  // reservas: H1 · check-in QR del conserje · H2/H3 · configuración de áreas
  checkin: { permiso: 'reservas.checkin', C: lazy(() => import('../pantallas/35-reservas-avanzadas/Checkin.jsx')) },
  areascomunes: { permiso: 'areas.administrar', C: lazy(() => import('../pantallas/35-reservas-avanzadas/ConfigAreas.jsx')) },
};
// extras: J1/J2/I3 · encuestas; videollamadas y dominio propio (una pantalla con dos vistas)
PANTALLAS.encuestas = { permiso: 'encuestas.ver', C: lazy(() => import('../pantallas/40-encuestas/Encuestas.jsx')) };
PANTALLAS.videollamadas = { permiso: ['videollamadas.ver', 'dominios.administrar'], C: lazy(() => import('../pantallas/41-videollamadas/Videollamadas.jsx')) };

function CargandoPantalla() {
  return (
    <div className="flex flex-col gap-4 p-4 lg:p-8" aria-busy="true">
      <Esqueleto className="h-10 w-1/3" />
      <Esqueleto className="h-32 w-full" />
      <Esqueleto className="h-64 w-full" />
    </div>
  );
}

function Pantalla({ pantalla }) {
  const s = useSesion();
  const def = PANTALLAS[pantalla];
  const permitido = def && s.tiene(def.permiso);
  // /app/ es el dashboard; quien no lo ve aterriza en la pantalla de su rol (§3 · 01).
  const redirigir = pantalla === 'inicio' && !permitido;
  useEffect(() => {
    if (redirigir) navegar(ruta(destinoPorRol(s.rol)), { reemplazar: true });
  }, [redirigir, s.rol]);
  if (redirigir) return <CargandoApp texto="Abriendo tu pantalla…" />;
  return (
    <Armazon pagina={pantalla}>
      {!def ? (
        <SinPermiso />
      ) : !permitido ? (
        <SinPermiso permiso={Array.isArray(def.permiso) ? def.permiso.join(' o ') : def.permiso} />
      ) : (
        <Suspense fallback={<CargandoPantalla />}>
          <def.C />
        </Suspense>
      )}
    </Armazon>
  );
}

/** Isla común de todas las páginas Astro: toasts, PWA, guarda de sesión (/yo), armazón y la pantalla. */
export default function Isla({ pantalla }) {
  return (
    <ToastProvider>
      <Pwa />
      <SesionProvider>
        <Pantalla pantalla={pantalla} />
      </SesionProvider>
    </ToastProvider>
  );
}
