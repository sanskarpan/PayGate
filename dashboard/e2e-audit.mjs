// Playwright audit script — logs in and visits every page, capturing errors/screenshots
import { chromium } from '@playwright/test';
import { writeFileSync, mkdirSync } from 'fs';

const BASE = 'http://localhost:3001';
const SS_DIR = '/tmp/paygate-screenshots';
mkdirSync(SS_DIR, { recursive: true });

const PAGES = [
  { path: '/', name: 'login' },
  { path: '/orders', name: 'orders' },
  { path: '/api-keys', name: 'api-keys' },
  { path: '/webhooks', name: 'webhooks' },
  { path: '/settlements', name: 'settlements' },
  { path: '/recon', name: 'recon' },
  { path: '/risk', name: 'risk' },
  { path: '/audit', name: 'audit' },
  { path: '/team', name: 'team' },
  { path: '/refunds', name: 'refunds' },
];

const results = {};

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await context.newPage();

// Capture all console errors and network failures globally
const allErrors = [];
page.on('console', msg => {
  if (msg.type() === 'error') allErrors.push({ type: 'console.error', text: msg.text() });
  if (msg.type() === 'warning') allErrors.push({ type: 'console.warn', text: msg.text() });
});
page.on('pageerror', err => allErrors.push({ type: 'pageerror', text: err.message }));
page.on('response', resp => {
  const url = resp.url();
  const status = resp.status();
  if (status >= 400 && !url.includes('_next') && !url.includes('_rsc') && !url.includes('favicon')) {
    allErrors.push({ type: `http_${status}`, url, status });
  }
});
page.on('requestfailed', req => {
  const failure = req.failure()?.errorText || '';
  if (!req.url().includes('_next')) {
    allErrors.push({ type: 'request_failed', url: req.url(), failure });
  }
});

// ── Step 1: Login ─────────────────────────────────────────────────────────────
console.log('→ Logging in...');
await page.goto(BASE + '/');
await page.waitForLoadState('networkidle');
const MERCHANT_ID = process.env.AUDIT_MERCHANT_ID || '';
const EMAIL      = process.env.AUDIT_EMAIL      || '';
const PASSWORD   = process.env.AUDIT_PASSWORD   || '';
if (!MERCHANT_ID || !EMAIL || !PASSWORD) {
  console.error('Set AUDIT_MERCHANT_ID, AUDIT_EMAIL, AUDIT_PASSWORD env vars before running.');
  process.exit(1);
}
await page.fill('input[name="merchant_id"]', MERCHANT_ID);
await page.fill('input[name="email"]', EMAIL);
await page.fill('input[name="password"]', PASSWORD);
await page.click('button[type="submit"]');
await page.waitForLoadState('networkidle');
const afterLogin = page.url();
console.log('  After login URL:', afterLogin);
await page.screenshot({ path: `${SS_DIR}/00-login.png`, fullPage: true });

if (afterLogin.includes('localhost:3001/')) {
  console.log('  ✓ Login successful');
} else {
  console.log('  ✗ Login may have failed, URL:', afterLogin);
}

// ── Step 2: Grab a real order ID and payment ID from the API ──────────────────
let orderId = null, paymentId = null, webhookId = null, settlementId = null;

// Check cookies to forward
const cookies = await context.cookies();
const cookieHeader = cookies.map(c => `${c.name}=${c.value}`).join('; ');

// Try direct API (server-side)
try {
  const r = await fetch('http://localhost:8090/v1/orders', {
    headers: { Cookie: cookieHeader }
  });
  if (r.ok) {
    const d = await r.json();
    orderId = d.items?.[0]?.id;
    console.log('  Order ID (direct):', orderId);
  }
} catch(e) {}

// Get payment for this order
if (orderId) {
  try {
    const r = await fetch(`http://localhost:8090/v1/payments?order_id=${orderId}`, {
      headers: { Cookie: cookieHeader }
    });
    if (r.ok) {
      const d = await r.json();
      paymentId = d.items?.[0]?.id || d[0]?.id;
      console.log('  Payment ID:', paymentId);
    }
  } catch(e) {}
}

// Get webhook subscription
try {
  const r = await fetch('http://localhost:8090/v1/webhooks', {
    headers: { Cookie: cookieHeader }
  });
  if (r.ok) {
    const d = await r.json();
    webhookId = d.items?.[0]?.id || d[0]?.id;
    console.log('  Webhook ID:', webhookId);
  }
} catch(e) {}

// Get settlement
try {
  const r = await fetch('http://localhost:8090/v1/settlements', {
    headers: { Cookie: cookieHeader }
  });
  if (r.ok) {
    const d = await r.json();
    settlementId = d.items?.[0]?.id || d[0]?.id;
    console.log('  Settlement ID:', settlementId);
  }
} catch(e) {}

