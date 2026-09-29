import { useEffect, useState } from 'react';
import { api, subir } from '../../lib/api.js';
import { useCarga } from '../../lib/useCarga.js';
import { formatearFecha } from '../../lib/fechas.js';
import { Seccion } from '../../layout/Encabezado.jsx';
import { Boton, Campo, ErrorCarga, Esqueleto, Insignia, SubirArchivo, useToast } from '../../ui/index.js';

const MODOS = [
  { valor: 'off', etiqueta: 'Apagado' },
  { valor: 'simulado', etiqueta: 'Simulado (no sale nada a SUNAT)' },
  { valor: 'beta', etiqueta: 'Beta de SUNAT (pruebas, con credenciales)' },
  { valor: 'produccion', etiqueta: 'Producción (deshabilitado en esta entrega)', deshabilitado: true },
];
const AFECTACIONES = [
  { valor: 'inafecto', etiqueta: 'Inafecto' },
  { valor: 'exonerado', etiqueta: 'Exonerado' },
  { valor: 'gravado', etiqueta: 'Gravado (IGV 18 %)' },
];
const TIPOS_LINEA = [
  ['cuota', 'Cuota de mantenimiento'],
  ['agua', 'Agua (consumo propio)'],
  ['agua_comun', 'Agua de áreas comunes'],
  ['reserva', 'Reservas de áreas'],
  ['multa', 'Multas'],
  ['ajuste', 'Ajustes'],
];

/** RUC peruano: 11 dígitos con dígito verificador (el API valida igual). */
export function rucValido(ruc) {
  const r = String(ruc || '').trim();
  if (!/^(10|15|16|17|20)\d{9}$/.test(r)) return false;
  const pesos = [5, 4, 3, 2, 7, 6, 5, 4, 3, 2];
  const s = pesos.reduce((a, p, i) => a + p * Number(r[i]), 0);
  let d = 11 - (s % 11);
  if (d === 10) d = 0;
  if (d === 11) d = 1;
  return d === Number(r[10]);
}

