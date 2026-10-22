#!/usr/bin/env python3
"""Exercise the actual CLI in archive, installer-symlink and standalone layouts.

Only loopback HTTP and temporary HOME/store/cache directories are used. Compare
the resolved core bytes to the archive manifest, then exercise a provider that
returns explicit null arrays (MiMo's empty tool_calls/usage-chunk shape).
"""

import argparse
import hashlib
import http.server
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading


ANSWER = "EOS CLI 普通对话正常"


class Model(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if self.path != "/v1/chat/completions":
            self.send_error(404)
            return
        usage = {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}
        if request.get("stream"):
            chunks = [
                {"choices": [{"index": 0, "delta": {"content": ANSWER, "tool_calls": None}, "finish_reason": None}]},
                {"choices": [{"index": 0, "delta": {"tool_calls": None}, "finish_reason": "stop"}]},
                {"choices": None, "usage": usage},
            ]
            body = ("".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n").encode()
            content_type = "text/event-stream"
        else:
            body = json.dumps({"choices": [{"message": {"content": ANSWER, "tool_calls": None}, "finish_reason": "stop"}], "usage": usage}).encode()
            content_type = "application/json"
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def run(cli, args, workspace, env):
    result = subprocess.run([str(cli), *args], cwd=workspace, env=env, capture_output=True, text=True, encoding="utf-8", timeout=45)
    if result.returncode:
        raise RuntimeError(f"{cli.name} {args}: exit={result.returncode}\n{result.stdout}\n{result.stderr}")
    return result.stdout


def verify(cli, core_dir):
    manifest = json.loads((core_dir / "manifest.json").read_text())
    expected_hash = manifest["sha256"].removeprefix("sha256:")
    if hashlib.sha256((core_dir / manifest["binary"]).read_bytes()).hexdigest() != expected_hash:
        raise RuntimeError("archive core does not match its manifest")
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Model)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="eos-release-runtime-") as temp:
            root = Path(temp).resolve()
            package = root / "share" / "eos"
            package.mkdir(parents=True)
            packaged_cli = package / cli.name
            shutil.copy2(cli, packaged_cli)
            shipped_core = package / "core" / manifest["target"]
            shutil.copytree(core_dir, shipped_core)
            standalone = root / "standalone" / cli.name
            standalone.parent.mkdir()
            shutil.copy2(cli, standalone)
            layouts = [("archive", packaged_cli, shipped_core / manifest["binary"])]
            if os.name != "nt":
                linked_cli = root / "bin" / cli.name
                linked_cli.parent.mkdir()
                linked_cli.symlink_to(Path("..") / "share" / "eos" / cli.name)
                layouts.append(("installer-symlink", linked_cli, shipped_core / manifest["binary"]))
            layouts.append(("standalone", standalone, None))
            for name, executable, expected_core in layouts:
                state = root / name
                home, workspace = state / "home", state / "workspace"
                home.mkdir(parents=True)
                workspace.mkdir()
                # No inherited provider/credential/override variables, and no
                # real user's config, store, keyring or logs.
                env = {k: v for k, v in os.environ.items() if k.upper() in {"PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "LANG", "LC_ALL"}}
                env.update({
                    "HOME": str(home), "USERPROFILE": str(home),
                    "XDG_CONFIG_HOME": str(state / "config"),
                    "XDG_CACHE_HOME": str(state / "cache"),
                    "XDG_DATA_HOME": str(state / "data"),
                    "XDG_STATE_HOME": str(state / "logs"),
                    "LocalAppData": str(state / "cache"), "APPDATA": str(state / "config"),
                    "EOS_HOME": str(home / ".eos"),
                    "EOS_CORE_STORE_DIR": str(state / "store"),
                    "EOS_WORKSPACE_ROOT": str(workspace),
                    "EOS_SANDBOX_WORKSPACE_ROOT": str(workspace),
                    "EOS_API_BASE": f"http://127.0.0.1:{server.server_port}/v1",
                    "EOS_API_KEY": "loopback-fixture-only", "EOS_MODEL": "mimo-null-fixture",
                    "EOS_MEMORIES_GENERATE": "false", "EOS_MEMORIES_USE": "false",
                    "EOS_KEYRING": "filevault-only", "EOS_LOG_LEVEL": "error",
                })
                report = run(executable, ["doctor"], workspace, env)
                if expected_core is None:
                    cache = home / "Library" / "Caches" if os.sys.platform == "darwin" else state / "cache"
                    if os.name == "nt":
                        # Go's UserCacheDir uses LocalAppData on Windows.
                        cache = state / "cache"
                    expected_core = cache / "eos" / "core" / manifest["target"] / expected_hash[:12] / manifest["binary"]
                if str(expected_core) not in report:
                    raise RuntimeError(f"{name}: CLI did not resolve expected core {expected_core}\n{report}")
                if hashlib.sha256(expected_core.read_bytes()).hexdigest() != expected_hash:
                    raise RuntimeError(f"{name}: embedded/selected core differs from archive core")
                for args in (["exec", "你是什么模型？", "--output", "json", "--timeout", "30s"], ["--print", "你好", "--output-format", "json"]):
                    output = json.loads(run(executable, args, workspace, env))
                    if output.get("content") != ANSWER or "error" in output:
                        raise RuntimeError(f"{name}: wrong conversation result: {output}")
                print(f"PASS {name}: sha256={expected_hash} exec/print explicit-null conversation", flush=True)
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cli", required=True, type=Path)
    parser.add_argument("--core-dir", required=True, type=Path)
    options = parser.parse_args()
    verify(options.cli.resolve(), options.core_dir.resolve())
