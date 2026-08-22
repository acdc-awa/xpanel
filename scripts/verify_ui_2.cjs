const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const outDir = path.join(__dirname, '..', 'ui-audit-screenshots', 'verify');

async function run() {
  const browser = await chromium.launch({ headless: true });

  // 1. 用户端订阅中心 Mobile
  const mobileUserContext = await browser.newContext({
    viewport: { width: 375, height: 812 },
    isMobile: true,
  });
  const pageUserMobile = await mobileUserContext.newPage();
  await pageUserMobile.goto('http://127.0.0.1:5173/login');
  await pageUserMobile.fill('input[placeholder="you@example.com"]', 'admin');
  await pageUserMobile.fill('input[placeholder="请输入密码"]', 'admin123');
  await pageUserMobile.click('button:has-text("登 录")');
  await pageUserMobile.waitForTimeout(1500);

  await pageUserMobile.goto('http://127.0.0.1:5173/client/subscribe');
  await pageUserMobile.waitForTimeout(1000);

  // 如果有公告弹窗，点击“已了解并关闭”
  const closeBtn = pageUserMobile.locator('button:has-text("已了解并关闭")');
  if (await closeBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
    await closeBtn.click();
    await pageUserMobile.waitForTimeout(500);
  }

  await pageUserMobile.screenshot({ path: path.join(outDir, '08-client-subscribe-mobile-noclose.png'), fullPage: false });
  // 滚动到推荐客户端区域并截图
  await pageUserMobile.evaluate(() => window.scrollBy(0, 400));
  await pageUserMobile.waitForTimeout(500);
  await pageUserMobile.screenshot({ path: path.join(outDir, '08-client-subscribe-mobile-scrolled.png'), fullPage: false });
  console.log('Saved 08-client-subscribe-mobile screenshots');

  // 2. 深色模式验证 (通过点击顶部主题切换按钮或设置 html.dark)
  const adminDarkContext = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
  });
  const pageDark = await adminDarkContext.newPage();
  await pageDark.goto('http://127.0.0.1:5173/login');
  await pageDark.fill('input[placeholder="you@example.com"]', 'admin');
  await pageDark.fill('input[placeholder="请输入密码"]', 'admin123');
  await pageDark.click('button:has-text("登 录")');
  await pageDark.waitForTimeout(1500);

  await pageDark.goto('http://127.0.0.1:5173/admin/dashboard');
  await pageDark.waitForTimeout(1000);

  // 点击顶部月亮/太阳图标切换主题
  const themeToggle = pageDark.locator('.x-topbar button, .x-topbar .el-icon-moon, .x-topbar [title*="主题"], header button, .topbar-actions button').first();
  // 或者直接通过 JS 切换 class
  await pageDark.evaluate(() => {
    document.documentElement.classList.add('dark');
    document.documentElement.setAttribute('data-theme', 'dark');
  });
  await pageDark.waitForTimeout(500);
  await pageDark.screenshot({ path: path.join(outDir, '10-dashboard-real-dark.png') });

  // 审计日志多行长数据模拟
  await pageDark.goto('http://127.0.0.1:5173/admin/audit');
  await pageDark.waitForTimeout(1000);
  await pageDark.screenshot({ path: path.join(outDir, '11-audit-desktop-dark.png') });

  await browser.close();
  console.log('Done additional screenshots');
}

run().catch(console.error);