/** Configuración › Facturación electrónica: emisor, series, modo, OSE y certificado (.pfx). Las claves nunca se muestran. */
export default function FacturacionElectronica({ eid }) {
  const { toast } = useToast();
  const cfg = useCarga(() => api.get(`/edificios/${eid}/facturacion/config`), [eid]);
  const [f, setF] = useState(null);
  const [errores, setErrores] = useState({});
  const [guardando, setGuardando] = useState(false);
  const [pfx, setPfx] = useState(null);
  const [clavePfx, setClavePfx] = useState('');
  const [subiendo, setSubiendo] = useState(false);

  useEffect(() => {
    if (cfg.datos) setF({ ...cfg.datos, ose_clave: '' });
  }, [cfg.datos]);

  if (cfg.error) return <ErrorCarga error={cfg.error} onReintentar={cfg.recargar} />;
  if (!f) return <Esqueleto className="h-64 w-full" />;
  const cambiar = (k) => (v) => setF((x) => ({ ...x, [k]: v }));

  const guardar = async () => {
    const e = {};
    if (f.modo !== 'off' && !rucValido(f.ruc)) e.ruc = 'RUC de 11 dígitos válido.';
    if (f.modo !== 'off' && !String(f.razon_social || '').trim()) e.razon_social = 'Obligatoria.';
    setErrores(e);
    if (Object.keys(e).length) return;
    setGuardando(true);
    try {
      const cuerpo = { ...f };
      if (!cuerpo.ose_clave) delete cuerpo.ose_clave;
      await api.put(`/edificios/${eid}/facturacion/config`, cuerpo);
      toast('Facturación electrónica guardada.', { tipo: 'exito' });
      cfg.recargar();
    } catch (err) {
      setErrores({ ...err.campos, general: err.message });
    } finally {
      setGuardando(false);
    }
  };

  const subirPfx = async () => {
    const form = new FormData();
    form.set('certificado', pfx);
    form.set('clave', clavePfx);
    setSubiendo(true);
    try {
      const r = await subir(`/edificios/${eid}/facturacion/certificado`, form);
      toast(`Certificado de ${r.titular} cargado (vence ${formatearFecha(r.certificado_vence)}).`, { tipo: 'exito' });
      setPfx(null);
      setClavePfx('');
      cfg.recargar();
    } catch (err) {
      setErrores({ clave_pfx: err.message });
    } finally {
      setSubiendo(false);
    }
  };

  return (
    <div className="flex flex-col gap-4 lg:gap-6">
      <Seccion titulo="Emisor y modo" extra={<Insignia tono={f.modo === 'off' ? 'neutro' : f.modo === 'simulado' ? 'aviso' : 'acento'} texto={MODOS.find((m) => m.valor === f.modo)?.etiqueta || f.modo} />}>
        <p className="text-sm text-texto-apoyo">{cfg.datos.aviso}</p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Campo etiqueta="Modo" tipo="select" opciones={MODOS} valor={f.modo} onCambio={cambiar('modo')} error={errores.modo} />
          <Campo etiqueta="RUC del emisor" tipo="numero" valor={f.ruc} onCambio={cambiar('ruc')} error={errores.ruc} />
          <Campo etiqueta="Razón social" valor={f.razon_social} onCambio={cambiar('razon_social')} error={errores.razon_social} className="sm:col-span-2" />
          <Campo etiqueta="Dirección fiscal" valor={f.direccion} onCambio={cambiar('direccion')} className="sm:col-span-2" />
          <Campo etiqueta="Ubigeo" valor={f.ubigeo} onCambio={cambiar('ubigeo')} ayuda="6 dígitos, p. ej. 150122 (Miraflores)." />
          <div className="grid grid-cols-2 gap-4">
            <Campo etiqueta="Serie de boletas" valor={f.serie_boleta} onCambio={cambiar('serie_boleta')} error={errores.serie_boleta} />
            <Campo etiqueta="Serie de facturas" valor={f.serie_factura} onCambio={cambiar('serie_factura')} error={errores.serie_factura} />
          </div>
        </div>
      </Seccion>
      <Seccion titulo="Afectación al IGV por concepto">
        <p className="text-sm text-texto-apoyo">La cuota de mantenimiento suele estar inafecta; las reservas de áreas, gravadas.</p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {TIPOS_LINEA.map(([k, nombre]) => (
            <Campo key={k} etiqueta={nombre} tipo="select" opciones={AFECTACIONES} valor={f.afectacion?.[k] || 'inafecto'} onCambio={(v) => setF((x) => ({ ...x, afectacion: { ...x.afectacion, [k]: v } }))} />
          ))}
        </div>
      </Seccion>
      <Seccion titulo="Proveedor OSE / SUNAT beta">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Campo etiqueta="URL del servicio" valor={f.ose_url} onCambio={cambiar('ose_url')} ayuda="Vacío en beta: se usa el servicio de pruebas de SUNAT." className="sm:col-span-2" />
          <Campo etiqueta="Usuario (RUC + usuario SOL)" valor={f.ose_usuario} onCambio={cambiar('ose_usuario')} />
          <Campo etiqueta="Clave" tipo="clave" valor={f.ose_clave} onCambio={cambiar('ose_clave')} ayuda={cfg.datos.tiene_ose_clave ? 'Hay una clave guardada: escribe solo para cambiarla.' : 'No hay clave guardada.'} />
        </div>
        {errores.general && <p className="text-sm text-alerta">{errores.general}</p>}
        <Boton cargando={guardando} onClick={guardar} className="self-start">
          Guardar
        </Boton>
      </Seccion>
      <Seccion titulo="Certificado digital">
        <p className="text-sm">
          {cfg.datos.tiene_certificado ? `Certificado cargado, vence el ${formatearFecha(cfg.datos.certificado_vence)}.` : 'Sin certificado: en modo simulado se firma con un certificado de prueba de EDISYS.'}
        </p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <SubirArchivo etiqueta="Archivo .pfx o .p12" ayuda="Se guarda cifrado en el almacén privado; no se vuelve a mostrar." aceptar=".pfx,.p12" tipos={['application/x-pkcs12']} extensiones={['.pfx', '.p12']} archivo={pfx} onArchivo={setPfx} />
          <Campo etiqueta="Clave del certificado" tipo="clave" valor={clavePfx} onCambio={setClavePfx} error={errores.clave_pfx} />
        </div>
        <Boton variante="secundario" icono="subir" disabled={!pfx} cargando={subiendo} onClick={subirPfx} className="self-start">
          Cargar certificado
        </Boton>
      </Seccion>
    </div>
  );
}
