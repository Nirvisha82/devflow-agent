import os
import sys
import json
from dotenv import load_dotenv
from agent import create_devflow_agent
os.environ["BYPASS_TOOL_CONSENT"] = "true"

load_dotenv()

def main():
    
    if len(sys.argv) < 3:
        print("Usage: python main.py <repo_path> <issue_json>")
        sys.exit(1)
    
    repo_path = sys.argv[1]
    issue_json = sys.argv[2]
    
    if not os.path.exists(repo_path):
        print(f"Error: Repository path does not exist: {repo_path}")
        sys.exit(1)
    
    try:
        issue_data = json.loads(issue_json)
    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON: {e}")
        sys.exit(1)
    
    issue_title = issue_data.get('title', 'N/A')
    issue_body = issue_data.get('body', '')
    labels = issue_data.get('labels', [])
    
    print(f"Starting DevFlow Agent")
    print(f"Repository: {repo_path}")
    print(f"Issue: {issue_title}")
    print("-" * 50)
    
    agent = create_devflow_agent(repo_path)
    
    task = f"""Process this GitHub issue and make the necessary code changes:

Title: {issue_title}
Body: {issue_body}
Labels: {', '.join(labels)}

Return your response as JSON with:
- completed: true/false
- success: true/false
- changes_made: list of file paths you modified
- summary: description of what you did
- error_message: any errors (empty string if none)
"""
    
    try:
        print("[Agent] Processing issue...")
        response = agent(task)
        
        print("-" * 50)
        print("[Agent] Response:")
        print(response)
        
        try:
            result = json.loads(response)
        except:
            result = {
                "completed": True,
                "success": True,
                "changes_made": [],
                "summary": response,
                "error_message": ""
            }
        
        print("\n=== JSON RESULT ===")
        print(json.dumps(result, indent=2))
            
    except Exception as e:
        print(f"Error running agent: {e}")
        result = {
            "completed": True,
            "success": False,
            "changes_made": [],
            "summary": "Failed to process issue",
            "error_message": str(e)
        }
        print("\n=== JSON RESULT ===")
        print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
