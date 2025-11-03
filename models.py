

from typing import List, Optional
from pydantic import BaseModel, Field
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
