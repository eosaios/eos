#!/usr/bin/env bash
# EOS CLI 官方安装脚本（macOS / Linux）
#
# 用法：
#   curl -fsSL https://cdn.jsdelivr.net/gh/eosaios/eos@main/scripts/install.sh | bash
#   ./install.sh --version v1.0.0          # 安装指定版本
#   ./install.sh --bin ~/.local/bin --dir ~/.local/share/eos
#
# 行为：GitHub Releases 拉取对应平台归档 → SHA256 校验 → 安装到
#   ~/.local/share/eos（eos + core/ 整树）→ 符号链接 ~/.local/bin/eos。
# PATH 不含链接目录时自动写入当前 shell 的配置文件（幂等），装完即可使用。
# `eos update` 之后升级同一位置，符号链接保持不变。

set -euo pipefail

REPO="eosaios/eos"
VERSION=""
BIN_DIR="${EOS_INSTALL_BIN:-$HOME/.local/bin}"
DIST_DIR="${EOS_INSTALL_DIR:-$HOME/.local/share/eos}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --bin) BIN_DIR="${2:-}"; shift 2 ;;
    --dir) DIST_DIR="${2:-}"; shift 2 ;;
    -h|--help)
      cat <<'USAGE'
用法: install.sh [--version vX.Y.Z] [--bin DIR] [--dir DIR]
  --version  安装指定版本（默认最新）
  --bin      可执行文件链接目录（默认 ~/.local/bin，可用 EOS_INSTALL_BIN 覆盖）
  --dir      安装目录（默认 ~/.local/share/eos，可用 EOS_INSTALL_DIR 覆盖）
USAGE
      exit 0 ;;
    *) echo "未知参数: $1（--help 查看用法）" >&2; exit 1 ;;
  esac
done

err() { echo "错误: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "缺少依赖 $1，请先安装"; }
need curl; need tar

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin|linux) ;;
  *) err "不支持的系统: $(uname -s)（本脚本支持 macOS / Linux）" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  arm64|aarch64) ARCH="arm64" ;;
  x86_64|amd64)  ARCH="amd64" ;;
  *) err "不支持的架构: $(uname -m)" ;;
esac

SHA_CMD="sha256sum"; command -v sha256sum >/dev/null 2>&1 || SHA_CMD="shasum -a 256"

# 解析最新版本（未指定 --version 时）
if [ -z "$VERSION" ]; then
  echo "正在获取最新版本..."
  # 不走 api.github.com：未认证 API 限流 60 次/小时/IP，共享出口极易触发
  # rate limit。releases/latest 网页 302 到 releases/tag/<版本>，取
  # Location 头解析 tag，不占 API 配额。
  VERSION="$(curl -fsSI --retry 3 --connect-timeout 10 --max-time 30 \
    "https://github.com/${REPO}/releases/latest" \
    | tr -d '\r' | sed -n 's/^[Ll]ocation:.*\/tag\/\([^[:space:]]*\).*/\1/p' | tail -1)"
  [ -n "$VERSION" ] || err "无法获取最新版本号，请到 https://github.com/${REPO}/releases 手动下载"
fi
echo "目标版本: ${VERSION} (${OS}/${ARCH})"

VER_NUM="${VERSION#v}"
ASSET="eos-cli_v${VER_NUM}_${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# 下载 + SHA256 校验一体循环：
#   - --http1.1：部分网络对 GitHub CDN 的 HTTP/2 流会中途断流
#     （curl: (92) PROTOCOL_ERROR），降级 HTTP/1.1 实测稳定；
#   - -C - 断点续传：重试不从头下载；
#   - 校验不过自动删档重下，防止截断的包被安装。
WANT=""
VERIFIED=""
for round in 1 2 3 4 5; do
  if [ -z "$WANT" ]; then
    curl -fL --http1.1 --retry 2 --connect-timeout 10 --max-time 30 \
      -o "${TMP}/SHA256SUMS.txt" "${BASE_URL}/SHA256SUMS.txt" 2>/dev/null || true
    [ -s "${TMP}/SHA256SUMS.txt" ] && \
      WANT="$(awk -v f="$ASSET" '$2 == f {print tolower($1)}' "${TMP}/SHA256SUMS.txt")"
  fi
  if [ -n "$WANT" ]; then
    if [ ! -f "${TMP}/${ASSET}" ] || \
       [ "$($SHA_CMD "${TMP}/${ASSET}" | awk '{print tolower($1)}')" != "$WANT" ]; then
      echo "下载 ${ASSET}..."
      curl -fL --http1.1 --retry 2 --connect-timeout 10 --max-time 300 -C - \
        -o "${TMP}/${ASSET}" "${BASE_URL}/${ASSET}" || true
    fi
    if [ -f "${TMP}/${ASSET}" ] && \
       [ "$($SHA_CMD "${TMP}/${ASSET}" | awk '{print tolower($1)}')" = "$WANT" ]; then
      VERIFIED=1
      break
    fi
    rm -f "${TMP}/${ASSET}"
  fi
  echo "下载未完成，重试 (${round}/5)..."
