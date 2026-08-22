const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const outDir = path.join(__dirname, '..', 'ui-audit-screenshots', 'verify');

async function run() {
  const browser = await chromium.launch({ headless: true });

  const mobileUserContext = await browser.newContext({
    viewport: { width: 375, height: 812 },
    isMobile: true,
  });
  const page = await mobileUserContext.newPage();
  await page.goto('http://127.0.0.1:5173/login');
  await page.fill('input[placeholder="you@example.com"]', 'admin');
  await page.fill('input[placeholder="请输入密码"]', 'admin123');
  await page.click('button:has-text("登 录")');
  await page.waitForTimeout(1500);

  // 访问真正路径 /subscribe
  await page.goto('http://127.0.0.1:5173/subscribe');
  await page.waitForTimeout(1500);

  const closeBtn = page.locator('button:has-text("已了解并关闭")');
  if (await closeBtn.isVisible({ timeout: 1000 }).catch(() => false)) {
    await closeBtn.click();
    await page.waitForTimeout(500);
  }

  await page.screenshot({ path: path.join(outDir, '12-real-subscribe-mobile-top.png') });

  // 滚动到客户端卡片
  await page.evaluate(() => window.scrollBy(0, 500));
  await page.waitForTimeout(500);
  await page.screenshot({ path: path.join(outDir, '13-real-subscribe-mobile-clients.png') });

  await browser.close();
  console.log('Saved real subscribe screenshots!');
}

run().catch(console.error);
