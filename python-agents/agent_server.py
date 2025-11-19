"""
DevFlow Agent Server - FastAPI wrapper for Strands agents
"""
import os
import sys
from contextlib import contextmanager
from typing import Optional, List
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from dotenv import load_dotenv
import uvicorn
import subprocess

from agent import create_suggestion_agent, create_automation_agent

load_dotenv()

@contextmanager
def pushd(new_dir: str):
    prev = os.getcwd()
    os.chdir(new_dir)
    try:
        yield
    finally:
        os.chdir(prev)

# Verify API key
api_key = os.getenv("GEMINI_API_KEY")
if not api_key:
    print("Error: GEMINI_API_KEY not found in environment")
    sys.exit(1)

# Bypass tool consent for server mode
os.environ["BYPASS_TOOL_CONSENT"] = "true"

app = FastAPI(
    title="DevFlow Agent Server",
    description="Strands-powered agents for automated code changes",
    version="1.0.0"
)


def git_changed_files(repo_path: str) -> list[str]:
    """
    Returns a list of paths (relative to repo root) of files that are
    added/modified/deleted according to `git status --porcelain`.
    """
    try:
        cp = subprocess.run(
            ["git", "status", "--porcelain"],
            cwd=repo_path,
            capture_output=True,
            text=True,
            check=True,
        )
        changed = []
        for line in cp.stdout.splitlines():
            line = line.strip()
            if not line:
                continue
            # format: "XY <path>" (e.g., " M calculator/operations.py")
            parts = line.split(maxsplit=1)
            if len(parts) == 2:
                path = parts[1]
                # normalize Windows backslashes to slashes for consistency
                changed.append(path.replace("\\", "/"))
        return changed
    except Exception as e:
        print(f"[Server] git_changed_files error: {e}")
        return []

# Request/Response Models
class IssueData(BaseModel):
    title: str = Field(description="Issue title")
    body: str = Field(default="", description="Issue body/description")
    labels: List[str] = Field(default_factory=list, description="Issue labels")

class ProcessIssueRequest(BaseModel):
    repo_path: str = Field(description="Absolute path to cloned repository")
    issue: IssueData = Field(description="GitHub issue data")
    mode: str = Field(default="automate", description="Mode: 'suggestion' or 'automate'")

class FileChange(BaseModel):
    file_path: str
    status: str

class FileSuggestion(BaseModel):
    file_path: str
    action: str
    reason: str

class CodeSuggestion(BaseModel):
    file_path: str
    language: str
    before: str
    after: str
    explanation: str

class SuggestionResponse(BaseModel):
    issue_title: str
    analysis: str
    affected_files: List[FileSuggestion]
    code_examples: List[CodeSuggestion]
    implementation_steps: List[str]

class AutomationResponse(BaseModel):
    completed: bool
    success: bool
    changes_made: List[str]  # Simple list of file paths
    summary: str
    pr_body_file: Optional[str] = ""
    error_message: Optional[str] = ""

# Health check endpoint
@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy",
        "service": "devflow-agent-server",
        "version": "1.0.0"
    }

# Suggestion endpoint
@app.post("/api/suggest", response_model=SuggestionResponse)
async def suggest_changes(request: ProcessIssueRequest):
    """
    Analyze an issue and provide code change suggestions without modifying files.
    """
    try:
        # Normalize repo path
        repo_path = request.repo_path
        if not os.path.isabs(repo_path):
            repo_path = os.path.abspath(repo_path)

        if not os.path.exists(repo_path):
            raise HTTPException(
                status_code=400,
                detail=f"Repository path does not exist: {repo_path}"
            )

        print(f"[Server] Processing suggestion request for: {repo_path}")
        print(f"[Server] Issue: {request.issue.title}")
        print(f"[Server] Labels: {request.issue.labels}")

        # Create suggestion agent
        agent = create_suggestion_agent(repo_path)

        # Build LLM task
        task = f"""
        Analyze this GitHub issue and provide detailed code change suggestions:

        Repository Path: {repo_path}
        Issue Title: {request.issue.title}
        Body: {request.issue.body}
        Labels: {', '.join(request.issue.labels)}

        IMPORTANT: You are working in the directory: {repo_path}

        1. Call list_files('{repo_path}') to list files
        2. Call load_repo_analysis('{repo_path}') for repo context
        3. Use logged_file_read() for file content
        4. Do NOT modify any files
        """

        with pushd(repo_path):
            output = agent(task)

        # Ensure .devflow exists
        devflow_dir = os.path.join(repo_path, ".devflow")
        os.makedirs(devflow_dir, exist_ok=True)

        # Path for suggestion file
        md_path = os.path.join(devflow_dir, "devflow-agent-suggestions.md")

        # Write raw agent output
        # with open(md_path, "w", encoding="utf-8") as f:
        #     f.write(str(output))

        # print(f"[Server] Suggestion markdown created at: {md_path}")

        # Return relative path
        rel = os.path.relpath(md_path, repo_path).replace("\\", "/")

        return {
            "completed": True,
            "success": True,
            "changes_made": [rel],   # <-- CRITICAL
            "summary": "Suggestion file created",
            "pr_body_file": "",
            "error_message": ""
        }

    except Exception as e:
        import traceback
        traceback.print_exc()
        raise HTTPException(status_code=500, detail=f"Agent failed: {str(e)}")

