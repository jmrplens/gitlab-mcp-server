---
name: create-mcp-evaluation
description: 'Create evaluation Q&A pairs to test MCP server quality. Use when validating how well an MCP server enables LLMs to accomplish real tasks, benchmarking tool design, or measuring server effectiveness through realistic question-answer evaluations.'
---

# MCP Server Evaluation Creator

Create comprehensive evaluations to test whether LLMs can effectively use your MCP server to answer realistic, complex questions using only the tools provided.

## Overview

The quality of an MCP server is measured by how well its implementations (input/output schemas, descriptions, functionality) enable LLMs with no other context to answer realistic and difficult questions.

## Process

### Step 1: Tool Inspection

1. List all tools available in the MCP server
2. Understand input/output schemas, descriptions, and annotations
3. Identify read-only vs. write operations
4. Note pagination capabilities and limits

### Step 2: Content Exploration

1. Use READ-ONLY tools to explore available data
2. Identify stable data points that won't change over time
3. Map relationships between resources (projects → issues → comments → users)
4. Note interesting patterns, edge cases, and complex relationships

### Step 3: Question Design

Create 10 evaluation questions following these requirements:

#### Core Rules

- Questions MUST be independent (no dependency on other answers)
- Questions MUST require ONLY read-only, non-destructive operations
- Questions MUST be realistic — tasks humans with LLM assistance would care about
- Each answer MUST be a single, verifiable value (string comparison)
- Answers MUST be stable (won't change over time)

#### Complexity Guidelines

- Require multiple tool calls (potentially dozens)
- Multi-hop: answer depends on chaining information from multiple queries
- Require deep exploration, not surface-level keyword search
- Use synonyms and paraphrases, not direct keywords from target content
- May require extensive pagination through results
- Should stress-test tool return values across data modalities

**Answer Diversity**
Cover diverse answer types:

- Names (user, project, group)
- IDs (project ID, issue IID)
- URLs and paths
- Timestamps and dates (specify format in question)
- Counts and quantities
- Boolean (True/False)
- Status values

### Step 4: Verification

For each question:

1. Solve it yourself using only the MCP tools
2. Verify the answer is correct and stable
3. Confirm it requires multiple tool calls
4. Ensure no write operations are needed

## Output Format

```xml
<evaluation>
  <qa_pair>
    <question>Find the GitLab project that contains a CI/CD pipeline with a job named "deploy-prod". What is the default branch of that project? Answer with the exact branch name.</question>
    <answer>main</answer>
  </qa_pair>
  <qa_pair>
    <question>In the project with ID 42, find the merge request that was merged on 2024-01-15. Who authored it? Answer with the username.</question>
    <answer>jdoe</answer>
  </qa_pair>
  <!-- 8 more qa_pairs -->
</evaluation>
```

## Example Question Patterns for GitLab MCP

1. **Cross-resource lookup**: "Find the issue in project X that mentions Y. Who was assigned to it?"
2. **Multi-hop navigation**: "Find the MR that fixed issue #N. What pipeline job failed first in that MR?"
3. **Aggregation**: "How many open issues with label 'bug' exist across all projects in group Z?"
4. **Temporal reasoning**: "What was the last commit to the develop branch of project X before 2024-06-01? Answer with the short SHA."
5. **Relationship tracing**: "Find the user who created the most merge requests in project X during 2024. Answer with their username."

## Quality Checklist

- [ ] 10 questions created
- [ ] All questions use only read-only operations
- [ ] All questions are independent
- [ ] All answers verified manually
- [ ] Answers are stable (won't change)
- [ ] Answers use direct string comparison
- [ ] Questions cover diverse tool usage (projects, issues, MRs, pipelines, users)
- [ ] Questions require multi-step reasoning
- [ ] No questions solvable with a single tool call
- [ ] Output format matches XML schema
