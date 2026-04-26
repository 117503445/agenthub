import logging
from pathlib import Path

from playwright.sync_api import expect, sync_playwright


def run_test(
    base_url: str,
    output_dir: Path,
    screenshots_dir: Path,
    logs_dir: Path,
    logger: logging.Logger,
) -> bool:
    logger.info("打开页面: %s", base_url)
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1366, "height": 900})
        try:
            page.goto(base_url, wait_until="networkidle")
            expect(page.get_by_test_id("connection-state")).to_contain_text("已连接", timeout=10000)
            page.screenshot(path=screenshots_dir / "01-connected.png", full_page=True)

            message = "来自 E2E 的消息"
            page.get_by_test_id("message-input").fill(message)
            page.get_by_test_id("send-button").click()
            expect(page.get_by_test_id("message-log")).to_contain_text(f"后端已收到：{message}", timeout=10000)
            page.screenshot(path=screenshots_dir / "02-echo.png", full_page=True)
            return True
        except Exception as err:
            logger.exception("WebSocket E2E 失败: %s", err)
            page.screenshot(path=screenshots_dir / "failed.png", full_page=True)
            return False
        finally:
            browser.close()