// ── Step 3: Add dynamic pages ─────────────────────────────────────────────────
if (orderId) PAGES.push({ path: `/orders/${orderId}`, name: 'order-detail' });
if (paymentId) PAGES.push({ path: `/payments/${paymentId}`, name: 'payment-detail' });
if (webhookId) PAGES.push({ path: `/webhooks/${webhookId}`, name: 'webhook-detail' });
if (settlementId) PAGES.push({ path: `/settlements/${settlementId}`, name: 'settlement-detail' });

// ── Step 4: Visit every page ──────────────────────────────────────────────────
for (const { path, name } of PAGES) {
  if (name === 'login') continue;
  console.log(`\n→ Visiting ${path}`);
  const pageErrors = [];
  const networkFails = [];

  const consoleListener = msg => {
    if (msg.type() === 'error') pageErrors.push({ type: 'console', text: msg.text() });
  };
  const failListener = req => {
    if (!req.url().includes('_next') && !req.url().includes('favicon')) {
      networkFails.push({ url: req.url(), failure: req.failure()?.errorText });
    }
  };
  const respListener = resp => {
    const url = resp.url();
    const status = resp.status();
    if (status >= 400 && !url.includes('_next') && !url.includes('_rsc') && !url.includes('favicon')) {
      networkFails.push({ url, status });
    }
  };
  page.on('console', consoleListener);
  page.on('requestfailed', failListener);
  page.on('response', respListener);

  let httpStatus = 0;
  try {
    const resp = await page.goto(BASE + path, { waitUntil: 'networkidle', timeout: 15000 });
    httpStatus = resp?.status() || 0;
  } catch (e) {
    pageErrors.push({ type: 'navigation', text: e.message });
  }

  // Check overflow
  let overflowElements = [];
  try {
    overflowElements = await page.evaluate(() => {
      const els = document.querySelectorAll('*');
      const bad = [];
      for (const el of els) {
        if (el.scrollWidth > el.clientWidth + 5 && el.tagName !== 'BODY' && el.tagName !== 'HTML') {
          const style = window.getComputedStyle(el);
          if (style.overflow !== 'auto' && style.overflow !== 'scroll' &&
              style.overflowX !== 'auto' && style.overflowX !== 'scroll') {
            bad.push(`<${el.tagName.toLowerCase()} class="${el.className}"> overflows by ${el.scrollWidth - el.clientWidth}px`);
          }
        }
      }
      return bad.slice(0, 10);
    });
  } catch (e) { /* page may have navigated */ }

  // Get page text content for empty state check
  let bodyText = '';
  try {
    bodyText = await page.evaluate(() => document.body.innerText.substring(0, 500));
  } catch (e) { /* page may have navigated */ }

  try {
    await page.screenshot({ path: `${SS_DIR}/${String(PAGES.indexOf(PAGES.find(p=>p.name===name))).padStart(2,'0')}-${name}.png`, fullPage: true });
  } catch (e) { /* page may have navigated */ }

  page.off('console', consoleListener);
  page.off('requestfailed', failListener);
  page.off('response', respListener);

  results[name] = { httpStatus, pageErrors, networkFails, overflowElements, bodyText: bodyText.trim() };

  console.log(`  HTTP: ${httpStatus}`);
  if (pageErrors.length) console.log(`  Console errors: ${pageErrors.map(e=>e.text).join(' | ')}`);
  if (networkFails.length) console.log(`  Network fails: ${JSON.stringify(networkFails)}`);
  if (overflowElements.length) console.log(`  Overflow: ${overflowElements.join(', ')}`);
}

await browser.close();

// ── Final report ──────────────────────────────────────────────────────────────
console.log('\n\n═══════════════ FINAL AUDIT REPORT ═══════════════');
let totalIssues = 0;
for (const [name, r] of Object.entries(results)) {
  const issues = [...r.pageErrors, ...r.networkFails, ...r.overflowElements.map(o=>({type:'overflow',text:o}))];
  if (issues.length || r.httpStatus >= 400) {
    console.log(`\n❌ ${name} (HTTP ${r.httpStatus})`);
    for (const i of issues) console.log(`   • ${JSON.stringify(i)}`);
    totalIssues += issues.length;
  } else {
    console.log(`✓  ${name} (HTTP ${r.httpStatus})`);
  }
}
console.log(`\nTotal issues: ${totalIssues}`);
writeFileSync('/tmp/paygate-audit.json', JSON.stringify(results, null, 2));
console.log('Full results saved to /tmp/paygate-audit.json');
console.log('Screenshots saved to', SS_DIR);
