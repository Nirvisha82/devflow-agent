import os
import json
from typing import List, Optional
from pydantic import BaseModel, Field
from strands import Agent, tool
from strands_tools import file_read, file_write, editor
from strands.models.gemini import GeminiModel
from dotenv import load_dotenv
import sys
load_dotenv()

api_key=os.getenv("GEMINI_API_KEY")
if not api_key:
        print("Error: GEMINI_API_KEY not found in environment")
        sys.exit(1)

# Configuration for PR template path
PR_TEMPLATE_PATH = os.getenv("PR_TEMPLATE_PATH", "/app/templates/pr-body-template.md")

# Define structured output schema for devflow-suggestion
class FileSuggestion(BaseModel):
    file_path: str = Field(description="Path to the file to be modified or created")
    action: str = Field(description="Either 'create', 'modify', or 'delete'")
    reason: str = Field(description="Why this file needs to be touched")

class CodeSuggestion(BaseModel):
    file_path: str = Field(description="File where the code change applies")
    language: str = Field(description="Programming language of the file")
    before: str = Field(description="Current code snippet (if modifying)")
    after: str = Field(description="Suggested code change")
    explanation: str = Field(description="Explanation of the change")

class IssueSuggestion(BaseModel):
    issue_title: str = Field(description="Title of the issue")
    analysis: str = Field(description="Detailed analysis of the issue")
    affected_files: List[FileSuggestion] = Field(description="List of files to be touched")
    code_examples: List[CodeSuggestion] = Field(description="Specific code examples")
    implementation_steps: List[str] = Field(description="Step-by-step implementation guide")

# Define structured output schema for devflow-automate
class FileChange(BaseModel):
    file_path: str = Field(description="Path to the file changed")
    status: str = Field(description="Either 'created', 'modified', or 'deleted'")

class AutomationResult(BaseModel):
    completed: bool = Field(description="Whether the automation is complete")
    success: bool = Field(description="Whether the automation was successful")
    changes_made: List[FileChange] = Field(description="List of files changed")
    summary: str = Field(description="Summary of what was done")
    pr_body_file: Optional[str] = Field(default="", description="Path to generated PR body markdown file (relative to repo root)")
    error_message: Optional[str] = Field(default="", description="Any error messages")

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

def load_repo_analysis(repo_path: str) -> str:
    """Load repository analysis from .devflow directory."""
    analysis_file = os.path.join(repo_path, ".devflow", "repo-analysis.md")
    if os.path.exists(analysis_file):
        with open(analysis_file, 'r') as f:
            return f.read()
    return "No repo analysis available"

def load_dependency_graph(repo_path: str) -> str:
    """Load dependency graph from .devflow directory."""
    dep_file = os.path.join(repo_path, ".devflow", "dependency-graph.json")
    if os.path.exists(dep_file):
        with open(dep_file, 'r') as f:
            return f.read()
    return "No dependency graph available"

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
    
    return "\n".join(files[:100])


@tool
def generate_pr_body_tool(template_path: str, output_path: str, **variables) -> str:
    """
    Generate PR body from template and save to markdown file.
    This is a Strands tool that will be available to the agent.
    
    Args:
        template_path: Path to the PR body template file
        output_path: Where to save the generated PR body (should be in repo root)
        **variables: Key-value pairs to replace in template
    
    Returns:
        Success message with output path
    """
    try:
        # Read template
        with open(template_path, 'r') as f:
            template = f.read()
        
        # Replace all variables
        for key, value in variables.items():
            placeholder = "{" + key + "}"
            template = template.replace(placeholder, str(value))
        
        # Ensure output directory exists
        os.makedirs(os.path.dirname(output_path), exist_ok=True)
        
        # Save to output
        with open(output_path, 'w') as f:
            f.write(template)
        
        return f"PR body successfully generated at: {output_path}"
    except Exception as e:
        return f"Error generating PR body: {str(e)}"


