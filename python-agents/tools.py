"""
DevFlow Agent Tools - File operations and repository analysis tools
"""
import os
import re
import json
import tempfile
import subprocess
from pathlib import Path
from strands_tools import file_read, file_write  # removed editor import
from strands import tool

def normalize_path_for_display(path: str) -> str:
    return path.replace("\\", "/")

@tool
def load_repo_analysis(repo_path: str) -> str:
    print(f"[Tool] load_repo_analysis: {normalize_path_for_display(repo_path)}")
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)
    analysis_file = os.path.join(repo_path, ".devflow", "repo-analysis.md")
    if not os.path.exists(analysis_file):
        msg = f"Repository analysis not found at {normalize_path_for_display(analysis_file)}"
        print(f"[Tool] {msg}")
        return msg
    try:
        with open(analysis_file, 'r', encoding='utf-8') as f:
            content = f.read()
        print(f"[Tool] Loaded analysis ({len(content)} chars)")
        return content
    except Exception as e:
        error_msg = f"Error reading repo analysis: {str(e)}"
        print(f"[Tool] {error_msg}")
        return error_msg

@tool
def load_dependency_graph(repo_path: str) -> str:
    print(f"[Tool] load_dependency_graph: {normalize_path_for_display(repo_path)}")
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)
    dep_file = os.path.join(repo_path, ".devflow", "dependency-graph.json")
    if not os.path.exists(dep_file):
        msg = f"Dependency graph not found at {normalize_path_for_display(dep_file)}"
        print(f"[Tool] {msg}")
        return msg
    try:
        with open(dep_file, 'r', encoding='utf-8') as f:
            content = json.load(f)
        formatted = json.dumps(content, indent=2)
        print(f"[Tool] Loaded dependency graph ({len(content.get('nodes', []))} nodes)")
        return formatted
    except Exception as e:
        error_msg = f"Error reading dependency graph: {str(e)}"
        print(f"[Tool] {error_msg}")
        return error_msg

@tool
def list_files(repo_path: str, max_files: int = 100) -> str:
    print(f"[Tool] list_files: {normalize_path_for_display(repo_path)}")
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)
    if not os.path.exists(repo_path):
        return f"Error: Directory does not exist: {repo_path}"

    ignore_patterns = {
        '.git', '__pycache__', 'node_modules', '.venv', '.venv-devflow',
        'venv', 'dist', 'build', '.devflow', '.pytest_cache', '.DS_Store'
    }
    files = []
    try:
        for root, dirs, filenames in os.walk(repo_path):
            dirs[:] = [d for d in dirs if d not in ignore_patterns]
            for filename in filenames:
                if filename.startswith('.'):
                    continue
                if any(pattern in filename for pattern in ignore_patterns):
                    continue
                abs_path = os.path.join(root, filename)
                rel_path = os.path.relpath(abs_path, repo_path)
                files.append(normalize_path_for_display(rel_path))
                if len(files) >= max_files:
                    break
            if len(files) >= max_files:
                break
        result = "\n".join(sorted(files))
        print(f"[Tool] Found {len(files)} files")
        return result
    except Exception as e:
        error_msg = f"Error listing files: {str(e)}"
        print(f"[Tool] {error_msg}")
        return error_msg

@tool
def logged_file_read(path: str) -> str:
    if not os.path.isabs(path):
        path = os.path.abspath(path)
    display_path = normalize_path_for_display(path)
    print(f"[Tool] file_read: {display_path}")
    if not os.path.exists(path):
        msg = f"Error: File does not exist: {display_path}"
        print(f"[Tool] {msg}")
        return msg
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = f.read()
        print(f"[Tool] Read {len(data)} chars from {display_path}")
        return data
    except Exception as e:
        msg = f"Error reading {display_path}: {e}"
        print(f"[Tool] {msg}")
        return msg

