// SPDX-License-Identifier: AGPL-3.0-only
import {createRequire} from 'node:module';
import assert from 'node:assert/strict';
const {chromium}=createRequire(import.meta.url)(process.env.PLAYWRIGHT_MODULE);
const browser=await chromium.launch({executablePath:process.env.CHROMIUM,args:['--no-sandbox']});
try {
 const ctx=await browser.newContext({ignoreHTTPSErrors:true});
 await ctx.addCookies([{name:'__Host-obsidian',value:'valid',url:process.argv[2],secure:true,httpOnly:true}]);
 const page=await ctx.newPage();
 // Speed up only the documented retry delays, not htmx or dialog timers.
 await page.addInitScript(()=>{const original=setTimeout;window.setTimeout=(fn,delay,...args)=>original(fn,[2000,5000,15000,30000,60000].includes(delay)?200:delay,...args);});
 const posts=[]; page.on('request',r=>{if(r.method()==='POST') posts.push(r.postData());});
 await page.goto(process.argv[2]);
 if(process.argv[3]==='bounded') {
  await page.getByRole('status').filter({hasText:'paused after five attempts'}).waitFor();
  assert.equal(posts.length,5);
  await page.waitForTimeout(200);
  assert.equal(posts.length,5);
 } else {
  await page.waitForFunction(()=>!document.querySelector('[data-retry]') && document.querySelector('.vault'));
  assert.equal(await page.locator('.vault').count(),1);
  assert.match(await page.locator('.vault h3').textContent(),/Default/);
  assert.equal(posts.length,process.argv[3]==='bootstrap'?2:1);
 }
 for(const post of posts){const fields=new URLSearchParams(post);assert.equal(fields.get('action'),'sync');assert.equal(fields.get('csrf'),'csrf');assert.equal(fields.has('id'),false);}
 console.log(`Real Chromium automatic ${process.argv[3]} retries passed (CSRF, backoff and intent).`);
}finally{await browser.close();}
