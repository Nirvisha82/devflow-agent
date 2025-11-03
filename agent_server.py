"""
DevFlow Agent Server - FastAPI wrapper for Strands agents
"""
import os
import sys
from typing import Optional, List
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from dotenv import load_dotenv
import uvicorn

from agent import create_suggestion_agent, create_automation_agent

load_dotenv()

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
        # Validate repo path
        if not os.path.exists(request.repo_path):
            raise HTTPException(
                status_code=400, 
                detail=f"Repository path does not exist: {request.repo_path}"
            )
        
        # Create suggestion agent
        agent = create_suggestion_agent(request.repo_path)
        
        # Prepare task
        task = f"""Analyze this GitHub issue and provide detailed code change suggestions:

Title: {request.issue.title}
Body: {request.issue.body}
Labels: {', '.join(request.issue.labels)}

Provide comprehensive suggestions including:
1. Detailed analysis of the issue
2. All files that need to be touched with specific actions
3. Code examples with before/after comparisons
4. Step-by-step implementation guide

Do NOT make any actual file changes. Only provide suggestions.
"""
        
        # Execute agent
        result = agent(task)
        
        # Convert to response model
        return SuggestionResponse(
            issue_title=result.issue_title,
            analysis=result.analysis,
            affected_files=[
                FileSuggestion(
                    file_path=f.file_path,
                    action=f.action,
                    reason=f.reason
                ) for f in result.affected_files
            ],
            code_examples=[
                CodeSuggestion(
                    file_path=c.file_path,
                    language=c.language,
                    before=c.before,
                    after=c.after,
                    explanation=c.explanation
                ) for c in result.code_examples
            ],
            implementation_steps=result.implementation_steps
        )
        
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Agent failed: {str(e)}")

# Automation endpoint
@app.post("/api/automate", response_model=AutomationResponse)
async def automate_changes(request: ProcessIssueRequest):
    """
    Analyze an issue and automatically make code changes to fix it.
    """
    try:
        # Validate repo path
        if not os.path.exists(request.repo_path):
            raise HTTPException(
                status_code=400, 
                detail=f"Repository path does not exist: {request.repo_path}"
            )
        
        # Create automation agent
        agent = create_automation_agent(request.repo_path)
        
        # Prepare comprehensive task with PR body instructions
        pr_body_output = os.path.join(request.repo_path, ".devflow-pr-body.md")
        
        task = f"""Process this GitHub issue and make the necessary code changes:

Title: {request.issue.title}
Body: {request.issue.body}
Labels: {', '.join(request.issue.labels)}

WORKFLOW STEPS:
1. Analyze the issue carefully and understand what needs to be fixed
2. Review repo-analysis.md and dependency-graph.json for context
3. Make all necessary code changes to fix the issue
4. Track all files you create, modify, or delete
5. Generate a comprehensive PR body that includes:
   - Clear overview of what was changed and why
   - Technical implementation details
   - List of all files modified/created/deleted with brief descriptions
   - Testing instructions for reviewers
   - Any breaking changes or important notes
   - References to the issue being resolved

CRITICAL: After completing all code changes, use the generate_pr_body_tool to create a professional PR description.
Save it to: {pr_body_output}

The PR body should be comprehensive, professional, and help reviewers understand:
- What problem was solved
- How it was solved
- What changed in the codebase
- How to verify the changes work

IMPORTANT NOTES:
- Only include actual code files in changes_made (not the PR body file)
- The pr_body_file field should contain the relative path: .devflow-pr-body.md
- Provide detailed, accurate information in your response

Return detailed information about all changes made and the location of the generated PR body.
"""
        
        # Execute agent
        result = agent(task)
        
        # Convert FileChange objects to simple string paths
        changes_list = []
        for change in result.changes_made:
            if hasattr(change, 'file_path'):
                changes_list.append(change.file_path)
            else:
                changes_list.append(str(change))
        
        # Verify PR body was created
        if result.pr_body_file:
            full_pr_path = os.path.join(request.repo_path, result.pr_body_file)
            if not os.path.exists(full_pr_path):
                print(f"Warning: PR body file not found at: {full_pr_path}")
        
        # Return response
        return AutomationResponse(
            completed=result.completed,
            success=result.success,
            changes_made=changes_list,
            summary=result.summary,
            pr_body_file=result.pr_body_file,
            error_message=result.error_message
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
    labels = request.issue.labels
    
    if 'devflow-suggestion' in labels:
        return await suggest_changes(request)
    elif 'devflow-agent-automate' in labels:
        return await automate_changes(request)
    else:
        # Default to suggestion
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