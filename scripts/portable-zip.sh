#!/bin/sh
# 打 Windows 的免安装便携包：把 Gloss.exe 和它旁边运行时要用的文件收成
# 一个 zip，解压即用。
#
# 为什么只有一个包而不是整个 build/windows-amd64：那个目录是发布流程的
# 暂存区，装着四种不该给用户的东西 ——
#
#   - Gloss Setup X.exe：NSIS 安装包，走的是安装那条路；
#   - gloss-<版本>-windows-amd64.tar.gz、.delta、update-windows-amd64.json：
#     自动更新的产物。客户端只按 update-windows-amd64.json 里的 URL 下载，
#     带 ed25519 签名校验，不会去 release 资产里按名字找包，zip 里多一份
#     它也用不上；
#   - resources 下的 PNG：已经 go:embed 进二进制了，磁盘上那份没人读。
#
# 这些文件和 Gloss.exe 躺在一起，zip 整个目录就会连它们一起带走 ——
# v0.3.32 的便携包 30MB，其中 21MB 是它们。装不了安装包的用户下载的
# 是一个跑得起来、但 70% 体积没用的包。
#
# 用法：portable-zip.sh <staging-dir> <output.zip>
# staging-dir 是 mygo build 的产物目录，exe 和原生库必须在它下面。
set -eu

stage="$1"
out="$2"

test -d "$stage" || { echo "portable-zip: $stage 不是目录" >&2; exit 1; }
test -n "$out" || { echo "portable-zip: 没有输出路径" >&2; exit 1; }

exe="Gloss.exe"
test -f "$stage/$exe" || { echo "portable-zip: $stage 里没有 $exe" >&2; exit 1; }

# 只收运行时要用的：可执行文件，加上终端插件运行时才加载的原生库
# （plugins/terminal 的 library.Find 依次找资源目录、exe 旁边、用户缓存，
# 所以放在 exe 旁边就够）。其余文件一律不收。
files="$exe"
for f in "$stage"/*.dll; do
	[ -f "$f" ] || continue
	files="$files ${f##*/}"
done

rm -f "$out"
# -j 丢掉目录层级：staging 里这些文件本来就在顶层，zip 里也不该有目录，
# 解压出来直接是 Gloss.exe。
# shellcheck disable=SC2086 # files 是上面拼出来的文件名清单，故意要分词
(cd "$stage" && zip -qj "$out" $files)

echo "portable-zip: $(basename "$out") 收了 $files"
