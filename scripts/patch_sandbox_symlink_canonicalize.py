#!/usr/bin/env python3
"""Patch eos-core-sandbox check_write to resolve symlinks before boundary checks.

SEC-02 via_symlink: lexical path_within treats workspace/escape_link/via_link as
inside the workspace even when escape_link is a symlink to OutsideRoot. The
vendored core then creates the directory outside. Fix: resolve existing symlink
components (and canonicalize roots) before the writable-root membership test.

Idempotent: skips if the sentinel comment is already present.
"""

from __future__ import annotations

import pathlib
import sys

SENTINEL = "symlink-canonicalize-for-writable-roots (SEC-02)"

RESOLVE_FN = r'''
fn resolve_symlinks_for_policy(path: &std::path::Path) -> std::path::PathBuf {
    // Walk components, resolving each existing symlink. Non-existent final
    // components (e.g. mkdir target) stay lexical on top of the resolved prefix.
    // This closes SEC-02 via_symlink: workspace/escape_link/child must be judged
    // against the symlink target, not the lexical workspace-relative spelling.
    let mut out = std::path::PathBuf::new();
    for component in path.components() {
        match component {
            std::path::Component::Prefix(_) | std::path::Component::RootDir => {
                out.push(component.as_os_str());
            }
            std::path::Component::CurDir => {}
            std::path::Component::ParentDir => {
                let _ = out.pop();
            }
            std::path::Component::Normal(part) => {
                out.push(part);
                if let Ok(meta) = std::fs::symlink_metadata(&out) {
                    if meta.file_type().is_symlink() {
                        match std::fs::canonicalize(&out) {
                            Ok(canon) => out = canon,
                            Err(_) => {
                                // Dangling symlink: fall back to read_link join.
                                if let Ok(target) = std::fs::read_link(&out) {
                                    let parent = out.parent().unwrap_or_else(|| std::path::Path::new(""));
                                    let joined = if target.is_absolute() {
                                        target
                                    } else {
                                        parent.join(target)
                                    };
                                    out = resolve_symlinks_for_policy(&joined);
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    out
}

fn absolutize_for_policy(path: &str, workspace_root: &str) -> std::path::PathBuf {
    let p = std::path::Path::new(path);
    if p.is_absolute() {
        return resolve_symlinks_for_policy(p);
    }
    let base = workspace_root.trim();
    if base.is_empty() {
        return resolve_symlinks_for_policy(p);
    }
    resolve_symlinks_for_policy(&std::path::Path::new(base).join(p))
}
'''

CHECK_WRITE_NEW = r'''
pub fn check_write(path: &str, policy: &SandboxPolicy) -> CheckResult {
    // ''' + SENTINEL + r'''
    match policy.mode {
        SandboxMode::ReadOnly => CheckResult::deny(
            "read-only policy denies all writes; if this write is required, ask the user to switch the access boundary to workspace-write or full access",
        ),
        SandboxMode::DangerFullAccess => CheckResult::allow("danger-full-access allows all writes"),
        SandboxMode::WorkspaceWrite => {
            let mut roots: Vec<String> = Vec::new();
            let ws = policy.workspace_root.trim();
            if !ws.is_empty() {
                roots.push(
                    resolve_symlinks_for_policy(std::path::Path::new(ws))
                        .to_string_lossy()
                        .into_owned(),
                );
            }
            for r in &policy.writable_roots {
                let trimmed = r.trim();
                if !trimmed.is_empty() {
                    roots.push(
                        resolve_symlinks_for_policy(std::path::Path::new(trimmed))
                            .to_string_lossy()
                            .into_owned(),
                    );
                }
            }

            if roots.is_empty() {
                return CheckResult::deny(
                    "workspace-write sandbox is active but no writable root is configured for this session; every write is blocked regardless of path. The supplied path cannot be written here. Ask the user to open a workspace folder, or report that the desktop failed to register the workspace root with the core.",
                );
            }

            let resolved = absolutize_for_policy(path, ws);
            let resolved_str = resolved.to_string_lossy();
            for root in &roots {
                if path_within(root, resolved_str.as_ref()) {
                    return CheckResult::allow(format!("path is within writable root: {}", root));
                }
            }

            let root_refs: Vec<&str> = roots.iter().map(String::as_str).collect();
            CheckResult::deny(outside_writable_roots_reason(&root_refs))
        }
    }
}
'''


def main() -> int:
    if len(sys.argv) != 2:
        print(f"usage: {sys.argv[0]} path/to/eos-core-sandbox/src/lib.rs", file=sys.stderr)
        return 2
    path = pathlib.Path(sys.argv[1])
    text = path.read_text(encoding="utf-8")
    if SENTINEL in text:
        print(f"already patched: {path}")
        return 0

    # Insert helpers just before `pub fn check_write`.
    marker = "pub fn check_write(path: &str, policy: &SandboxPolicy) -> CheckResult {"
    idx = text.find(marker)
    if idx < 0:
        print("ERROR: check_write not found", file=sys.stderr)
        return 1

    # Find the end of the existing check_write function by brace matching from marker.
    brace_start = text.find("{", idx)
    depth = 0
    end = None
    for i, ch in enumerate(text[brace_start:], start=brace_start):
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                end = i + 1
                break
    if end is None:
        print("ERROR: could not find end of check_write", file=sys.stderr)
        return 1

    new_text = text[:idx] + RESOLVE_FN + "\n" + CHECK_WRITE_NEW.strip() + "\n" + text[end:]
    path.write_text(new_text, encoding="utf-8")
    print(f"patched: {path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
