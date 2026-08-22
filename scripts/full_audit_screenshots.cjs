const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const outDir = path.join(__dirname, '..', 'ui-audit-screenshots', 'all');
if (!fs.existsSync(outDir)) {
  fs.mkdirSync(outDir, { recursive: true });
}

const adminPages = [
  { name: 'dashboard', path: '/admin/dashboard' },
  { name: 'servers', path: '/admin/servers' },
  { name: 'nodes', path: '/admin/nodes' },
  { name: 'routing', path: '/admin/routing' },
  { name: 'plans', path: '/admin/plans' },
  { name: 'certs', path: '/admin/certs' },
  { name: 'permission-groups', path: '/admin/permission-groups' },
  { name: 'users', path: '/admin/users' },
  { name: 'gift-cards', path: '/admin/gift-cards' },
  { name: 'orders', path: '/admin/orders' },
  { name: 'invitations', path: '/admin/invitations' },
  { name: 'audit', path: '/admin/audit' },
  { name: 'notices', path: '/admin/notices' },
  { name: 'settings', path: '/admin/settings' },
  { name: 'design-demo', path: '/admin/design-demo' },
];

const clientPages = [
  { name: 'dashboard', path: '/dashboard' },
  { name: 'shop', path: '/shop' },
  { name: 'subscribe', path: '/subscribe' },
  { name: 'account', path: '/account' },
];

const authPages = [
  { name: 'login', path: '/login' },
  { name: 'register', path: '/register' },
  { name: 'forgot', path: '/forgot' },
];

async function dismissModals(page) {
  try {
    const closeBtns = page.locator('button:has-text("已了解并关闭"), button:has-text("我知道了"), .el-dialog__headerbtn');
    const count = await closeBtns.count();
    for (let i = 0; i < count; i++) {
      const btn = closeBtns.nth(i);
      if (await btn.isVisible().catch(() => false)) {
        await btn.click().catch(() => {});
        await page.waitForTimeout(300);
      }
    }
  } catch {}
}

async function setTheme(page, theme) {
  await page.evaluate((t) => {
    if (t === 'dark') {
      document.documentElement.classList.add('dark');
      document.documentElement.setAttribute('data-theme', 'dark');
      localStorage.setItem('x-theme', 'dark');
      localStorage.setItem('theme', 'dark');
    } else {
      document.documentElement.classList.remove('dark');
      document.documentElement.setAttribute('data-theme', 'light');
      localStorage.setItem('x-theme', 'light');
      localStorage.setItem('theme', 'light');
    }
  }, theme);
  await page.waitForTimeout(300);
}

async function captureAll() {
  const browser = await chromium.launch({ headless: true });
  console.log('Starting full UI audit screenshot capture...');

  const viewports = [
    { label: 'win', width: 1920, height: 1080, isMobile: false },
    { label: 'mobile', width: 375, height: 812, isMobile: true },
  ];

  const themes = ['light', 'dark'];

  for (const vp of viewports) {
    for (const theme of themes) {
      console.log(`\n=== Processing Viewport: ${vp.label} (${vp.width}x${vp.height}) | Theme: ${theme} ===`);
      const context = await browser.newContext({
        viewport: { width: vp.width, height: vp.height },
        isMobile: vp.isMobile,
      });
      const page = await context.newPage();

      // 1. 认证页（未登录）
      for (const ap of authPages) {
        try {
          await page.goto(`http://127.0.0.1:5173${ap.path}`, { waitUntil: 'domcontentloaded' });
          await setTheme(page, theme);
          await page.waitForTimeout(600);
          const filename = `auth-${ap.name}-${vp.label}-${theme}.png`;
          await page.screenshot({ path: path.join(outDir, filename) });
          console.log(`✓ Saved ${filename}`);
        } catch (e) {
          console.error(`✗ Error on auth ${ap.name}:`, e.message);
        }
      }

      // 2. 登录管理员
      try {
        await page.goto('http://127.0.0.1:5173/login', { waitUntil: 'domcontentloaded' });
        await page.fill('input[placeholder="you@example.com"]', 'admin');
        await page.fill('input[placeholder="请输入密码"]', 'admin123');
        await page.click('button:has-text("登 录")');
        await page.waitForTimeout(1500);
      } catch (e) {
        console.error('Login error:', e);
      }

      // 3. 管理端所有页面
      for (const p of adminPages) {
        try {
          await page.goto(`http://127.0.0.1:5173${p.path}`, { waitUntil: 'domcontentloaded' });
          await setTheme(page, theme);
          await dismissModals(page);
          await page.waitForTimeout(800);
          const filename = `admin-${p.name}-${vp.label}-${theme}.png`;
          await page.screenshot({ path: path.join(outDir, filename) });
          console.log(`✓ Saved ${filename}`);
        } catch (e) {
          console.error(`✗ Error on admin ${p.name}:`, e.message);
        }
      }

      // 4. 用户端所有页面
      for (const p of clientPages) {
        try {
          await page.goto(`http://127.0.0.1:5173${p.path}`, { waitUntil: 'domcontentloaded' });
          await setTheme(page, theme);
          await dismissModals(page);
          await page.waitForTimeout(800);
          const filename = `client-${p.name}-${vp.label}-${theme}.png`;
          await page.screenshot({ path: path.join(outDir, filename) });
          console.log(`✓ Saved ${filename}`);
        } catch (e) {
          console.error(`✗ Error on client ${p.name}:`, e.message);
        }
      }

      await context.close();
    }
  }

  await browser.close();
  console.log('\n=============================================');
  console.log(`All UI screenshots saved to: ${outDir}`);
  console.log('=============================================');
}

captureAll().catch(err => {
  console.error('Audit failed:', err);
  process.exit(1);
});
