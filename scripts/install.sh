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
CHANNEL=""
RESOLVE_ONLY=""
BIN_DIR="${EOS_INSTALL_BIN:-$HOME/.local/bin}"
DIST_DIR="${EOS_INSTALL_DIR:-$HOME/.local/share/eos}"

while [ $# -gt 0 ]; do
  case "$1" in
    --channel) CHANNEL="${2:-}"; shift 2 ;;
    --resolve-only) RESOLVE_ONLY=1; shift ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --bin) BIN_DIR="${2:-}"; shift 2 ;;
    --dir) DIST_DIR="${2:-}"; shift 2 ;;
    -h|--help)
      cat <<'USAGE'
用法: install.sh [--channel stable|beta] [--version vX.Y.Z] [--resolve-only] [--bin DIR] [--dir DIR]
  --channel  安装渠道（新安装默认 stable；现有安装沿用其渠道）
  --version  安装指定版本（校验 Release 属性并记录正确渠道）
  --resolve-only  仅输出选中的版本和渠道，不安装
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

# The selector is embedded so curl | bash remains a single-file installer.
release_json() {
  awk -v channel="$1" -v want="$2" -v best="$3" -v mode="${4:-release}" '
# Strict JSON reader: only top-level release objects supply selection fields.
# No jq/Python/Node dependency is required by the standalone Bash installer.
function fail() { bad=1; exit 1 }
function ws() { while (substr(json,pos,1) ~ /[ \t\r\n]/ && pos<=length(json)) pos++ }
function stringval( out,c,e) {
 if (substr(json,pos++,1)!="\"") fail()
 out=""
 while (pos<=length(json)) {
  c=substr(json,pos++,1)
  if(c=="\"") return out
  if(c=="\\") {
   e=substr(json,pos++,1)
   if(e=="u") { if(substr(json,pos,4)!~/^[0-9a-fA-F]+$/ || length(substr(json,pos,4))!=4) fail(); out=out "\\u" substr(json,pos,4); pos+=4 }
   else if(e ~ /^["\\\/bfnrt]$/) out=out "\\" e
   else fail()
  } else { if(c ~ /[\r\n\t]/) fail(); out=out c }
 }
 fail()
}
function newer(a,b, ap,bp,av,bv,i,an,bn) {
 sub(/^v/,"",a); sub(/^v/,"",b)
 an=split(a,ap,/-beta\./); bn=split(b,bp,/-beta\./)
 split(ap[1],av,"."); split(bp[1],bv,".")
 for(i=1;i<=3;i++) if(av[i]+0!=bv[i]+0) return av[i]+0>bv[i]+0
 if(an!=bn) return an<bn
 return ap[2]+0>bp[2]+0
}
function candidate(tag,pre,draft, tc,plain) {
 count++
 if(draft!="false" || (pre!="true" && pre!="false")) return
 if(tag!~/^v?[0-9]+\.[0-9]+\.[0-9]+(-beta\.[0-9]+)?$/) return
 plain=tag;sub(/^v/,"",plain)
 tc=(plain=="1.0.0-beta.34" || plain!~/-beta\./)?"stable":"beta"
 if((tc=="beta") != (pre=="true")) return
 if(want!="") { plain=want;sub(/^v/,"",plain); if("v" plain != (tag~/^v/?tag:"v" tag)) return }
 if(channel!="" && channel!=tc) return
 if(best=="" || newer(tag,best)) { best=tag; bestchannel=tc }
}
function object(depth, key,v,vt,tag,pre,draft,seenTag,seenPre,seenDraft) {
 pos++; ws()
 if(substr(json,pos,1)=="}") { pos++; if((depth==0 || (rootkind=="array" && depth==1))) fail(); return }
 while(1) {
  ws();key=stringval();ws();if(substr(json,pos++,1)!=":")fail()
  v=value(depth+1); vt=kind
  if((depth==0 || (rootkind=="array" && depth==1))) {
   if(mode=="installation" && depth==0 && key=="channel") { if(vt!="string")fail();installchannel=v }
   if(mode=="installation" && depth==0 && key=="method") { if(vt!="string")fail();installmethod=v }
   if(key=="tag_name") { if(seenTag++ || vt!="string")fail();tag=v }
   if(key=="prerelease") { if(seenPre++ || vt!="boolean")fail();pre=v }
   if(key=="draft") { if(seenDraft++ || vt!="boolean")fail();draft=v }
  }
  ws();v=substr(json,pos++,1);if(v=="}")break;if(v!=",")fail()
 }
 if((depth==0 || (rootkind=="array" && depth==1))) { if(mode!="installation"){if(!seenTag || !seenPre || !seenDraft)fail();candidate(tag,pre,draft)} }
}
function array(depth, c) {
 pos++;ws();if(substr(json,pos,1)=="]"){pos++;return}
 while(1) { value(depth+1);ws();c=substr(json,pos++,1);if(c=="]")return;if(c!=",")fail() }
}
function value(depth, c,start,token) {
 if(depth>64)fail()
 ws();c=substr(json,pos,1)
 if(c=="{"){object(depth);kind="object";return ""}
 if(c=="["){array(depth);kind="array";return ""}
 if(c=="\""){token=stringval();kind="string";return token}
 start=pos
 while(pos<=length(json) && substr(json,pos,1)!~/[ \t\r\n,\]}]/)pos++
 token=substr(json,start,pos-start)
 if(token=="true" || token=="false")kind="boolean"
 else if(token=="null")kind="null"
 else if(token ~ /^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$/)kind="number"
 else fail()
 return token
}
{json=json $0 "\n"}
END {
 if(bad)exit 1
 pos=1;ws();if(substr(json,pos,1)!~/[\[{]/)fail()
 rootkind=(substr(json,pos,1)=="["?"array":"object");value(0);ws();if(pos<=length(json))fail()
 if(mode=="installation") {
  if(installchannel!="stable" && installchannel!="beta")fail()
  if(installmethod!="script" && installmethod!="archive" && installmethod!="package-manager")fail()
  print 0,installchannel,installmethod; exit
 }
 if(best=="") print count+0,"-","-"
 else { if(bestchannel=="") bestchannel=channel; print count+0,best,bestchannel }
}
  '
}
channel_for_tag() {
  case "${1#v}" in
    1.0.0-beta.34) echo stable ;;
    *-beta.*) echo beta ;;
    *-*) err "不支持的版本渠道: $1" ;;
    *) echo stable ;;
  esac
}
[ -z "$CHANNEL" ] || [ "$CHANNEL" = stable ] || [ "$CHANNEL" = beta ] || err "渠道必须是 stable 或 beta"
case "$(printf '%s' "$DIST_DIR" | tr '[:upper:]' '[:lower:]')/" in
  */cellar/*|*/homebrew/caskroom/*|*/scoop/apps/*|*/chocolatey/*|*/nix/store/*|*/snap/*)
    err "此目录由包管理器维护，请使用原包管理器更新" ;;
esac
if [ -f "$DIST_DIR/eos-install.json" ]; then
  marker="$(release_json "" "" "" installation < "$DIST_DIR/eos-install.json")" || err "安装渠道记录无效"
  read -r marker_count installed_channel installed_method <<< "$marker"
  [ "$installed_method" != package-manager ] || err "此安装由包管理器维护，请使用原包管理器更新"
  if [ -z "$CHANNEL" ] && [ -z "$VERSION" ]; then CHANNEL="$installed_channel"; fi
fi
if [ -z "$CHANNEL" ] && [ -z "$VERSION" ] && [ -f "$DIST_DIR/eos" ]; then
  installed_version="$("$DIST_DIR/eos" version | awk 'NR==1 && $1=="eos" {print $2}')"
  [ -n "$installed_version" ] || err "无法识别现有安装，请明确指定 --channel 或 --version"
  CHANNEL="$(channel_for_tag "$installed_version")"
fi
API="https://api.github.com/repos/${REPO}/releases"
if [ -n "$VERSION" ]; then
  # A version pin is checked against GitHub's actual draft/prerelease flags.
  VERSION="v${VERSION#v}"
  metadata="$(curl -fsSL --retry 3 --connect-timeout 10 --max-time 30 "$API/tags/$VERSION")" || err "无法读取指定版本的发布信息"
  resolved="$(printf '%s' "$metadata" | release_json "$CHANNEL" "$VERSION" "")" || err "发布信息无效"
  read -r release_count VERSION CHANNEL <<< "$resolved"
  [ "$VERSION" != - ] || err "指定版本不存在、不是公开发布或与渠道不一致"
else
  CHANNEL="${CHANNEL:-stable}"
  if [ "$CHANNEL" = stable ]; then
    metadata="$(curl -fsSL --retry 3 --connect-timeout 10 --max-time 30 "$API/latest")" || err "无法获取正式渠道版本"
    resolved="$(printf '%s' "$metadata" | release_json stable "" "")" || err "正式发布信息无效"
    read -r release_count VERSION selected_channel <<< "$resolved"
    [ "$VERSION" != - ] || err "latest 不是公开正式发布，拒绝跨渠道安装"
  else
    BEST=""
    for page in $(seq 1 100); do
      metadata="$(curl -fsSL --retry 3 --connect-timeout 10 --max-time 30 "$API?per_page=100&page=$page")" || err "无法读取 Beta 发布信息（可能是网络或 GitHub 限流）"
      resolved="$(printf '%s' "$metadata" | release_json beta "" "$BEST")" || err "Beta 发布信息无效"
      read -r release_count BEST selected_channel <<< "$resolved"
      [ "$BEST" != - ] || BEST=""
      [ "$release_count" -ge 100 ] || break
      [ "$page" -lt 100 ] || err "发布分页超出上限"
    done
    [ -n "$BEST" ] || err "没有已发布的 Beta 版本"
    VERSION="$BEST"
  fi
fi
if [ -n "$RESOLVE_ONLY" ]; then printf '%s %s\n' "$VERSION" "$CHANNEL"; exit 0; fi
echo "目标版本: ${VERSION} (${OS}/${ARCH}), 渠道: ${CHANNEL}"

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
printf '{"channel":"%s","method":"script"}\n' "$CHANNEL" > "${DIST_DIR}/eos-install.json"

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
