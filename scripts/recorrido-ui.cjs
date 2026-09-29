const { chromium } = require('/Users/david.roncal/Downloads/PjgFactSalud_completo/playwright-runner/node_modules/playwright');
const OUT='/private/tmp/claude-501/-Users-david-roncal-Downloads-PjgFactSalud-completo/4c23383a-64b3-46c0-9c9d-3200e232fa65/scratchpad/edisys-shots';
const B='http://localhost:4700';
const casos=[
 {u:'admin@demo.pe',w:1440,h:950,rutas:['','balance','recibos','unidades','unidades?id=1','recibos?id=1','balance?abrir=egresos','reservas','mantenimiento','roles','whatsapp','chatbot','analitica']},
 {u:'propietario201@demo.pe',w:390,h:844,rutas:['portal','reservas','mantenimiento?reportar=1','recibos']},
 {u:'operario@demo.pe',w:390,h:844,rutas:['medidores']},
];
(async()=>{
 const b=await chromium.launch({executablePath:'/Users/david.roncal/Library/Caches/ms-playwright/chromium_headless_shell-1243/chrome-headless-shell-mac-arm64/chrome-headless-shell'});
 for(const c of casos){
  const ctx=await b.newContext({viewport:{width:c.w,height:c.h}}); const p=await ctx.newPage();
  const errs=[]; p.on('console',m=>{if(m.type()==='error')errs.push('console: '+m.text().slice(0,160))});
  p.on('pageerror',e=>errs.push('pageerror: '+e.message.slice(0,160)));
  p.on('response',r=>{if(r.url().includes('/api/')&&r.status()>=400)errs.push(`api ${r.status()} ${r.request().method()} ${r.url().replace(B,'')}`)});
  await p.goto(B+'/login/'); await p.fill('input[type=email],input[name=correo]',c.u); await p.fill('input[type=password],input[name=clave]','Demo2026!');
  await Promise.all([p.waitForURL(/\/app\//,{timeout:15000}).catch(()=>errs.push('no redirigió a /app')), p.keyboard.press('Enter')]);
  for(const r of c.rutas){
   const n=errs.length; await p.goto(`${B}/app/${r}`); await p.waitForLoadState('networkidle').catch(()=>{}); await p.waitForTimeout(1200);
   const txt=(await p.locator('main').innerText().catch(()=>'')).replace(/\s+/g,' ').slice(0,110);
   const sc=await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
   await p.screenshot({path:`${OUT}/${c.u.split('@')[0]}-${r.replace(/[?=]/g,'_')||'dashboard'}.png`,fullPage:false});
   console.log(`[${c.u.split('@')[0]} ${c.w}] /app/${r} ${sc?'SCROLL-H ':''}errores=${errs.length-n} | ${txt}`);
   for(const e of errs.slice(n)) console.log('    '+e);
  }
  await ctx.close();
 }
 await b.close();
})();