done
[ -n "$VERIFIED" ] || err "下载失败：网络不稳定或 ${BASE_URL} 不可达。建议：1) 重新运行本脚本（支持断点续传）  2) 配置代理后重试  3) go install github.com/eosaios/eos@latest  4) 手动下载 ${BASE_URL}/${ASSET}"
echo "SHA256 校验通过"

echo "安装到 ${DIST_DIR} ..."
mkdir -p "$DIST_DIR" "$BIN_DIR"
# 旧版本先移走（运行中的进程不受影响），失败则清理后重试
OLD=""
if [ -d "${DIST_DIR}/core" ] || [ -e "${DIST_DIR}/eos" ]; then
  OLD="${DIST_DIR}.old-$$"
  mv "$DIST_DIR" "$OLD" 2>/dev/null || { rm -rf "$DIST_DIR"; }
  mkdir -p "$DIST_DIR"
fi
tar -xzf "${TMP}/${ASSET}" -C "$TMP"
# 归档结构固定为单一顶层目录（<stage>/eos + <stage>/core/）。不用 find
# -maxdepth：那是 GNU 扩展，macOS 自带的 BSD find 不支持会直接报错。
SRC_DIR=""
for d in "${TMP}"/*/; do
  if [ -f "${d}eos" ]; then SRC_DIR="${d%/}"; break; fi
done
[ -n "$SRC_DIR" ] || err "归档结构异常：未在 ${ASSET} 中找到 eos 可执行文件"
cp -R "${SRC_DIR}/." "$DIST_DIR/"
[ -n "$OLD" ] && rm -rf "$OLD" 2>/dev/null || true
chmod +x "${DIST_DIR}/eos"

ln -sf "${DIST_DIR}/eos" "${BIN_DIR}/eos"

# PATH 不含链接目录时，自动把 export 行写入当前 shell 的配置文件（幂等）：
# 优先按 $SHELL 判断（macOS 默认 zsh → ~/.zshrc），否则挑一个已存在的
# 常见配置文件，都没有则落到 ~/.profile（bash 登录 shell 会读）。
ensure_path() {
  case ":${PATH}:" in
    *":${BIN_DIR}:"*) return 0 ;;
  esac
  local rc=""
  case "${SHELL:-}" in
    */zsh*)  rc="$HOME/.zshrc" ;;
    */bash*) rc="$HOME/.bashrc" ;;
  esac
  if [ -z "$rc" ]; then
    for f in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.profile"; do
      if [ -f "$f" ]; then rc="$f"; break; fi
    done
  fi
  [ -n "$rc" ] || rc="$HOME/.profile"

  if [ ! -f "$rc" ] || ! grep -qF "${BIN_DIR}" "$rc"; then
    printf '\n# eos CLI\nexport PATH="%s:$PATH"\n' "${BIN_DIR}" >> "$rc"
    echo
    echo "已把 ${BIN_DIR} 写入 ${rc}"
    echo "新开一个终端即可使用 eos（或先执行: source ${rc}）"
  else
    echo
    echo "检测到 ${rc} 已包含 ${BIN_DIR}，但当前 PATH 未生效。"
    echo "请执行: source ${rc}（或新开一个终端）"
  fi
}
ensure_path

echo
"${BIN_DIR}/eos" --help >/dev/null 2>&1 || true
echo "安装完成: $(${BIN_DIR}/eos version 2>/dev/null || echo "${BIN_DIR}/eos")"
echo "运行 eos 开始使用；eos update 可自升级到最新版。"
