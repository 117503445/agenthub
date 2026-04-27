import logging
from pathlib import Path
from time import time

from playwright.sync_api import expect, sync_playwright


def write_report(output_dir: Path, success: bool, steps: list[str]) -> None:
    """写入 output_dir 参数对应用例目录的 Markdown 测试报告。"""
    status = "通过" if success else "失败"
    report = [
        "# WebSocket 和子路径状态同步 E2E 测试报告",
        "",
        f"- 结果: {status}",
        "- 日志: [test.log](logs/test.log)",
        "- 服务日志: [server.log](logs/server.log)",
        "",
        "## 步骤",
        "",
        *[f"- {step}" for step in steps],
        "",
        "## 截图",
        "",
        "![连接成功](screenshots/01-connected.png)",
        "",
        "![状态恢复](screenshots/02-restored.png)",
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
    """运行 WebSocket 状态同步用例，参数提供目标地址和输出目录。"""
    subpath_url = f"{base_url}/console/#/"
    logger.info("打开页面: %s", subpath_url)
    steps: list[str] = []
    success = False
    server_log_path = logs_dir / "server.log"
    project_name = f"WS Project {int(time())}"
    project_path = str(Path(__file__).resolve().parents[3])
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1366, "height": 900})
        try:
            if not server_log_path.exists():
                raise RuntimeError(f"未找到当前用例服务日志: {server_log_path}")

            page.goto(subpath_url, wait_until="networkidle")
            expect(page.get_by_test_id("connection-state")).to_contain_text("已连接", timeout=10000)
            page.screenshot(path=screenshots_dir / "01-connected.png", full_page=True)
            steps.append("页面从 /console/ 子路径打开后，WebSocket 状态变为已连接，并收到后端状态快照。")

            page.get_by_test_id("project-name-input").fill(project_name)
            page.get_by_test_id("project-path-input").fill(project_path)
            page.get_by_test_id("project-save-button").click()
            expect(page.get_by_test_id("project-list")).to_contain_text(project_name, timeout=10000)
            hash_value = page.evaluate("window.location.hash")
            if not str(hash_value).startswith("#/projects/"):
                raise RuntimeError(f"hash 路由未指向 project: {hash_value}")
            page.reload(wait_until="networkidle")
            expect(page.get_by_test_id("project-list")).to_contain_text(project_name, timeout=10000)
            page.screenshot(path=screenshots_dir / "02-restored.png", full_page=True)
            steps.append("创建 project 后 hash 路由指向 project，刷新页面仍从后端内存状态恢复。")
            success = True
            return success
        except Exception as err:
            logger.exception("WebSocket 状态同步 E2E 失败: %s", err)
            page.screenshot(path=screenshots_dir / "failed.png", full_page=True)
            steps.append(f"用例失败: {err}")
            return success
        finally:
            _ = logs_dir
            write_report(output_dir, success, steps)
            browser.close()
