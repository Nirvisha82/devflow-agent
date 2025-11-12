"""
DevFlow Strands Agents - Agent Creation Module
"""
import os
import sys
from strands import Agent
from strands.models.gemini import GeminiModel
from strands.models.anthropic import AnthropicModel
from dotenv import load_dotenv

from models import IssueSuggestion, AutomationResult
from prompts import AUTO_SYS_PROMPT, SUG_SYS_PROMPT
from tools import (
    # context / repo reading
    logged_file_read,
    load_repo_analysis,
    load_dependency_graph,
    list_files,

    # patch-based editing tools
    read_file_with_lines,
    apply_unified_patch,

    # precise tiny-edit tool (our own; no .bak files)
    logged_editor,

    # PR body generation
    generate_pr_body_tool,
)

load_dotenv()

gemini_api_key = os.getenv("GEMINI_API_KEY")
anthropic_api_key = os.getenv("ANTHROPIC_API_KEY")
use_anthropic = True

if not gemini_api_key and not anthropic_api_key:
    print("Error: No API keys found. Set GEMINI_API_KEY or ANTHROPIC_API_KEY")
    sys.exit(1)

if use_anthropic and anthropic_api_key:
    print("[Agent] Using Anthropic Claude Haiku 4.5")
    model = AnthropicModel(
        client_args={"api_key": anthropic_api_key},
        max_tokens=8192,
        model_id="claude-haiku-4-5-20251001",
        params={"temperature": 0.5},
    )
elif gemini_api_key:
    print("[Agent] Using Google Gemini 2.5 Flash")
    model = GeminiModel(
        client_args={"api_key": gemini_api_key},
        model_id="gemini-2.5-flash",
        params={"temperature": 0.5, "max_output_tokens": 8092, "top_p": 0.9, "top_k": 40},
    )
else:
    print("Error: No valid API key configuration found")
    sys.exit(1)


def create_suggestion_agent(repo_path: str) -> Agent:
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)

    print(f"[Agent] Creating suggestion agent for: {repo_path}")
    system_prompt = SUG_SYS_PROMPT.format(repo_path=repo_path)

    agent = Agent(
        name="DevFlowSuggestionAgent",
        model=model,
        tools=[
            logged_file_read,
            load_repo_analysis,
            load_dependency_graph,
            list_files,
        ],
        system_prompt=system_prompt,
        structured_output_model=IssueSuggestion,
    )

    print("[Agent] Suggestion agent created")
    return agent


def create_automation_agent(repo_path: str) -> Agent:
    """
    Automation agent.

    RULES
    - Existing files: use apply_unified_patch for multi-line edits.
    - Tiny ≤5-line substitutions: use logged_editor(path, old, new).
    - New files: prefer unified diff creation via apply_unified_patch.
    - Never create backup files (*.bak). Never rewrite entire files for small changes.
    """
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)

    print(f"[Agent] Creating automation agent for: {repo_path}")
    system_prompt = AUTO_SYS_PROMPT.format(repo_path=repo_path)

    tools = [
        # Context / repo discovery
        list_files,
        load_repo_analysis,
        load_dependency_graph,
        logged_file_read,
        read_file_with_lines,

        # Editing
        apply_unified_patch,
        logged_editor,  # <— our precise editor (no backups)

        # PR content
        generate_pr_body_tool,
    ]

    try:
        tool_names = [getattr(t, "__name__", str(t)) for t in tools]
    except Exception:
        tool_names = ["<unknown_tool>" for _ in tools]
    print("[Agent] Automation tools:", tool_names)

    agent = Agent(
        name="DevFlowAutomationAgent",
        model=model,
        tools=tools,
        system_prompt=system_prompt,
        structured_output_model=AutomationResult,
    )

    print("[Agent] Automation agent created")
    return agent
