const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const outDir = path.join(__dirname, '..', 'ui-audit-screenshots', 'verify');
if (!fs.existsSync(outDir)) {
  fs.mkdirSync(outDir, { recursive: true });
}

async function run() {
  const browser = await chromium.launch({ headless: true });

  // 1. 管理员登录并在 Win / Mobile 下截取相关页面
  const adminContextWin = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
    colorScheme: 'light',
  });
  const pageWin = await adminContextWin.newPage();

  console.log('Navigating to login...');
  await pageWin.goto('http://127.0.0.1:5173/login');
  await pageWin.waitForSelector('input[placeholder="you@example.com"]', { timeout: 10000 });
  await pageWin.fill('input[placeholder="you@example.com"]', 'admin');
  await pageWin.fill('input[placeholder="请输入密码"]', 'admin123');
  await pageWin.click('button:has-text("登 录")');

  await pageWin.waitForTimeout(2000);
  console.log('Admin logged in! Current URL:', pageWin.url());

  // A. P2-1: 仪表盘 Win 1920 浅色模式
  await pageWin.goto('http://127.0.0.1:5173/admin/dashboard');
  await pageWin.waitForTimeout(1500);
  await pageWin.screenshot({ path: path.join(outDir, '01-dashboard-win-light.png') });
  console.log('Saved 01-dashboard-win-light.png');

  // A2. 仪表盘 Win 1920 深色模式
  await pageWin.evaluate(() => {
    document.documentElement.classList.add('dark');
    localStorage.setItem('theme', 'dark');
  });
  await pageWin.reload();
  await pageWin.waitForTimeout(1500);
  await pageWin.screenshot({ path: path.join(outDir, '02-dashboard-win-dark.png') });
  console.log('Saved 02-dashboard-win-dark.png');

  // 恢复浅色
  await pageWin.evaluate(() => {
    document.documentElement.classList.remove('dark');
    localStorage.setItem('theme', 'light');
  });
  await pageWin.reload();
  await pageWin.waitForTimeout(1000);

  // B. P2-2: 套餐管理页 Win 1920 浅色
  await pageWin.goto('http://127.0.0.1:5173/admin/plans');
  await pageWin.waitForTimeout(1000);
  await pageWin.screenshot({ path: path.join(outDir, '03-plans-win-light.png') });
  console.log('Saved 03-plans-win-light.png');

  // C. Mobile Context (375x812)
  const mobileContext = await browser.newContext({
    viewport: { width: 375, height: 812 },
    isMobile: true,
  });
  const pageMobile = await mobileContext.newPage();
  await pageMobile.goto('http://127.0.0.1:5173/login');
  await pageMobile.fill('input[placeholder="you@example.com"]', 'admin');
  await pageMobile.fill('input[placeholder="请输入密码"]', 'admin123');
  await pageMobile.click('button:has-text("登 录")');
  await pageMobile.waitForTimeout(2000);

  // P1-1: 用户管理 Mobile
  await pageMobile.goto('http://127.0.0.1:5173/admin/users');
  await pageMobile.waitForTimeout(1500);
  await pageMobile.screenshot({ path: path.join(outDir, '04-users-mobile-375.png') });
  console.log('Saved 04-users-mobile-375.png');

  // P1-2: 设置页 Mobile
  await pageMobile.goto('http://127.0.0.1:5173/admin/settings');
  await pageMobile.waitForTimeout(1500);
  await pageMobile.screenshot({ path: path.join(outDir, '05-settings-mobile-375.png') });
  console.log('Saved 05-settings-mobile-375.png');

  // P1-3: 邀请码页 Mobile
  await pageMobile.goto('http://127.0.0.1:5173/admin/invitations');
  await pageMobile.waitForTimeout(1500);
  await pageMobile.screenshot({ path: path.join(outDir, '06-invitations-mobile-375.png') });
  console.log('Saved 06-invitations-mobile-375.png');

  // P1-4: 审计日志页 Mobile
  await pageMobile.goto('http://127.0.0.1:5173/admin/audit');
  await pageMobile.waitForTimeout(1500);
  await pageMobile.screenshot({ path: path.join(outDir, '07-audit-mobile-375.png') });
  console.log('Saved 07-audit-mobile-375.png');

  // D. 用户端订阅中心与商城 P1-5 / P2-3
  await pageMobile.goto('http://127.0.0.1:5173/client/subscribe');
  await pageMobile.waitForTimeout(1500);
  await pageMobile.screenshot({ path: path.join(outDir, '08-client-subscribe-mobile-375.png') });
  console.log('Saved 08-client-subscribe-mobile-375.png');

  await pageMobile.goto('http://127.0.0.1:5173/client/shop');
  await pageMobile.waitForTimeout(1500);
  await pageMobile.screenshot({ path: path.join(outDir, '09-client-shop-mobile-375.png') });
  console.log('Saved 09-client-shop-mobile-375.png');

  await browser.close();
  console.log('All verification screenshots captured successfully!');
}

run().catch(err => {
  console.error('Error running verification:', err);
  process.exit(1);
});
