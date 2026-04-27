import logging
from pathlib import Path
from time import time

from playwright.sync_api import expect, sync_playwright


def write_report(output_dir: Path, success: bool, steps: list[str]) -> None:
    """写入 output_dir 参数对应用例目录的 Markdown 测试报告。"""
    status = "通过" if success else "失败"
    report = [
        "# Agent 聊天 E2E 测试报告",
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
        "![项目和聊天](screenshots/01-chat-created.png)",
        "",
        "![流式输出](screenshots/02-streaming.png)",
        "",
        "![刷新恢复](screenshots/03-restored.png)",
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
    """运行 Agent 聊天用例，参数提供目标地址和输出目录。"""
    logger.info("打开页面: %s", base_url)
    steps: list[str] = []
    success = False
    server_log_path = logs_dir / "server.log"
    project_name = f"E2E Project {int(time())}"
    project_path = str(Path(__file__).resolve().parents[3])

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1440, "height": 920})
        try:
            if not server_log_path.exists():
                raise RuntimeError(f"未找到当前用例服务日志: {server_log_path}")

            page.goto(base_url, wait_until="networkidle")
            expect(page.get_by_test_id("connection-state")).to_contain_text("已连接", timeout=10000)

            page.get_by_test_id("project-name-input").fill(project_name)
            page.get_by_test_id("project-path-input").fill(project_path)
            page.get_by_test_id("project-save-button").click()
            expect(page.get_by_test_id("project-list")).to_contain_text(project_name, timeout=10000)
            steps.append("创建一个绑定本机目录的 project，并在列表中看到该 project。")

            page.get_by_test_id("chat-new-button").click()
            expect(page.get_by_test_id("chat-tabs")).to_contain_text("聊天 1", timeout=10000)
            page.screenshot(path=screenshots_dir / "01-chat-created.png", full_page=True)
            steps.append("选中 project 后可以创建聊天页。")

            first_prompt = "第一条流式测试"
            page.get_by_test_id("message-input").fill(first_prompt)
            page.keyboard.press("Enter")
            expect(page.get_by_test_id("send-button")).to_contain_text("停止", timeout=10000)
            expect(page.get_by_test_id("message-log")).to_contain_text(first_prompt, timeout=10000)
            expect(page.get_by_test_id("message-log")).to_contain_text("Mock Claude", timeout=20000)
            page.screenshot(path=screenshots_dir / "02-streaming.png", full_page=True)
            steps.append("首次输入 prompt 后后端启动 agent，并把 mock Claude 输出流式返回前端。")

            second_prompt = "第二条长流式测试"
            page.get_by_test_id("message-input").fill(second_prompt)
            page.keyboard.press("Enter")
            expect(page.get_by_test_id("message-log")).to_contain_text(second_prompt, timeout=10000)
            expect(page.get_by_test_id("send-button")).to_contain_text("停止", timeout=10000)
            steps.append("agent 正在输出时直接输入并回车，会停止上一轮并发送新的 prompt。")

            page.reload(wait_until="networkidle")
            expect(page.get_by_test_id("connection-state")).to_contain_text("已连接", timeout=10000)
            expect(page.get_by_test_id("project-list")).to_contain_text(project_name, timeout=10000)
            expect(page.get_by_test_id("message-log")).to_contain_text(second_prompt, timeout=10000)
            expect(page.get_by_test_id("message-log")).to_contain_text("Mock Claude", timeout=20000)
            page.screenshot(path=screenshots_dir / "03-restored.png", full_page=True)
            steps.append("刷新页面后仍能从后端恢复 project、聊天和正在输出的会话。")

            expect(page.get_by_test_id("send-button")).to_contain_text("发送", timeout=30000)
            steps.append("agent 输出完成后，停止按钮重新变回发送按钮。")
            success = True
            return success
        except Exception as err:
            logger.exception("Agent 聊天 E2E 失败: %s", err)
            page.screenshot(path=screenshots_dir / "failed.png", full_page=True)
            steps.append(f"用例失败: {err}")
            return success
        finally:
            _ = logs_dir
            write_report(output_dir, success, steps)
            browser.close()
