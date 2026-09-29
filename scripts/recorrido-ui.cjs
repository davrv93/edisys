// Recorrido de la interfaz contra el API real: 3 roles, 18 vistas, errores de consola y del API,
// scroll horizontal y capturas.
//
// Variables de entorno (todas opcionales):
//   EDISYS_BASE   origen a recorrer (por defecto http://localhost:4700; el dev server: http://localhost:4720)
//   EDISYS_LOGIN  origen del login (por defecto EDISYS_BASE). Con el dev server de Astro, /login/ no se
//                 sirve por el proxy: se entra por el edge (http://localhost:4700) y la cookie, que no
//                 distingue puertos, vale para localhost:4720.
//   EDISYS_OUT    carpeta de capturas (por defecto la del scratchpad de siempre)
//   EDISYS_ANCHOS lista de anchos, p. ej. «390,768,1024,1440»: captura cada vista a cada ancho
//                 (sin ella, cada rol usa su ancho de siempre: admin 1440, el resto 390)
//   EDISYS_360=1  además comprueba scroll horizontal de cada vista a 360 px (sin capturas)
//   PLAYWRIGHT    ruta al módulo playwright; CHROMIUM ruta al ejecutable
// Sale con código 1 si hubo errores de consola, del API o scroll horizontal.
const { chromium } = require(process.env.PLAYWRIGHT || '/Users/david.roncal/Downloads/PjgFactSalud_completo/playwright-runner/node_modules/playwright');
const fs = require('fs');

const OUT = process.env.EDISYS_OUT || '/private/tmp/claude-501/-Users-david-roncal-Downloads-PjgFactSalud-completo/4c23383a-64b3-46c0-9c9d-3200e232fa65/scratchpad/edisys-shots';
const B = (process.env.EDISYS_BASE || 'http://localhost:4700').replace(/\/$/, '');
const LOGIN = (process.env.EDISYS_LOGIN || B).replace(/\/$/, '');
const CHROMIUM = process.env.CHROMIUM || `${process.env.HOME}/Library/Caches/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-mac-arm64/chrome-headless-shell`;
const ANCHOS = (process.env.EDISYS_ANCHOS || '').split(',').map(Number).filter(Boolean);
const CON_360 = process.env.EDISYS_360 === '1';

const casos = [
  { u: 'admin@demo.pe', w: 1440, h: 950, rutas: ['', 'balance', 'recibos', 'unidades', 'unidades?id=1', 'recibos?id=1', 'balance?abrir=egr', 'reservas', 'mantenimiento', 'roles', 'whatsapp', 'chatbot', 'analitica', 'conciliacion', 'configuracion', 'medidores'] },
  { u: 'propietario201@demo.pe', w: 390, h: 844, rutas: ['portal', 'reservas', 'mantenimiento?reportar=1', 'recibos'] },
  { u: 'operario@demo.pe', w: 390, h: 844, rutas: ['medidores'] },
];

const nombre = (u, r, w) => `${u.split('@')[0]}-${r.replace(/[?=]/g, '_') || 'dashboard'}${w ? `-${w}` : ''}.png`;

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const b = await chromium.launch({ executablePath: CHROMIUM });
  let total = 0;
  for (const c of casos) {
    const ctx = await b.newContext({ viewport: { width: c.w, height: c.h }, locale: 'es-PE' });
    const p = await ctx.newPage();
    const errs = [];
    p.on('console', (m) => {
      if (m.type() === 'error') errs.push('console: ' + m.text().slice(0, 160));
    });
    p.on('pageerror', (e) => errs.push('pageerror: ' + e.message.slice(0, 160)));
    p.on('response', (r) => {
      if (r.url().includes('/api/') && r.status() >= 400) errs.push(`api ${r.status()} ${r.request().method()} ${r.url().replace(B, '')}`);
    });
    await p.goto(LOGIN + '/login/');
    await p.fill('input[type=email],input[name=correo]', c.u);
    await p.fill('input[type=password],input[name=clave]', 'Demo2026!');
    await Promise.all([p.waitForURL(/\/app\//, { timeout: 15000 }).catch(() => errs.push('no redirigió a /app')), p.keyboard.press('Enter')]);
    for (const r of c.rutas) {
      const n = errs.length;
      const anchos = ANCHOS.length ? ANCHOS : [null];
      let sc = false;
      let txt = '';
      for (const w of anchos) {
        if (w) await p.setViewportSize({ width: w, height: w < 700 ? 844 : w < 1100 ? 1024 : 900 });
        await p.goto(`${B}/app/${r}`);
        await p.waitForLoadState('networkidle').catch(() => {});
        await p.waitForTimeout(900);
        txt = (await p.locator('main').innerText().catch(() => '')).replace(/\s+/g, ' ').slice(0, 100);
        if (await p.evaluate(() => document.documentElement.scrollWidth > innerWidth)) sc = `SCROLL-H@${w || c.w} `;
        await p.screenshot({ path: `${OUT}/${nombre(c.u, r, w)}`, fullPage: false });
      }
      if (CON_360) {
        await p.setViewportSize({ width: 360, height: 800 });
        await p.goto(`${B}/app/${r}`);
        await p.waitForLoadState('networkidle').catch(() => {});
        await p.waitForTimeout(700);
        const ancho = await p.evaluate(() => [document.documentElement.scrollWidth, innerWidth]);
        if (ancho[0] > ancho[1]) sc = (sc || '') + `SCROLL-H@360(${ancho[0]}px) `;
        await p.setViewportSize({ width: c.w, height: c.h });
      }
      const nuevos = errs.length - n;
      if (sc) total++;
      total += nuevos;
      console.log(`[${c.u.split('@')[0]}] /app/${r} ${sc || ''}errores=${nuevos} | ${txt}`);
      for (const e of errs.slice(n)) console.log('    ' + e);
    }
    await ctx.close();
  }
  await b.close();
  console.log(total ? `TOTAL con problemas: ${total}` : 'OK: 0 errores, sin scroll horizontal');
  process.exit(total ? 1 : 0);
})();
