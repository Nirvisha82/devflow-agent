import os
import json
from models import *
from strands import Agent
from tools import *
from strands.models.gemini import GeminiModel
from strands.models.anthropic import AnthropicModel
from dotenv import load_dotenv
from prompts import AUTO_SYS_PROMPT, SUG_SYS_PROMPT
import sys
load_dotenv()

# Check which API keys are available
gemini_api_key = os.getenv("GEMINI_API_KEY")
anthropic_api_key = os.getenv("ANTHROPIC_API_KEY")
use_anthropic = True

if not gemini_api_key and not anthropic_api_key:
    print("Error: No API keys found. Set GEMINI_API_KEY or ANTHROPIC_API_KEY")
    sys.exit(1)


# Initialize the model based on environment
if use_anthropic and anthropic_api_key:
    print("[Agent] Using Anthropic Claude Haiku 4.5")
    model = AnthropicModel(
        client_args={
            "api_key": anthropic_api_key,
        },
        max_tokens=8192,
        model_id="claude-haiku-4-5-20251001",
        params={
            "temperature": 0.5,
        }
    )
elif gemini_api_key:
    print("[Agent] Using Google Gemini 2.5 Flash")
    model = GeminiModel(
        client_args={
            "api_key": gemini_api_key,
        },
        model_id="gemini-2.5-flash",
        params={ 
            "temperature": 0.5,
            "max_output_tokens": 8092,
            "top_p": 0.9,
            "top_k": 40
        }
    )
else:
    print("Error: No valid API key configuration found")
    sys.exit(1)


def create_suggestion_agent(repo_path: str) -> Agent:
    """Create a DevFlow agent for generating issue suggestions."""
    
    # Convert to absolute path and change working directory
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)
    
    print(f"[Agent] Working with repository: {repo_path}")
    print(f"[Agent] Current working directory: {os.getcwd()}")
    
    # Change to repo directory so relative paths work
    original_cwd = os.getcwd()
    try:
        os.chdir(repo_path)
        print(f"[Agent] Changed working directory to: {os.getcwd()}")
    except Exception as e:
        print(f"[Agent] Warning: Could not change to repo directory: {e}")
    
    # Build system prompt with repo context
    system_prompt = f"""{SUG_SYS_PROMPT}

REPOSITORY CONTEXT:
- Repository Path: {repo_path}
- Working Directory: {repo_path}
- Use load_repo_analysis('{repo_path}') to get repo analysis
- Use load_dependency_graph('{repo_path}') to get dependency info
- Use list_files('{repo_path}') to see available files
- All file paths should be relative to {repo_path}
"""
    
    agent = Agent(
        name="DevFlowSuggestionAgent",
        model=model,
        tools=[logged_file_read, load_repo_analysis, load_dependency_graph, list_files],
        system_prompt=system_prompt,
        structured_output_model=IssueSuggestion,
    )
    
    return agent


def create_automation_agent(repo_path: str) -> Agent:
    """Create a DevFlow agent for automating code changes."""
    
    # Convert to absolute path and change working directory
    if not os.path.isabs(repo_path):
        repo_path = os.path.abspath(repo_path)
    
    print(f"[Agent] Working with repository: {repo_path}")
    print(f"[Agent] Current working directory: {os.getcwd()}")
    
    # Change to repo directory so relative paths work
    original_cwd = os.getcwd()
    try:
        os.chdir(repo_path)
        print(f"[Agent] Changed working directory to: {os.getcwd()}")
    except Exception as e:
        print(f"[Agent] Warning: Could not change to repo directory: {e}")
    
    # Build system prompt with repo context
    system_prompt = f"""{AUTO_SYS_PROMPT}

REPOSITORY CONTEXT:
- Repository Path: {repo_path}
- Working Directory: {repo_path}
- Use load_repo_analysis('{repo_path}') to get repo analysis
- Use load_dependency_graph('{repo_path}') to get dependency info  
- Use list_files('{repo_path}') to see available files
- When reading/writing files, use relative paths from {repo_path}
- For PR body: use generate_pr_body_tool('{repo_path}/.devflow-pr-body.md', ...)

CRITICAL FILE PATH RULES:
- For logged_file_read: Use relative paths like "main.py" or "src/utils.py"
- For logged_file_write: Use relative paths like "new_file.py"
- For logged_editor: Use relative paths like "existing_file.py"
- The tools will automatically resolve to absolute paths
"""

    agent = Agent(
        name="DevFlowAutomationAgent",
        model=model,
        tools=[
            logged_file_read, 
            logged_file_write, 
            logged_editor, 
            generate_pr_body_tool,
            load_repo_analysis,
            load_dependency_graph,
            list_files
        ],
        system_prompt=system_prompt,
        structured_output_model=AutomationResult,
    )
    
    return agent