const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const outDir = path.join(__dirname, '..', 'ui-audit-screenshots', 'after_fix');
if (!fs.existsSync(outDir)) {
  fs.mkdirSync(outDir, { recursive: true });
}

async function run() {
  const browser = await chromium.launch({ headless: true });

  // 1. Mobile (375x812) 验收
  const mobileContext = await browser.newContext({
    viewport: { width: 375, height: 812 },
    isMobile: true,
  });
  const pageMobile = await mobileContext.newPage();

  // 登录管理员
  await pageMobile.goto('http://127.0.0.1:5173/login');
  await pageMobile.fill('input[placeholder="you@example.com"]', 'admin');
  await pageMobile.fill('input[placeholder="请输入密码"]', 'admin123');
  await pageMobile.click('button:has-text("登 录")');
  await pageMobile.waitForTimeout(1500);

  // A. 节点接入移动端（检查顶栏标题是否不再折行）
  await pageMobile.goto('http://127.0.0.1:5173/admin/nodes');
  await pageMobile.waitForTimeout(1000);
  await pageMobile.screenshot({ path: path.join(outDir, '01-nodes-mobile-topbar.png') });
  console.log('Saved 01-nodes-mobile-topbar.png');

  // B. 设置页移动端（检查 6 个 Tab 是否平铺工整无截断）
  await pageMobile.goto('http://127.0.0.1:5173/admin/settings');
  await pageMobile.waitForTimeout(1000);
  await pageMobile.screenshot({ path: path.join(outDir, '02-settings-mobile-tabs.png') });
  console.log('Saved 02-settings-mobile-tabs.png');

  // C. 路由管理移动端（检查 Tab 是否工整）
  await pageMobile.goto('http://127.0.0.1:5173/admin/routing');
  await pageMobile.waitForTimeout(1000);
  await pageMobile.screenshot({ path: path.join(outDir, '03-routing-mobile-tabs.png') });
  console.log('Saved 03-routing-mobile-tabs.png');

  // D. 用户管理移动端（检查操作栏）
  await pageMobile.goto('http://127.0.0.1:5173/admin/users');
  await pageMobile.waitForTimeout(1000);
  await pageMobile.screenshot({ path: path.join(outDir, '04-users-mobile-actions.png') });
  console.log('Saved 04-users-mobile-actions.png');

  // E. 审计日志移动端
  await pageMobile.goto('http://127.0.0.1:5173/admin/audit');
  await pageMobile.waitForTimeout(1000);
  await pageMobile.screenshot({ path: path.join(outDir, '05-audit-mobile-detail.png') });
  console.log('Saved 05-audit-mobile-detail.png');

  // F. 订阅中心移动端
  await pageMobile.goto('http://127.0.0.1:5173/subscribe');
  await pageMobile.waitForTimeout(1000);
  const closeBtn = pageMobile.locator('button:has-text("已了解并关闭")');
  if (await closeBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
    await closeBtn.click();
    await pageMobile.waitForTimeout(400);
  }
  await pageMobile.evaluate(() => window.scrollBy(0, 480));
  await pageMobile.waitForTimeout(400);
  await pageMobile.screenshot({ path: path.join(outDir, '06-subscribe-mobile-clients.png') });
  console.log('Saved 06-subscribe-mobile-clients.png');

  // 2. Desktop (1920x1080) 验收
  const desktopContext = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
  });
  const pageWin = await desktopContext.newPage();
  await pageWin.goto('http://127.0.0.1:5173/login');
  await pageWin.fill('input[placeholder="you@example.com"]', 'admin');
  await pageWin.fill('input[placeholder="请输入密码"]', 'admin123');
  await pageWin.click('button:has-text("登 录")');
  await pageWin.waitForTimeout(1500);

  // G. 仪表盘 1920 宽屏（检查 6 个 KPI 卡片是否单行排布对称）
  await pageWin.goto('http://127.0.0.1:5173/admin/dashboard');
  await pageWin.waitForTimeout(1200);
  await pageWin.screenshot({ path: path.join(outDir, '07-dashboard-win-kpi.png') });
  console.log('Saved 07-dashboard-win-kpi.png');

  // H. 套餐管理页 1920 宽屏（检查表格列宽是否铺满）
  await pageWin.goto('http://127.0.0.1:5173/admin/plans');
  await pageWin.waitForTimeout(1000);
  await pageWin.screenshot({ path: path.join(outDir, '08-plans-win-table.png') });
  console.log('Saved 08-plans-win-table.png');

  await browser.close();
  console.log('All after-fix verification screenshots captured!');
}

run().catch(console.error);