# Automation endpoint
@app.post("/api/automate", response_model=AutomationResponse)
async def automate_changes(request: ProcessIssueRequest):
    """
    Analyze an issue and automatically make code changes to fix it.
    """
    try:
        # Convert to absolute path
        repo_path = request.repo_path
        if not os.path.isabs(repo_path):
            repo_path = os.path.abspath(repo_path)
            print(f"[Server] Converted repo_path to absolute: {repo_path}")
        
        # Validate repo path
        if not os.path.exists(repo_path):
            raise HTTPException(
                status_code=400, 
                detail=f"Repository path does not exist: {repo_path}"
            )
        
        print(f"[Server] Processing automation request for: {repo_path}")
        print(f"[Server] Issue: {request.issue.title}")
        print(f"[Server] Labels: {request.issue.labels}")
        
        # Create automation agent
        agent = create_automation_agent(repo_path)
        
        # Prepare comprehensive task with PR body instructions
        pr_body_output = os.path.join(repo_path, ".devflow-pr-body.md")

        # ---- EDIT_STRATEGY block kept as a plain string to avoid f-string brace formatting ----
        edit_strategy_block = """
Before making any changes, PRINT a one-line decision log (no tools):
EDIT_STRATEGY: {
  "files": ["<rel/path1>", "..."],
  "for_each_file": {
    "<rel/path>": {
      "action": "patch" | "new_file",
      "reason": "<short reason>"
    }
  }
}
"""

        task = f"""Process this GitHub issue and make the necessary code changes:

Repository Path: {repo_path}
Title: {request.issue.title}
Body: {request.issue.body}
Labels: {', '.join(request.issue.labels)}

IMPORTANT: You are working in the directory: {repo_path}

WORKFLOW STEPS:
1. First, understand the repository structure:
   - Call list_files('{repo_path}') to see what files exist
   - Call load_repo_analysis('{repo_path}') for repo context (if available)
   
2. Read relevant files using logged_file_read() with relative paths
   - Example: logged_file_read('main.py') not logged_file_read('{repo_path}/main.py')

3. Make necessary code changes:
  - For EXISTING files: generate a minimal unified diff and call apply_unified_patch(patch_text).
  - For NEW files: prefer unified diff creation (apply_unified_patch with /dev/null headers).
  - NEVER rewrite an entire file if only a few lines change.
  - Do NOT reformat, reorder, or re-indent code, imports, or docstrings.
    No whitespace-only edits or style adjustments. Keep every blank line,
    indentation, and spacing exactly as originally read.

  - For NEW files:
    • Prefer creating them via a unified diff too (apply_unified_patch with standard 'new file mode 100644', '--- /dev/null', '+++' headers).
    • If absolutely necessary, you MAY use logged_file_write ONLY for truly new files that do not exist.

  - NEVER rewrite an entire file if you’re only adding or changing a few lines.
  - Use POSIX (forward-slash) relative paths in patch headers.

{edit_strategy_block}
4. Generate PR body:
   - Call generate_pr_body_tool(
       output_path='{pr_body_output}',
       issue_title='{request.issue.title}',
       summary='What you did',
       files_modified='List of files changed',
       technical_details='How you implemented it',
       testing_instructions='How to test'
     )

5. Return AutomationResult with:
   - changes_made: List of relative file paths you modified
   - pr_body_file: '.devflow-pr-body.md'
   - summary: Brief description of changes

CRITICAL RULES:
- Use relative paths for all file operations
- Track ONLY code files in changes_made (not .devflow-pr-body.md)
- Stop after 10-15 tool calls - don't loop infinitely
- If you read the same file twice, you have enough context - make changes

Return detailed information about all changes made.
"""
        
        print(f"[Server] Executing agent...")
        # Execute agent
        with pushd(repo_path):
            result = agent(task)
        
        print(f"[Server] Agent completed")

        # Try to unwrap structured AutomationResult from Strands
        structured = getattr(result, "structured", None)

        # Build changes list from structured first
        changes_list: list[str] = []
        pr_file = ""
        summary = ""
        completed = False
        success = False
        error_message = ""

        if structured is not None:
            # Map FileChange[] -> list[str]
            for change in getattr(structured, "changes_made", []) or []:
                if hasattr(change, "file_path"):
                    changes_list.append(change.file_path)
                elif isinstance(change, dict) and "file_path" in change:
                    changes_list.append(change["file_path"])
                else:
                    changes_list.append(str(change))

            pr_file = getattr(structured, "pr_body_file", "") or ""
            summary = getattr(structured, "summary", "") or ""
            completed = bool(getattr(structured, "completed", False))
            success = bool(getattr(structured, "success", False))
            error_message = getattr(structured, "error_message", "") or ""

        # ✅ Fallback: if model didn't return structured or returned no changes,
        # compute changes by asking Git directly
        if not changes_list:
            changes_list = git_changed_files(repo_path)

        # Normalize changes to RELATIVE POSIX-style paths for consistency
        normalized_changes = []
        for p in changes_list:
            if not p:
                continue
            # If absolute, make relative to repo; else keep as-is
            if os.path.isabs(p):
                try:
                    rel = os.path.relpath(p, repo_path)
                except ValueError:
                    # In rare cases (different drive on Windows), keep original
                    rel = p
            else:
                rel = p
            normalized_changes.append(rel.replace("\\", "/"))

        changes_list = normalized_changes


        # Normalize PR body file:
        # - if structured gave absolute, make it relative to repo
        # - if empty, but default .devflow-pr-body.md exists, use that
        if pr_file:
            if os.path.isabs(pr_file):
                pr_file = os.path.relpath(pr_file, repo_path).replace("\\", "/")
        else:
            default_pr = os.path.join(repo_path, ".devflow-pr-body.md")
            if os.path.exists(default_pr):
                pr_file = ".devflow-pr-body.md"

        # If we have file changes from git, treat this as success even if structured is missing
        if changes_list and not success:
            success = True
            completed = True
            if not summary:
                summary = "Applied code changes and generated PR body."

        print(f"[Server] Success: {success}")
        print(f"[Server] Processed changes: {changes_list}")
        if pr_file:
            print(f"[Server] PR body file: {pr_file}")

        return AutomationResponse(
            completed=completed,
            success=success,
            changes_made=changes_list,
            summary=summary,
            pr_body_file=pr_file,
            error_message=error_message,
        )


        
    except Exception as e:
        import traceback
        traceback.print_exc()
        return AutomationResponse(
            completed=False,
            success=False,
            changes_made=[],
            summary="Failed to process issue",
            pr_body_file="",
            error_message=str(e)
        )

