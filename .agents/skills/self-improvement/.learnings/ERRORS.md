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
