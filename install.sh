#!/usr/bin/env sh
set -eu

repo="${AGENTHUB_REPO:-117503445/agenthub}"
branch="${AGENTHUB_BRANCH:-master}"
release_tag="${AGENTHUB_RELEASE_TAG:-}"
install_dir="${AGENTHUB_INSTALL_DIR:-/usr/local/bin}"
bin_name="${AGENTHUB_BIN:-agenthub}"

# log_info 输出 message 参数指定的普通安装日志。
log_info() {
	printf '%s\n' "$*" >&2
}

# fail 输出 message 参数指定的错误并终止安装。
fail() {
	printf '错误: %s\n' "$*" >&2
	exit 1
}

# require_downloader 确认当前系统存在 curl 或 wget。
require_downloader() {
	if command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1; then
		return
	fi
	fail "需要安装 curl 或 wget"
}

# download_stdout 下载 url 参数指定的内容并输出到 stdout。
download_stdout() {
	url="$1"
	require_downloader
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url"
		return
	fi
	wget -qO- "$url"
}

# download_file 下载 url 参数指定的内容到 output 参数指定的文件。
download_file() {
	url="$1"
	output="$2"
	require_downloader
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$output" "$url"
		return
	fi
	wget -qO "$output" "$url"
}

# sanitize_ref 把 ref 参数转换成 Release tag 中使用的安全名称。
sanitize_ref() {
	ref="$1"
	printf '%s' "$ref" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9._-]/-/g; s/--*/-/g; s/^-//; s/-$//'
}

# detect_os 返回当前系统对应的发布产物 OS 名称。
detect_os() {
	case "$(uname -s)" in
		Linux) printf 'linux' ;;
		Darwin) printf 'darwin' ;;
		*) fail "当前系统不支持: $(uname -s)" ;;
	esac
}

# detect_arch 返回当前 CPU 架构对应的发布产物架构名称。
detect_arch() {
	case "$(uname -m)" in
		x86_64 | amd64) printf 'amd64' ;;
		arm64 | aarch64) printf 'arm64' ;;
		*) fail "当前架构不支持: $(uname -m)" ;;
	esac
}

# latest_branch_release_tag 查询 branch 参数最新 commit 对应的 Release tag。
latest_branch_release_tag() {
	ref="$1"
	api_url="https://api.github.com/repos/${repo}/commits/${ref}"
	sha="$(download_stdout "$api_url" | tr '{,' '\n' | sed -n 's/.*"sha":[[:space:]]*"\([0-9a-f]\{40\}\)".*/\1/p' | head -n 1)"
	if [ -z "$sha" ]; then
		fail "无法查询 ${repo} 的 ${ref} 最新提交"
	fi
	short_sha="$(printf '%s' "$sha" | cut -c 1-12)"
	safe_ref="$(sanitize_ref "$ref")"
	if [ -z "$safe_ref" ]; then
		fail "Release 分支名无效: ${ref}"
	fi
	printf '%s-%s' "$safe_ref" "$short_sha"
}

# verify_checksum 使用 sums 参数指定的校验文件验证 file 参数指定的二进制。
verify_checksum() {
	file="$1"
	sums="$2"
	name="$(basename "$file")"
	expected="$(awk -v name="$name" '$2 == name {print $1}' "$sums")"
	if [ -z "$expected" ]; then
		fail "校验文件中缺少 ${name}"
	fi

	if command -v sha256sum >/dev/null 2>&1; then
		actual="$(sha256sum "$file" | awk '{print $1}')"
	elif command -v shasum >/dev/null 2>&1; then
		actual="$(shasum -a 256 "$file" | awk '{print $1}')"
	else
		fail "需要安装 sha256sum 或 shasum 以校验下载内容"
	fi

	if [ "$expected" != "$actual" ]; then
		fail "SHA256 校验失败: ${name}"
	fi
}

# install_binary 将 file 参数指定的二进制安装到 target_dir 参数指定的目录。
install_binary() {
	file="$1"
	target_dir="$2"
	target="${target_dir%/}/${bin_name}"

	if ! mkdir -p "$target_dir" 2>/dev/null; then
		if ! command -v sudo >/dev/null 2>&1; then
			fail "无法创建安装目录 ${target_dir}，且 sudo 不可用"
		fi
		sudo mkdir -p "$target_dir"
	fi

	if install -m 0755 "$file" "$target" 2>/dev/null; then
		return
	fi
	if ! command -v sudo >/dev/null 2>&1; then
		fail "无法写入 ${target}，且 sudo 不可用"
	fi
	sudo install -m 0755 "$file" "$target"
}

os="$(detect_os)"
arch="$(detect_arch)"
asset="agenthub-${os}-${arch}"

if [ -z "$release_tag" ]; then
	release_tag="$(latest_branch_release_tag "$branch")"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

base_url="https://github.com/${repo}/releases/download/${release_tag}"
log_info "下载 ${repo} ${release_tag} 的 ${asset}"
download_file "${base_url}/${asset}" "${tmp_dir}/${asset}"
download_file "${base_url}/SHA256SUMS" "${tmp_dir}/SHA256SUMS"
verify_checksum "${tmp_dir}/${asset}" "${tmp_dir}/SHA256SUMS"
install_binary "${tmp_dir}/${asset}" "$install_dir"
log_info "已安装 ${install_dir%/}/${bin_name}"
