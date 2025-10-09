# TigerData MCP Feedback

## Overall Assessment
The TigerData MCP tools provide useful database management functionality but have several areas for improvement in naming consistency, parameter documentation, and descriptions.

## Tool Names

### Issues
1. **Inconsistent naming conventions**: Mixed use of camelCase (`getGuide`) and snake_case (`service_list`, `semantic_search`) creates unpredictability
2. **Verb consistency**: Some tools use action verbs (`create`, `list`) while others use descriptive terms (`show` instead of `get`)

### Recommendations
- **WHY**: Consistency reduces cognitive load and makes the API more predictable
- Standardize on snake_case throughout: `get_guide`, `semantic_search_postgres_docs`
- ~~Use consistent verbs: `service_get` instead of `service_show` for alignment with REST conventions~~ ✅ COMPLETED

## Input Parameters

### `mcp__tigerdata__getGuide`
**Issue**: Parameter `prompt_name` is misleading - these are guides, not prompts
**Recommendation**: Rename to `guide_name` or `guide_id`
**WHY**: Clear naming prevents confusion about what values are expected

### `mcp__tigerdata__semanticSearchPostgresDocs`
**Critical Issues**:
1. Parameter type confusion: `version` marked as required but allows null
2. Type mismatch: PostgreSQL versions like "17.2" need string type, not integer
**Recommendations**:
- Make `version` and `limit` truly optional by removing from required array
- Change `version` to string type
**WHY**: Prevents runtime errors and failed API calls

### `mcp__tigerdata__service_create`
**Strengths**: Excellent use of enums, clear defaults, good examples
**✅ IMPLEMENTED**: Renamed `timeout` parameter to `timeout_minutes` for clarity
**Result**: Parameter name now explicitly indicates units, preventing confusion.

### `mcp__tigerdata__service_update_password`
**Missing**: No password strength requirements documented
**Recommendation**: Add validation rules (min length, character requirements) to description
**WHY**: Prevents failed attempts due to weak passwords

## Descriptions

### Excellent Examples
- `service_create`: Clear billing warnings, thorough wait/timeout explanation
- `getGuide`: Upfront listing of all available guides

### Needs Improvement

1. **`semanticSearchPostgresDocs`**
   - Missing: Return format, number of results, relevance scoring
   - **Recommendation**: "Searches PostgreSQL docs using semantic similarity. Returns up to N results with content, metadata, and distance scores (lower = more relevant)"
   - **WHY**: Users need to understand what they'll receive

2. **`service_list`**
   - Unclear: How is "current project" determined?
   - **Recommendation**: Specify if from auth context or parameter
   - **WHY**: Ambiguity about data scope causes confusion

## Functionality Testing Results

### Working Well
- All tested tools executed successfully
- Response times were reasonable
- JSON responses well-structured

### Issues Discovered

1. **Guide Content Length**: The `setup_hypertable` guide returns 15,000+ characters
   - **Impact**: Can consume significant LLM context
   - **Recommendation**: Break into smaller focused guides or add length warnings
   - **WHY**: Prevents context exhaustion in conversations

2. **Missing Error Documentation**: No information about possible error codes or formats
   - **Recommendation**: Add common error scenarios to each tool description
   - **WHY**: Enables proper error handling

## Missing Critical Functionality

1. **No service deletion**: Can create but not remove services
2. **No pause/resume**: Response shows "paused" field but no control mechanism
3. **No SQL execution**: Can't run queries against created services
4. **No connection string helper**: Users must construct connection strings manually

**WHY THESE MATTER**: Complete lifecycle management requires all CRUD operations

## Schema Design Observations

### Resource Format Inconsistency
- **Input**: `"cpu_memory": "0.5 CPU/2GB"` (combined string)
- **Output**: `{"cpu": "0.5 cores", "memory": "2 GB"}` (separate fields)
- **Impact**: Requires different parsing logic for input vs output
- **Recommendation**: Use consistent format or document the rationale
- **WHY**: Reduces implementation complexity

## Specific Priority Improvements

### High Priority
1. Fix parameter type issues in `semanticSearchPostgresDocs`
2. Standardize naming conventions across all tools
   - ~~service_get~~ ✅ COMPLETED
3. Add service deletion capability
4. ~~Rename timeout to timeout_minutes~~ ✅ COMPLETED

### Medium Priority
1. Document return formats in all descriptions
2. Add error code documentation
3. Break up large guide content

### Low Priority
1. Add filtering to `service_list`
2. Include connection string formatting helper
3. Add dry-run options for destructive operations

## Positive Highlights
- Excellent enum usage for constrained choices
- Good use of JSON Schema for validation
- Clear billing warnings where appropriate
- Well-structured responses with consistent formatting
- Semantic search provides relevant, useful results
- Good use of defaults to simplify common cases

## Why These Improvements Matter
1. **Predictability**: Consistent naming reduces learning curve
2. **Reliability**: Proper typing prevents runtime failures
3. **Usability**: Clear documentation reduces trial-and-error
4. **Completeness**: Full CRUD operations enable real workflows
5. **Efficiency**: Appropriate content sizing preserves LLM context