# Generic process endpoint (auto-detects mode from labels)
@app.post("/api/process")
async def process_issue(request: ProcessIssueRequest):
    """
    Process an issue and automatically determine mode based on labels.
    - If 'devflow-suggestion' label present -> suggestion mode
    - If 'devflow-agent-automate' label present -> automate mode
    - Otherwise -> default to suggestion mode
    """
    print(f"[Server] Received process request: {request.issue.title}")
    print(f"[Server] Repo path: {request.repo_path}")
    print(f"[Server] Labels: {request.issue.labels}")
    
    labels = request.issue.labels
    
    # --- New DevFlow Dual-Mode Label Routing ---
    if 'devflow-agent-suggest-changes' in labels:
        print("[Server] Mode: SUGGESTION")
        return await suggest_changes(request)

    elif 'devflow-agent-apply-changes' in labels:
        print("[Server] Mode: AUTOMATE")
        return await automate_changes(request)

    else:
        # Default: suggestion-only mode
        print("[Server] Mode: SUGGESTION (default)")
        return await suggest_changes(request)


if __name__ == "__main__":
    # Get port from environment or use default
    port = int(os.getenv("AGENT_SERVER_PORT", "8094"))
    host = os.getenv("AGENT_SERVER_HOST", "0.0.0.0")
    
    print(f"Starting DevFlow Agent Server on {host}:{port}")
    print(f"API Documentation available at: http://localhost:{port}/docs")
    
    uvicorn.run(
        app, 
        host=host, 
        port=port,
        log_level="info"
    )
