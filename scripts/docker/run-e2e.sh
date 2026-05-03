#!/usr/bin/env bash
set -uo pipefail

# run-e2e.sh 在 E2E 容器内运行测试，并把报告写入挂载目录。
report_dir="${AGENTHUB_REPORT_DIR:-/report}"
sha="${GITHUB_SHA:-local}"
ref_name="${GITHUB_REF_NAME:-local}"
repository="${GITHUB_REPOSITORY:-117503445/agenthub}"
repository_owner="${GITHUB_REPOSITORY_OWNER:-${repository%%/*}}"
server_url="${GITHUB_SERVER_URL:-https://github.com}"
run_id="${GITHUB_RUN_ID:-local}"
pages_url="${AGENTHUB_PAGES_URL:-https://${repository_owner}.github.io/${repository#*/}/reports/${sha}/}"
run_url="${AGENTHUB_RUN_URL:-${server_url}/${repository}/actions/runs/${run_id}}"

mkdir -p "${report_dir}"
cd /src
rm -rf data/e2e data/ci-report

set +e
agenthub-scripts e2e --server-cmd /src/data/web/web
exit_code="$?"
set -e

agenthub-scripts ci-report \
	--input data/e2e \
	--output data/ci-report \
	--sha "${sha}" \
	--ref "${ref_name}" \
	--run-url "${run_url}" \
	--pages-url "${pages_url}" \
	--e2e-exit-code "${exit_code}"

cp -R data/ci-report/. "${report_dir}/"
printf '%s\n' "${exit_code}" >"${report_dir}/e2e-exit-code"
exit 0
