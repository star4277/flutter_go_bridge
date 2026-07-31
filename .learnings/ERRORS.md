# Errors

## [ERR-20260731-001] powershell-rg-scan

**Logged**: 2026-07-31T09:20:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

A repository inspection script exited with code 1 because a later `rg` query returned no matches or was truncated by the pipeline, even though the useful scan output was produced.

### Error

```text
Script error: Exit code: 1
```

### Context

- The command combined several independent scans in one PowerShell invocation.
- Ripgrep uses exit code 1 for “no matches”, which PowerShell propagated as the command result.

### Suggested Fix

Run independent scans separately, exclude generated dependency folders explicitly, and normalize expected `rg` exit code 1 before returning.

### Metadata

- Reproducible: yes
- Related Files: docs/
- Recurrence-Count: 2
- Last-Seen: 2026-07-30

### Resolution

- **Resolved**: 2026-07-31T09:20:00+08:00
- **Notes**: Subsequent scans use focused commands and explicitly handle expected no-match results.

---

## [ERR-20260730-006] skill-validator-missing-pyyaml

**Logged**: 2026-07-30T08:03:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
The skill validator could not import its required `yaml` module, and a later successful command masked the failure exit code.

### Error
```
ModuleNotFoundError: No module named 'yaml'
```

### Context
- `quick_validate.py` and `gopls version` were initially run in one PowerShell command.
- `gopls version` succeeded, making the combined shell command exit successfully despite the validator traceback.

### Suggested Fix
Install `PyYAML` for the selected Python interpreter and run required validation commands separately so each exit code is authoritative.

### Metadata
- Reproducible: yes
- Related Files: .agents/skills/fgb-develop-feature/SKILL.md

### Resolution
- **Resolved**: 2026-07-30T08:03:00+08:00
- **Notes**: Installed `PyYAML` and reran `quick_validate.py` independently.

---

## [ERR-20260730-005] skill-init-python-command

**Logged**: 2026-07-30T08:01:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
The skill initializer could not run because this Windows environment exposes Python through `py`, not `python`.

### Error
```
python : The term 'python' is not recognized
```

### Context
- Command: `python .../skill-creator/scripts/init_skill.py`
- Python 3.14 was installed and available through the Windows Python Launcher.

### Suggested Fix
On Windows, check `py -3 --version` when `python` is unavailable and invoke Python scripts with `py -3`.

### Metadata
- Reproducible: yes
- Related Files: .agents/skills/fgb-develop-feature/SKILL.md

### Resolution
- **Resolved**: 2026-07-30T08:01:00+08:00
- **Notes**: Initialized the skill successfully with `py -3`.

---

## [ERR-20260729-004] multi-file-special-type-patch

**Logged**: 2026-07-31T09:52:00+08:00
**Priority**: low
**Status**: resolved
**Area**: config

### Summary

A ripgrep regex containing escaped HTML double quotes was again parsed as PowerShell syntax.

### Error

```text
src=\/ : The module 'src=' could not be loaded.
```

### Context

- Operation: scan documentation for root-absolute links before GitHub Pages deployment.

### Suggested Fix

Use single-quoted regex arguments and separate searches rather than embedding escaped double quotes.

### Metadata

- Reproducible: yes
- Related Files: docs/.vitepress/shared.ts
- See Also: ERR-20260731-005

### Resolution

- **Resolved**: 2026-07-31T09:52:00+08:00
- **Notes**: Replaced with focused single-quoted searches.

---

## [ERR-20260731-005] powershell-nested-quote

**Logged**: 2026-07-31T09:42:00+08:00
**Priority**: low
**Status**: resolved
**Area**: frontend

### Summary

A PowerShell source-inspection command failed because a nested quoted ripgrep pattern was not terminated correctly.

### Error

```text
The string is missing the terminator: ".
```

### Context

- The command searched VitePress component sources for quoted HTML class names.

### Suggested Fix

Use single-quoted PowerShell patterns or split source inspection into focused commands.

### Metadata

- Reproducible: yes
- Related Files: docs/.vitepress/theme/style.css

### Resolution

- **Resolved**: 2026-07-31T09:42:00+08:00
- **Notes**: Replaced the combined query with simple literal-pattern searches.

---

## [ERR-20260731-004] ui-ux-skill-python-missing

**Logged**: 2026-07-31T09:39:00+08:00
**Priority**: low
**Status**: resolved
**Area**: frontend

### Summary

The optional UI/UX recommendation search script could not run because Python is not installed or not on PATH.

### Error

```text
python: The term 'python' is not recognized as the name of a cmdlet, function, script file, or operable program.
```

### Context

- Operation: UI/UX design-system lookup for the VitePress home-page overlap fix.
- The skill's embedded responsive-layout guidance remains available without the script.

### Suggested Fix

Proceed using the skill's documented layout rules. Install Python only if database-backed UI recommendations are needed later.

### Metadata

- Reproducible: yes
- Related Files: docs/.vitepress/theme/style.css

### Resolution

- **Resolved**: 2026-07-31T09:39:00+08:00
- **Notes**: Continued with direct CSS inspection and viewport validation.

---

## [ERR-20260731-003] volta-npm-installation

**Logged**: 2026-07-31T09:28:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: infra

### Summary

The system `npm` launcher is broken because its Volta image cannot locate npm's bundled `npm-prefix.js`.

### Error

```text
Could not determine Node.js install directory
Error: Cannot find module 'C:\Users\Administrator\AppData\Local\Volta\tools\image\npm\11.10.1\bin\node_modules\npm\bin\npm-prefix.js'
```

### Context

- Operation: `npm run build` inside `docs/`.
- Node itself runs (`v25.6.1`), and project dependencies are already installed.

### Suggested Fix

Use the repository's actual package manager: Bun. The npm installation is outside this project's documentation workflow.

### Metadata

- Reproducible: yes
- Related Files: docs/package.json, docs/bun.lock

### Resolution

- **Resolved**: 2026-07-31T09:30:00+08:00
- **Notes**: The user clarified that docs use Bun. `bun run typecheck` and `bun run build` both pass.

---

## [ERR-20260731-002] ripgrep-windows-glob

**Logged**: 2026-07-31T09:24:00+08:00
**Priority**: low
**Status**: resolved
**Area**: docs

### Summary

Ripgrep rejected a Unix-style wildcard embedded in a Windows path argument.

### Error

```text
rg: template/plugin/gokit/docs/*.md: 文件名、目录名或卷标语法不正确。 (os error 123)
```

### Context

- PowerShell passed the wildcard path through without expanding it.
- Ripgrep treated `*` as part of a Windows filename.

### Suggested Fix

Pass the directory as the search root and use `-g '*.md'` for file filtering.

### Metadata

- Reproducible: yes
- Related Files: template/plugin/gokit/docs/
- See Also: ERR-20260731-001

### Resolution

- **Resolved**: 2026-07-31T09:24:00+08:00
- **Notes**: All later ripgrep scans use `-g` globs instead of wildcard path arguments.

---
