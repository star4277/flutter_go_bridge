## [ERR-20260729-001] combined-rg-discovery

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: infra

### Summary
A combined PowerShell discovery command returned exit code 1 when an optional `rg` search had no matches.

### Error
```
Script error: Exit code: 1
```

### Context
- The command combined directory listing, an optional `AGENTS.md` search, and source searches.
- `rg` returns exit code 1 for no matches, which marked the whole tool call as failed.

### Suggested Fix
Run optional searches separately or explicitly handle the no-match exit code.

### Metadata
- Reproducible: yes
- Related Files: none
- Recurrence-Count: 2
- Last-Seen: 2026-07-30

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Subsequent discovery commands handle optional results explicitly. On Windows, pass directories to `rg` with `-g` filters instead of shell-style path globs such as `internal/*.go`.

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

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: backend

### Summary
A large multi-file patch for InternetAddress and UUID mappings failed context verification.

### Error
```
apply_patch verification failed: Failed to find expected lines in render_go_cst.go
```

### Context
- No part of the patch was applied.
- The DCO renderer used different indentation/text than the patch context assumed.

### Suggested Fix
Apply the model and each renderer change in small patches using exact local context.

### Metadata
- Reproducible: yes
- Related Files: internal/generator/render_go_cst.go

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Switched to small, independently verifiable patches.

---

## [ERR-20260729-003] mihomoui-flutter-analyze-timeout

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: low
**Status**: resolved
**Area**: tests

### Summary
Full Flutter analysis of `example/mihomoui` exceeded the 120-second command timeout without producing output.

### Error
```
command timed out after 120221 milliseconds
```

### Context
- Command: `fvm flutter analyze`
- The generated Go bridge had already compiled successfully.

### Suggested Fix
Run a focused Dart analysis on `lib/src` with a larger timeout, then retry broader validation only if necessary.

### Metadata
- Reproducible: unknown
- Related Files: example/mihomoui/lib/src/_generated.dart

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: User ran focused format and analysis successfully; only existing informational lints were reported.

---

## [ERR-20260729-005] flutter-pub-add-partial-write

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: config

### Summary
`flutter pub add uuid` timed out after modifying `pubspec.yaml`, leaving a successful partial write despite a failed command result.

### Error
```
mapping key "uuid" already defined
```

### Context
- The timed-out command inserted `uuid: ^4.6.0`.
- A subsequent manual dependency edit added a second UUID entry.

### Suggested Fix
After a pub-add failure or timeout, re-read `pubspec.yaml` before retrying or applying a fallback edit.

### Metadata
- Reproducible: unknown
- Related Files: example/mihomoui/pubspec.yaml, command/main.go

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Kept the pub-added `uuid: ^4.6.0` entry and removed the duplicate.

---

## [ERR-20260729-002] go-test-cache-miss

**Logged**: 2026-07-29T00:00:00+08:00
**Priority**: medium
**Status**: resolved
**Area**: tests

### Summary
Go tests failed because the shared Windows build cache referenced missing compiled source-list entries.

### Error
```
loading compiled Go files from cache: reading srcfiles list: cache entry not found
```

### Context
- `go test ./internal/generator ./internal/parser`
- Failures occurred inside `go/packages` while loading temporary fixture modules.

### Suggested Fix
Run validation with an isolated `GOCACHE` inside the writable workspace.

### Metadata
- Reproducible: unknown
- Related Files: internal/parser/parser.go

### Resolution
- **Resolved**: 2026-07-29T00:00:00+08:00
- **Notes**: Full validation passed with `GOCACHE` pointed at an isolated temporary directory.

---
