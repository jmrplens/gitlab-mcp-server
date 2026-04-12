---
name: update-project-documentation
description: 'Update existing project documentation to maintain parity with source code changes. Analyzes code diffs, identifies documentation gaps, and surgically updates affected documents while preserving structure and style.'
---

# Update Project Documentation

## Primary Directive

Update existing documentation in the `docs/` directory to reflect current source code changes. Perform a delta analysis between the implementation and documentation, then surgically update only the affected sections while preserving the overall document structure, style, and formatting.

## Execution Context

This skill is triggered after code changes to ensure documentation stays in sync. It focuses on efficiency — only updating what has changed rather than regenerating entire documents.

## Analysis Phase

### Step 1: Identify Changed Code

1. Check recent changes using git diff or by examining the files provided
2. If no specific files are indicated, scan all Go source files for modifications
3. Build a list of changed exported types, functions, constants, and configurations
4. Identify new, modified, or removed public APIs

### Step 2: Map Changes to Documentation

For each code change, identify which documentation files are affected:

| Change Type | Affected Documentation |
|-------------|----------------------|
| New exported type/function | Package docs, possibly tools/resources reference |
| Modified function signature | Package docs, tools reference, examples |
| New MCP tool | `docs/tools/README.md`, package docs |
| New MCP resource | `docs/resources-reference.md`, package docs |
| New MCP prompt | `docs/prompts-reference.md`, package docs |
| Configuration change | `docs/configuration.md` |
| New package | `docs/README.md` index, new package doc |
| Architecture change | `docs/architecture.md`, diagrams |
| Build/deploy change | `docs/development/development.md`, `docs/deployment.md` |
| Removed API | All referencing documents |

### Step 3: Assess Impact

For each affected document:

1. Read the current documentation
2. Compare with the current source code
3. Classify the update as: **Minor** (parameter change), **Moderate** (new section), or **Major** (restructure)
4. Prioritize Critical and High priority documents first

## Update Strategy

### Principles

- **UPD-001**: Preserve existing document structure, heading hierarchy, and formatting style
- **UPD-002**: Use surgical edits — replace only the changed sections, not entire files
- **UPD-003**: Maintain cross-reference integrity — check all links still work
- **UPD-004**: Update Mermaid diagrams if component relationships changed
- **UPD-005**: Update tables (parameters, types, functions) to match source
- **UPD-006**: Add deprecation notices for removed APIs rather than deleting immediately
- **UPD-007**: Never introduce TBD/TODO placeholders in updates
- **UPD-008**: Maintain consistent terminology with the rest of the documentation

### For New APIs

1. Add new entries to the appropriate reference document tables
2. Add new sections following the existing document pattern and style
3. Update the package documentation if the API belongs to an existing package
4. Add cross-references from related documents
5. Update the documentation index if new documents are created

### For Modified APIs

1. Update parameter tables with new/changed/removed parameters
2. Update return type documentation if changed
3. Update code examples to reflect the new signature
4. Add migration notes if the change is breaking
5. Update Mermaid diagrams if type relationships changed

### For Removed APIs

1. Mark the API as deprecated with a notice and removal version/date
2. Suggest the replacement API if one exists
3. Update cross-references that pointed to the removed API
4. After one release cycle, fully remove the deprecated section

### For Configuration Changes

1. Update the environment variables table
2. Update default values and descriptions
3. Add migration instructions if existing users need to change their config
4. Update deployment documentation if the change affects deployment

## Parity Verification

After all updates are applied, perform a parity check:

### For Each Updated Document

1. Read the updated documentation
2. Compare every documented API, parameter, and type against source code
3. Verify all code examples are syntactically valid
4. Confirm Mermaid diagrams reflect current architecture
5. Check all cross-reference links resolve correctly

### Verification Checklist

- [ ] All changed exported types/functions are documented correctly
- [ ] Parameter tables match current function signatures
- [ ] Return types and error handling match implementation
- [ ] Code examples compile and reflect current API
- [ ] Mermaid diagrams reflect current component relationships
- [ ] Cross-reference links are valid
- [ ] No TBD/TODO placeholders in updated sections
- [ ] Consistent terminology and style with surrounding content
- [ ] Deprecation notices added for removed APIs

## Output Format

After completing updates, provide a summary:

```markdown
## Documentation Update Summary

### Documents Updated
| Document | Sections Changed | Change Type |
|----------|-----------------|-------------|
| `docs/tools/README.md` | Added `gitlab_new_tool` section | New API |
| `docs/packages/tools.md` | Updated exported functions table | Modified API |

### Parity Status
- [x] All changes documented
- [x] Examples validated
- [x] Diagrams updated
- [x] Cross-references verified

### Notes
[Any observations, recommendations for follow-up, or areas needing manual review]
```

## Error Handling

- **ERR-001**: Source file not found — report the expected path and skip
- **ERR-002**: Documentation file not found — recommend running `generate-project-documentation` skill first
- **ERR-003**: Ambiguous change — document both interpretations and flag for manual review
- **ERR-004**: Breaking change detected — add prominent migration notice with before/after examples
- **ERR-005**: Diagram rendering failure — provide the Mermaid source and flag for manual validation
