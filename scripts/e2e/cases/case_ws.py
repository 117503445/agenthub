import logging
from pathlib import Path

from playwright.sync_api import expect, sync_playwright


def write_report(output_dir: Path, success: bool, steps: list[str]) -> None:
    """写入 output_dir 参数对应用例目录的 Markdown 测试报告。"""
    status = "通过" if success else "失败"
    report = [
        "# WebSocket E2E 测试报告",
        "",
        f"- 结果: {status}",
        "- 日志: [test.log](logs/test.log)",
        "",
        "## 步骤",
        "",
        *[f"- {step}" for step in steps],
        "",
        "## 截图",
        "",
        "![连接成功](screenshots/01-connected.png)",
        "",
        "![消息回环](screenshots/02-echo.png)",
    ]
    if not success:
        report.extend(["", "![失败现场](screenshots/failed.png)"])
    (output_dir / "report.md").write_text("\n".join(report) + "\n", encoding="utf-8")


def run_test(
    base_url: str,
    output_dir: Path,
    screenshots_dir: Path,
    logs_dir: Path,
    logger: logging.Logger,
) -> bool:
    """运行 WebSocket 回环用例，参数提供目标地址和输出目录。"""
    logger.info("打开页面: %s", base_url)
    steps: list[str] = []
    success = False
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1366, "height": 900})
        try:
            page.goto(base_url, wait_until="networkidle")
            expect(page.get_by_test_id("connection-state")).to_contain_text("已连接", timeout=10000)
            page.screenshot(path=screenshots_dir / "01-connected.png", full_page=True)
            steps.append("页面打开后 WebSocket 状态变为已连接。")

            message = "来自 E2E 的消息"
            page.get_by_test_id("message-input").fill(message)
            page.get_by_test_id("send-button").click()
            expect(page.get_by_test_id("message-log")).to_contain_text(f"后端已收到：{message}", timeout=10000)
            page.screenshot(path=screenshots_dir / "02-echo.png", full_page=True)
            steps.append("浏览器发送消息后，服务端返回同内容回环。")
            success = True
            return success
        except Exception as err:
            logger.exception("WebSocket E2E 失败: %s", err)
            page.screenshot(path=screenshots_dir / "failed.png", full_page=True)
            steps.append(f"用例失败: {err}")
            return success
        finally:
            _ = logs_dir
            write_report(output_dir, success, steps)
            browser.close()
