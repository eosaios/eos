# Copyright (c) 2026 EOSAIOS
# SPDX-License-Identifier: EOS-NCL-1.1
# 本文件基于 EOS 非商用许可证 v1.1 发布，详见 LICENSE。
# 商业使用请联系版权人获得商业授权。

"""打包 CLI windows 发布 zip（仓库内脚本，供 release workflow 调用）。

用途：Windows 发布归档必须用 ZIP 规范的正斜杠条目（目录条目以 "/" 结尾）。
PowerShell Compress-Archive 违反之（反斜杠），Git Bash 的 GNU tar -a 对
zip 无效（产出裸 tar 无 zip 结构），两者都导致 eos update 解压失败。
本脚本用 Python zipfile 显式写 POSIX 条目 + 目录条目。

用法：python scripts/pack_zip.py <stage_dir>（在 dist/ 下运行或传绝对路径）
产出：<stage_dir>.zip（与 stage_dir 同级）
"""

import os
import sys
import zipfile


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: pack_zip.py <stage_dir>", file=sys.stderr)
        return 2
    stage = sys.argv[1].rstrip(os.sep)
    if not os.path.isdir(stage):
        print(f"error: not a directory: {stage}", file=sys.stderr)
        return 1

    out_zip = stage + ".zip"
    with zipfile.ZipFile(out_zip, "w", zipfile.ZIP_DEFLATED) as z:
        for root, dirs, files in os.walk(stage):
            for d in sorted(dirs):
                abs_path = os.path.join(root, d)
                rel = os.path.relpath(abs_path, stage).replace("\\", "/")
                z.writestr(rel + "/", b"")
            for f in sorted(files):
                abs_path = os.path.join(root, f)
                rel = os.path.relpath(abs_path, stage).replace("\\", "/")
                z.write(abs_path, rel)
    print(f"packed {out_zip}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