@tool
def logged_file_write(path: str, content: str) -> str:
    if not os.path.isabs(path):
        path = os.path.abspath(path)
    display_path = normalize_path_for_display(path)
    print(f"[Tool] file_write: {display_path} ({len(content)} chars)")
    try:
        if os.path.exists(path):
            msg = (
                f"Refusing to overwrite existing file: {display_path}. "
                "Use apply_unified_patch or logged_editor(path, old_str, new_str)."
            )
            print(f"[Tool] {msg}")
            return msg
        dir_path = os.path.dirname(path)
        if dir_path:
            os.makedirs(dir_path, exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        print(f"[Tool] Successfully wrote to {display_path}")
        return f"Wrote {len(content)} chars to {display_path}"
    except Exception as e:
        msg = f"Error writing {display_path}: {e}"
        print(f"[Tool] {msg}")
        return msg

@tool
def logged_editor(path: str, old_str: str, new_str: str) -> str:
    """
    Minimal targeted replacement; preserves original EOLs.
    HARD LIMITS:
      - old_str must be <= 300 chars and <= 6 lines
      - new_str must be <= 300 chars and <= 6 lines
    If larger, refuse and instruct to use apply_unified_patch.
    """
    if not os.path.isabs(path):
        path = os.path.abspath(path)
    display_path = path.replace("\\", "/")

    # --- Hard limits to prevent big, risky block rewrites
    def _line_count(s: str) -> int:
        return s.count("\n") + 1 if s else 0
    if len(old_str) > 300 or len(new_str) > 300 or _line_count(old_str) > 6 or _line_count(new_str) > 6:
        msg = ("Refusing large edit in logged_editor. Use apply_unified_patch with a small, "
               "context-rich hunk instead.")
        print(f"[Tool] {msg} ({display_path})")
        return msg

    print(f"[Tool] logged_editor: {display_path} (replace {len(old_str)} -> {len(new_str)})")
    if not os.path.exists(path):
        msg = f"Error: File does not exist: {display_path}"
        print(f"[Tool] {msg}")
        return msg

    try:
        with open(path, "r", encoding="utf-8") as f:
            original = f.read()

        # Fast-path exact match
        if old_str in original:
            updated = original.replace(old_str, new_str, 1)
        else:
            # newline-tolerant fallback
            orig_style = "\r\n" if "\r\n" in original else "\n"
            norm_original = original.replace("\r\n", "\n")
            norm_old = old_str.replace("\r\n", "\n")
            norm_new = new_str.replace("\r\n", "\n")
            if norm_old not in norm_original:
                msg = f"Error: Pattern not found in {display_path}"
                print(f"[Tool] {msg}")
                return msg
            norm_updated = norm_original.replace(norm_old, norm_new, 1)
            updated = norm_updated.replace("\n", orig_style)

        if updated == original:
            msg = f"Warning: No changes applied to {display_path}"
            print(f"[Tool] {msg}")
            return msg

        with open(path, "w", encoding="utf-8") as f:
            f.write(updated)

        print(f"[Tool] Successfully edited {display_path}")
        return f"Successfully edited {display_path}"
    except Exception as e:
        msg = f"Error editing {display_path}: {e}"
        print(f"[Tool] {msg}")
        return msg

@tool
def read_file_with_lines(path: str) -> str:
    if not os.path.isabs(path):
        path = os.path.abspath(path)
    if not os.path.exists(path):
        return f"Error: file not found: {path}"
    with open(path, "r", encoding="utf-8") as f:
        lines = f.readlines()
    return "".join(f"{i+1:>5}: {line}" for i, line in enumerate(lines))
@tool
def apply_unified_patch(patch_text: str, three_way: bool = True, allow_new_files: bool = False) -> str:
    """
    Apply unified diff with tolerant flags + diagnostics; avoids CRLF/LF churn.
    Rejects risky 'near-rewrite' patches on existing files unless explicitly allowed.
    To allow a true full rewrite, include the literal token [ALLOW_FULL_REWRITE] in the patch.
    """
    import tempfile
    repo_cwd = os.getcwd()

    def _hunk_starts_at_top(lines):
        for ln in lines:
            if not ln.startswith("@@ "):
                continue
            m = re.match(r"@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@", ln)
            if m:
                try:
                    if int(m.group(1)) <= 3:
                        return True
                except (ValueError, IndexError):
                    pass
        return False

    # Normalize patch buffer to LF (doesn't change files on disk)
    patch_text = patch_text.replace("\r\n", "\n")

    # --- Fix 3: explicit override + rejection message holder ---
    allow_full = "[ALLOW_FULL_REWRITE]" in patch_text
    reject_msg = None

    # Basic per-file analysis
    files = []
    current = {"a": None, "b": None, "lines": []}
    for ln in patch_text.splitlines(keepends=False):
        if ln.startswith("diff --git "):
            if current["a"] and current["lines"]:
                files.append(current)
            current = {"a": None, "b": None, "lines": []}
        elif ln.startswith("--- "):
            current["a"] = ln[4:].strip()
        elif ln.startswith("+++ "):
            current["b"] = ln[4:].strip()
        current["lines"].append(ln)
    if current["a"] and current["lines"]:
        files.append(current)

    for fb in files:
        a, b = fb["a"], fb["b"]

        def _strip_prefix(p):
            if not p or p == "/dev/null":
                return None
            return p[2:] if p.startswith(("a/", "b/")) else p

        target = _strip_prefix(b) or _strip_prefix(a)
        total_lines = 0
        if target and os.path.exists(target):
            with open(target, "r", encoding="utf-8", errors="ignore") as f:
                total_lines = sum(1 for _ in f)

        adds = sum(1 for l in fb["lines"] if l.startswith("+") and not l.startswith("+++"))
        dels = sum(1 for l in fb["lines"] if l.startswith("-") and not l.startswith("---"))
        hunks = sum(1 for l in fb["lines"] if l.startswith("@@ "))
        starts_top = _hunk_starts_at_top([l for l in fb["lines"] if l.startswith("@@ ")])
        ratio = (adds + dels) / max(total_lines, 1) if total_lines else 1.0

        print(f"[PatchAnalysis] {target or '<new-file>'}: lines={total_lines}, +{adds}, -{dels}, ratio={ratio:.2%}, starts_at_top={starts_top}, hunks={hunks}")

        # --- Hard guard: reject near-rewrites on existing files unless explicitly allowed ---
        if (not allow_full) and total_lines > 0 and (
            ratio >= 0.5 or  # ≥50% of lines touched
            (starts_top and hunks <= 1 and (adds + dels) >= total_lines * 0.3)  # single top hunk touching ≥30%
        ):
            reject_msg = (
                "ERROR: Patch is too large for an existing file and looks like a rewrite.\n"
                f"File: {target}\n"
                "Guidance: generate a smaller, context-rich unified diff (change only necessary lines, keep original spacing), "
                "or use logged_editor(path, old, new) for ≤6-line exact substitutions.\n"
                "If a full rewrite is truly intended, include the literal token [ALLOW_FULL_REWRITE] in the patch."
            )
            break

    # Abort early if we detected an unsafe near-rewrite
    if reject_msg:
        return reject_msg

    # Write patch to temp file
    tmpdir = tempfile.mkdtemp(prefix="devflow_patch_")
    patch_path = os.path.join(tmpdir, "change.patch")
    with open(patch_path, "w", encoding="utf-8", newline="\n") as f:
        f.write(patch_text)

    def _run(cmd):
        p = subprocess.Popen(cmd, cwd=repo_cwd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, shell=False)
        o, e = p.communicate()
        return p.returncode, o, e

    # Be explicit: avoid autocrlf rewriting during apply
    try:
        _run(["git", "config", "--local", "core.autocrlf", "false"])
    except Exception:
        pass

    cmd = ["git", "apply", "--index", "--recount", "--ignore-space-change", "--ignore-whitespace"]
    if three_way:
        cmd.append("--3way")
    cmd.append(patch_path)

    code, out_git, err_git = _run(cmd)
    if code == 0:
        return f"OK: patch applied\n{out_git}".rstrip()

    return (
        "ERROR applying patch\n"
        f"\n--- sanitized patch path ---\n{patch_path}\n"
        f"\n--- git stdout ---\n{out_git}"
        f"\n--- git stderr ---\n{err_git}"
    )


@tool
def generate_pr_body_tool(
    output_path: str,
    issue_title: str,
    summary: str,
    files_modified: str,
    technical_details: str = "",
    testing_instructions: str = "",
    additional_notes: str = ""
) -> str:
    # Always write to private, ignored path
    if not os.path.isabs(output_path):
        output_path = os.path.abspath(output_path)
    repo_root = os.getcwd()
    devflow_private = os.path.join(repo_root, ".devflow", ".pr")
    os.makedirs(devflow_private, exist_ok=True)
    output_path = os.path.join(devflow_private, ".devflow-pr-body.md")

    # Ensure ignore rules (.git/info/exclude) to keep these out of git status
    git_info_exclude = os.path.join(repo_root, ".git", "info", "exclude")
    try:
        os.makedirs(os.path.dirname(git_info_exclude), exist_ok=True)
        rules = ["/.devflow/", ".devflow/", "/.devflow/*", "*.bak"]
        existing = ""
        if os.path.exists(git_info_exclude):
            with open(git_info_exclude, "r", encoding="utf-8") as f:
                existing = f.read()
        with open(git_info_exclude, "a", encoding="utf-8") as f:
            for r in rules:
                if r not in existing:
                    f.write(r + "\n")
    except Exception:
        pass

    pr_body = f"""# 🔧 Fix: {issue_title}

## Overview
{summary}

## 📝 Changes Made

### Files Modified
{files_modified}

## 🔧 Technical Implementation
{technical_details if technical_details else "See code changes for implementation details."}

## 🧪 Testing Instructions
{testing_instructions if testing_instructions else "1. Review the code changes\n2. Run existing tests\n3. Verify the changes work as expected"}

## 📋 Additional Notes
{additional_notes if additional_notes else "No additional notes."}

---
*This PR was automatically generated by DevFlow Agent*
"""
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(pr_body)
    display_path = normalize_path_for_display(output_path)
    print(f"[Tool] PR body generated at {display_path} ({len(pr_body)} chars) [ignored from VCS]")
    return f"PR body successfully generated at: {display_path}"
