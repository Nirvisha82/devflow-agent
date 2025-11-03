import os
from typing import List
from strands import Agent
from strands_tools import file_read, file_write, editor
from strands.models.gemini import GeminiModel
from dotenv import load_dotenv
import sys
load_dotenv()

api_key=os.getenv("GEMINI_API_KEY")
if not api_key:
        print("Error: GEMINI_API_KEY not found in environment")
        sys.exit(1)

model = GeminiModel(
    client_args={
        "api_key": api_key,
    },
    model_id="gemini-2.5-flash",
    params={ 
        "temperature": 0.5,
        "max_output_tokens": 8092,
        "top_p": 0.9,
        "top_k": 40
    }
)

def list_files(repo_path: str) -> str:
    """List all files in the repository."""
    files = []
    ignore_patterns = ['.git', 'node_modules', '__pycache__', '.venv', 'venv', 'dist', 'build']
    
    for root, _, filenames in os.walk(repo_path):
        if any(pattern in root for pattern in ignore_patterns):
            continue
        for filename in filenames:
            rel_path = os.path.relpath(os.path.join(root, filename), repo_path)
            files.append(rel_path)
    
    return "\n".join(files[:100])  # Limit to first 100 files


def create_devflow_agent(repo_path: str) -> Agent:
    """Create a DevFlow agent with file operation tools."""
    SYS_PROMPT=f"""You are a code generation agent. Your job is to:
        1. Analyze GitHub issues
        2. Understand the repository structure
        3. Generate and apply code changes to fix the issue

        Repository path: {repo_path}
        You must also look at these files and decide what needs to be done: 
        {repo_path}/.devflow/repo-analysis.md
        {repo_path}/.devflow/dependency-graph.json

        STRICT REQUIREMENT:
        Always return a summary of what you did at the end.
        You must also tell what steps you are taking.
        """
    
    # Create agent with system prompt
    agent = Agent(
        name="DevFlowAgent",
        model=model,
        tools=[file_write,file_read,editor],
        system_prompt=SYS_PROMPT
        )
    
    
    return agent
