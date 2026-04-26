#!/usr/bin/env python3
"""E2E 运行器。"""

import argparse
import importlib.util
import logging
import os
import shutil
import socket
import subprocess
import sys
import time
import urllib.request
from datetime import datetime
from pathlib import Path

ROOT_DIR = Path(__file__).resolve().parents[2]
CASES_DIR = Path(__file__).resolve().parent / "cases"
OUTPUT_BASE_DIR = ROOT_DIR / "data" / "e2e"

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")
logger = logging.getLogger(__name__)


def find_free_port() -> int:
    """查找可用于本次 E2E 服务的本地端口。"""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def wait_until_ready(base_url: str) -> None:
    """等待 base_url 参数对应的服务健康检查通过。"""
    deadline = time.time() + 20
    last_error: Exception | None = None
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(f"{base_url}/healthz", timeout=1) as response:
                if response.status == 200:
                    return
        except Exception as err:
            last_error = err
            time.sleep(0.25)
    raise RuntimeError(f"服务未就绪: {last_error}")


def discover_cases() -> list[str]:
    """发现 cases 目录下所有可运行的 E2E 用例。"""
    return sorted(file.stem for file in CASES_DIR.glob("case_*.py"))


def start_server(server_cmd: str, port: int, logs_dir: Path) -> subprocess.Popen[str]:
    """使用 server_cmd 参数启动被测服务，并把日志写入 logs_dir 参数。"""
    env = os.environ.copy()
    env["PORT"] = str(port)
    log_file = open(logs_dir / "server.log", "w", encoding="utf-8")
    process = subprocess.Popen(
        server_cmd.split(),
        cwd=ROOT_DIR,
        env=env,
        text=True,
        stdout=log_file,
        stderr=subprocess.STDOUT,
    )
    process._coding_log_file = log_file  # type: ignore[attr-defined]
    return process


def stop_server(process: subprocess.Popen[str] | None) -> None:
    """停止 process 参数对应的被测服务进程。"""
    if process is None:
        return
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=5)
    log_file = getattr(process, "_coding_log_file", None)
    if log_file is not None:
        log_file.close()


def run_case(case_name: str, base_url: str) -> bool:
    """运行 case_name 参数指定的用例，并把结果写入当前用例目录。"""
    case_file = CASES_DIR / f"{case_name}.py"
    output_dir = OUTPUT_BASE_DIR / case_name
    if output_dir.exists():
        shutil.rmtree(output_dir)
    screenshots_dir = output_dir / "screenshots"
    logs_dir = output_dir / "logs"
    screenshots_dir.mkdir(parents=True, exist_ok=True)
    logs_dir.mkdir(parents=True, exist_ok=True)

    file_handler = logging.FileHandler(logs_dir / "test.log", encoding="utf-8")
    file_handler.setFormatter(logging.Formatter("%(asctime)s - %(levelname)s - %(message)s"))
    logger.addHandler(file_handler)

    try:
        logger.info("开始运行用例: %s", case_name)
        spec = importlib.util.spec_from_file_location(case_name, case_file)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"无法加载用例: {case_file}")
        module = importlib.util.module_from_spec(spec)
        sys.modules[case_name] = module
        spec.loader.exec_module(module)

        success = module.run_test(
            base_url=base_url,
            output_dir=output_dir,
            screenshots_dir=screenshots_dir,
            logs_dir=logs_dir,
            logger=logger,
        )
        logger.info("用例 %s 结果: %s", case_name, "通过" if success else "失败")
        return bool(success)
    except Exception as err:
        logger.exception("用例运行异常: %s", err)
        return False
    finally:
        logger.removeHandler(file_handler)
        file_handler.close()


def main() -> int:
    """解析命令行参数并运行 E2E 测试。"""
    parser = argparse.ArgumentParser(description="运行 WebSocket E2E 测试")
    parser.add_argument("--case", help="指定要运行的用例")
    parser.add_argument("--base-url", default=os.getenv("E2E_BASE_URL"), help="复用外部服务地址")
    parser.add_argument("--server-cmd", default=str(ROOT_DIR / "data" / "web" / "web"), help="服务启动命令")
    args = parser.parse_args()

    cases = discover_cases()
    if args.case:
        cases = [args.case]
    missing_cases = [case for case in cases if not (CASES_DIR / f"{case}.py").exists()]
    if missing_cases:
        logger.error("用例不存在: %s", ", ".join(missing_cases))
        return 1

    server_process: subprocess.Popen[str] | None = None
    base_url = args.base_url
    run_logs_dir = OUTPUT_BASE_DIR / datetime.now().strftime("%Y%m%d-%H%M%S-run") / "logs"
    run_logs_dir.mkdir(parents=True, exist_ok=True)

    try:
        if not base_url:
            port = find_free_port()
            base_url = f"http://127.0.0.1:{port}"
            server_process = start_server(args.server_cmd, port, run_logs_dir)
            wait_until_ready(base_url)
        logger.info("E2E 目标地址: %s", base_url)

        results = {case: run_case(case, base_url) for case in cases}
        for case, success in results.items():
            logger.info("%s: %s", case, "PASSED" if success else "FAILED")
        return 0 if all(results.values()) else 1
    finally:
        stop_server(server_process)


if __name__ == "__main__":
    raise SystemExit(main())
