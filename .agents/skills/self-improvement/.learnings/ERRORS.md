# Errors Log

Command failures, exceptions, and unexpected behaviors.

---
## [ERR-20260731-006] git-diff-check-imported-skill

**Logged**: 2026-07-31T12:30:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

`git diff --check` reported pre-existing Markdown trailing whitespace in a user-confirmed imported Skill.

### Error

```text
trailing whitespace
new blank line at EOF
```

### Context

- The user explicitly requested committing all current worktree changes.
- The imported `.agents/skills/self-improvement/**` files were user-confirmed and were not authored in this task.
- Reformatting them would create unrelated changes.

### Suggested Fix

Preserve user-confirmed imported Skill content. Report the diff-check findings and avoid rewriting unrelated whitespace unless the user explicitly requests cleanup.

### Metadata

- Reproducible: yes
- Related Files: `.agents/skills/self-improvement/**`

### Resolution

- **Resolved**: 2026-07-31T12:30:00+08:00
- **Notes**: Preserved the imported Skill as requested and continued with the explicit commit/push request.

---
## [ERR-20260731-007] plan-tool-quote

**Logged**: 2026-07-31T13:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

A plan update invocation used an unescaped non-ASCII string in JavaScript and failed before any repository action.

### Error

```text
SyntaxError: Unexpected identifier
```

### Context

- The failure occurred while updating the implementation plan for version-management work.
- No files or repository state were changed by the failed invocation.

### Suggested Fix

Use valid JSON or carefully quoted JavaScript strings when passing non-ASCII plan text to the orchestration tool.

### Metadata

- Reproducible: no
- Related Files: none

### Resolution

- **Resolved**: 2026-07-31T13:00:00+08:00
- **Notes**: Continued with a correctly quoted plan update.

---
## [ERR-20260731-008] local-bash-unavailable

**Logged**: 2026-07-31T13:20:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary

A local beta-version shell simulation could not run because Bash is unavailable in the Windows environment.

### Error

```text
bash unavailable
```

### Context

- The GitHub Release workflow uses Bash on `ubuntu-latest`.
- `actionlint` had already validated the workflow syntax and embedded shell structure.

### Suggested Fix

Use an equivalent PowerShell simulation for local algorithm checks and rely on `actionlint` for GitHub Actions shell validation.

### Metadata

- Reproducible: yes
- Related Files: `.github/workflows/release.yml`

### Resolution

- **Resolved**: 2026-07-31T13:20:00+08:00
- **Notes**: Replaced the local Bash-only simulation with an equivalent PowerShell check.

---