def create_suggestion_agent(repo_path: str) -> Agent:
    """Create a DevFlow agent for generating issue suggestions."""
    repo_analysis = load_repo_analysis(repo_path)
    dependency_graph = load_dependency_graph(repo_path)
    
    SYS_PROMPT = f"""You are a code analysis agent optimized for Gemini Flash 2.5. Your job is to:
            1. Analyze GitHub issues thoroughly
            2. Review the repository structure and architecture
            3. Generate detailed code change suggestions without modifying files

            REPOSITORY CONTEXT:
            Path: {repo_path}
            Repository Analysis:
            {repo_analysis}

            Dependency Graph:
            {dependency_graph}

            WORKFLOW (devflow-suggestion):
            Step 1: Read and understand the issue thoroughly
            Step 2: Analyze the repo structure and existing code
            Step 3: Identify all files that need to be touched (create/modify/delete)
            Step 4: Generate detailed code examples for each change
            Step 5: Provide step-by-step implementation guide

            OUTPUT REQUIREMENT:
            You MUST respond with JSON matching the IssueSuggestion schema.
            Do NOT make any actual file changes - only provide suggestions.
            Always analyze repo-analysis.md and dependency-graph.json first.
            """
    
    agent = Agent(
        name="DevFlowSuggestionAgent",
        model=model,
        tools=[file_read],
        system_prompt=SYS_PROMPT,
        structured_output_model=IssueSuggestion
    )
    
    return agent


def create_automation_agent(repo_path: str) -> Agent:
    """Create a DevFlow agent for automating code changes."""
    repo_analysis = load_repo_analysis(repo_path)
    dependency_graph = load_dependency_graph(repo_path)
    pr_body_output = os.path.join(repo_path, ".devflow-pr-body.md")
    
    SYS_PROMPT = f"""You are a code automation agent optimized for Gemini Flash 2.5. Your job is to:
            1. Analyze GitHub issues thoroughly
            2. Review the repository structure and architecture
            3. Make actual code changes to fix the issue
            4. Generate a comprehensive PR body for the changes

            REPOSITORY CONTEXT:
            Path: {repo_path}
            Repository Analysis:
            {repo_analysis}

            Dependency Graph:
            {dependency_graph}

            WORKFLOW (devflow-automate):
            Step 1: Read and understand the issue thoroughly
            Step 2: Analyze the repo structure using repo-analysis.md and dependency-graph.json
            Step 3: Identify all files that need to be touched
            Step 4: Create new files or modify existing files as needed
            Step 5: Verify changes maintain code consistency
            Step 6: Generate a comprehensive PR body using the generate_pr_body_tool
            Step 7: Track all changes and generate summary

            IMPLEMENTATION GUIDELINES:
            - Always check repo-analysis.md and dependency-graph.json before making changes
            - Maintain code style consistency with existing codebase
            - Update dependencies if new packages are required
            - Keep changes minimal and focused on the issue
            - Test implications of changes on dependent files
            - Use descriptive commit messages and PR descriptions

            PR BODY GENERATION:
            After making all code changes, you MUST generate a comprehensive PR body using the generate_pr_body_tool.

            CRITICAL: Use these exact parameters for generate_pr_body_tool:
            - template_path: "{PR_TEMPLATE_PATH}"
            - output_path: "{pr_body_output}"
            - Then pass all required variables as keyword arguments

            The PR body should include:
            - Overview of changes
            - Technical implementation details
            - Files modified/created/deleted
            - Testing instructions
            - Any breaking changes or migration notes
            - Links to related issues

            Example tool call:
            generate_pr_body_tool(
                template_path="{PR_TEMPLATE_PATH}",
                output_path="{pr_body_output}",
                issue_number="123",
                issue_title="Fix authentication",
                summary="Fixed JWT validation...",
                files_modified="- auth/jwt.go: Updated validation\\n- go.mod: Updated deps",
                technical_details="Migrated to golang-jwt/jwt/v5...",
                testing_instructions="1. Run tests\\n2. Manual testing...",
                additional_notes="No breaking changes"
            )

            IMPORTANT: The PR body file (.devflow-pr-body.md) should NOT be included in changes_made list.
            Only actual code files that resolve the issue should be in changes_made.

            OUTPUT REQUIREMENT:
            You MUST respond with JSON matching the AutomationResult schema.
            Track all file changes with their status (created/modified/deleted).
            Include the pr_body_file path (relative to repo root) in your response: ".devflow-pr-body.md"
            Provide a comprehensive summary of implementation.
            """
    
    agent = Agent(
        name="DevFlowAutomationAgent",
        model=model,
        tools=[file_read, file_write, editor, generate_pr_body_tool],
        system_prompt=SYS_PROMPT,
        structured_output_model=AutomationResult
    )
    
    return agent