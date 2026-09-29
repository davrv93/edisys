// QA visual del motor conversacional: simulador del chatbot (reglas → motor,
// sugerencias dinámicas) y pantalla Motor. Sale con código 1 si algo falla.
//   node scripts/qa-motor.cjs
const { chromium } = require(process.env.PLAYWRIGHT || '/Users/david.roncal/Downloads/PjgFactSalud_completo/playwright-runner/node_modules/playwright');
const fs = require('fs');

const OUT = process.env.EDISYS_OUT || '/tmp/edisys-motor-qa';
const B = (process.env.EDISYS_BASE || 'http://localhost:4700').replace(/\/$/, '');
const CHROMIUM = process.env.CHROMIUM || `${process.env.HOME}/Library/Caches/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-mac-arm64/chrome-headless-shell`;

const fallos = [];
const ok = (m) => console.log(`  OK   ${m}`);
const mal = (m) => { console.log(`  FALLA ${m}`); fallos.push(m); };

(async () => {
  fs.mkdirSync(OUT, { recursive: true });
  const b = await chromium.launch({ executablePath: CHROMIUM });
  const ctx = await b.newContext({ viewport: { width: 1440, height: 950 }, locale: 'es-PE' });
  const p = await ctx.newPage();
  p.on('pageerror', (e) => mal(`pageerror: ${e.message.slice(0, 140)}`));
  p.on('response', (r) => {
    if (r.url().includes('/api/') && r.status() >= 400 && !r.url().includes('/motor/consulta')) {
      mal(`api ${r.status()} ${r.request().method()} ${r.url().replace(B, '')}`);
    }
  });

  // login admin
  await p.goto(`${B}/login/`);
  await p.fill('input[type=email],input[name=correo]', 'admin@demo.pe');
  await p.fill('input[type=password],input[name=clave]', 'Demo2026!');
  await Promise.all([p.waitForURL(/\/app\//, { timeout: 15000 }), p.keyboard.press('Enter')]);
  ok('login admin → /app');

  // ---------- 1 · simulador del chatbot ----------
  await p.goto(`${B}/app/chatbot/`);
  await p.waitForLoadState('networkidle');
  const msg = p.locator('#msg');

  // 1a · regla: «cuánto debo» (no pasa por el motor)
  await msg.fill('¿cuánto debo?');
  await msg.press('Enter');
  await p.waitForTimeout(2500);
  const regla = await p.locator('[aria-live=polite].bg-fondo').innerText();
  /intención: saldo/i.test(regla) ? ok('regla «cuánto debo» → intención saldo') : mal(`regla «cuánto debo»: intención inesperada (${(regla.match(/intención: \w+/) || ['?'])[0]})`);

  // 1b · motor: pregunta que ninguna regla entiende
  await msg.fill('Si se corta la luz en medio de una junta de propietarios, ¿los acuerdos tomados en la oscuridad valen igual?');
  await msg.press('Enter');
  try {
    await p.waitForFunction(
      () => document.querySelector('[aria-live=polite].bg-fondo')?.innerText.includes('intención: motor'),
      null, { timeout: 120000 },
    );
    ok('pregunta sin reglas → intención motor (el LLM local respondió)');
  } catch {
    mal('el motor no respondió en 120 s (no apareció intención: motor)');
  }
  await p.screenshot({ path: `${OUT}/chatbot-motor.png`, fullPage: false });

  // 1c · sugerencias dinámicas: el primer chip ya no es el estático «Hola»
  const chips = await p.locator('.overflow-x-auto button').allInnerTexts();
  Array.isArray(chips) && chips.length >= 3 && !chips.includes('Hola')
    ? ok(`sugerencias dinámicas: ${chips.slice(0, 3).join(' | ')}`)
    : mal(`sugerencias dinámicas no aparecieron (${JSON.stringify(chips)})`);
  await p.locator('.overflow-x-auto button').first().click();
  await p.waitForTimeout(6000);
  ok('chip de sugerencia clickeado y respondido');

  // ---------- 2 · pantalla Motor ----------
  await p.goto(`${B}/app/motor/`);
  await p.waitForLoadState('networkidle');
  const texto = await p.locator('main, body').first().innerText();
  /Motor conversacional/i.test(texto) ? ok('pantalla /app/motor/ carga') : mal('la pantalla Motor no cargó');
  /Operativo/.test(texto) ? ok('salud del motor: Operativo') : mal('la salud del motor no se ve (¿motor caído?)');
  /¿cuál es la morosidad del edificio\?/i.test(texto) ? ok('golden del seed listados') : mal('no se ven los golden del seed');

  // 2a · probar el motor desde la pantalla
  await p.locator('input[placeholder*="pregunta para el motor"]').fill('explícame el reparto de medidores');
  await p.getByRole('button', { name: 'Probar', exact: true }).click();
  try {
    await p.waitForFunction(
      () => /puntaje/i.test(document.querySelector('main, body')?.innerText || ''),
      null, { timeout: 120000 },
    );
    ok('«Probar» respondió con puntaje');
  } catch {
    mal('la prueba del motor no respondió en 120 s');
  }
  await p.screenshot({ path: `${OUT}/pantalla-motor.png`, fullPage: true });

  // 2b · ver SQL de un golden
  await p.getByRole('button', { name: 'ver SQL' }).first().click();
  await p.waitForTimeout(400);
  /SELECT/i.test(await p.locator('body').innerText()) ? ok('modal con el SQL del golden') : mal('el modal del SQL no muestra SELECT');

  await b.close();
  console.log(`\nCapturas en ${OUT}/`);
  if (fallos.length) { console.log(`${fallos.length} fallas`); process.exit(1); }
  console.log('QA del motor: todo OK');
})();
