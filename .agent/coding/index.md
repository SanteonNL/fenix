# Coding Skills Router

This folder is the canonical source for coding guidelines.
Use this index to decide which guideline file to read for the current task.

## Quick Start Order

1. Read overview.md.
2. Read technology-context.md.
3. Read the always needed topics:
   3.1. coding-philosophy.md
   3.2. naming.md
   3.3. nesting-and-structure.md
   3.4. comments-and-documentation.md
4. Read topic-specific file(s) needed for the task.
5. For non-trivial coding changes, also read abstraction.md and dependency-injection.md by default unless clearly irrelevant.
6. Check final-guidance.md before final output.
7. Check if personal folder is present in this folder and read all files

## Default Context Policy

- Prefer sufficient context over minimal context for coding tasks.
- Minimal context is only preferred for trivial edits (for example typo fixes, comments-only changes, or formatting-only changes).
- For feature work, behavior changes, or refactors, load abstraction.md and dependency-injection.md unless the task is purely procedural or script-only with no design choices.

## Skill Routing By Occasion

- File: overview.md
- Use when: starting any coding task, handling ambiguity, deciding how strictly to apply standards to legacy code.
- Primary focus: scope, operating principles, language matching, clarification behavior.

- File: technology-context.md
- Use when: selecting language conventions or working across Go, PowerShell, Bash, Bicep, or R.
- Primary focus: repository language context and idiomatic-language expectation.

- File: coding-philosophy.md
- Use when: making architectural tradeoffs between adaptability, performance, and delivery speed.
- Primary focus: system-level design philosophy and readability-first choices.

- File: naming.md
- Use when: naming variables, types, modules, packages, or refactoring unclear naming.
- Primary focus: descriptive naming, unit suffixes, avoiding vague utility buckets.

- File: functions.md
- Use when: creating or refactoring functions, deciding function size, or shaping parameters.
- Primary focus: purpose documentation, function focus, parameter intent.

- File: nesting-and-structure.md
- Use when: logic is deeply nested or hard to follow.
- Primary focus: early returns, helper extraction, shallow control flow.

- File: comments-and-documentation.md
- Use when: adding comments/docs, documenting new files, or writing scripts.
- Primary focus: document intent and usage, prefer self-explanatory code.

- File: abstraction.md
- Use when: deciding whether to introduce an abstraction.
- Primary focus: when abstraction is valuable versus unnecessary indirection.

- File: composition-and-inheritance.md
- Use when: designing type relationships or considering embedding/inheritance-style patterns.
- Primary focus: composition-first design, interfaces, safe embedding use.

- File: dependency-injection.md
- Use when: wiring infrastructure dependencies, choosing interfaces, or introducing factories.
- Primary focus: capability-based design, decoupling, construction boundaries.

- File: performance-and-optimization.md
- Use when: performance concerns affect implementation choices.
- Primary focus: avoid premature optimization, choose high-impact improvements.

- File: infrastructure-bicep.md
- Use when: creating or updating Bicep resources.
- Primary focus: Azure tagging consistency and operational discoverability.

- File: final-guidance.md
- Use when: validating final quality before completing a task.
- Primary focus: practical judgment, clarity, maintainability.

## Multi-File Triggers

- New feature in Go service layer: overview.md, technology-context.md, functions.md, nesting-and-structure.md, abstraction.md, dependency-injection.md, final-guidance.md.
- Refactor unclear code with many comments: naming.md, nesting-and-structure.md, comments-and-documentation.md, abstraction.md, dependency-injection.md, final-guidance.md.
- Performance-sensitive ETL path: coding-philosophy.md, performance-and-optimization.md, then final-guidance.md.
- New infra module in Bicep: overview.md, infrastructure-bicep.md, final-guidance.md.

## Conflict Resolution Priority

If guidance appears to conflict, apply this priority order:

1. overview.md (operating principles and repository consistency)
2. Topic-specific file for the current change
3. coding-philosophy.md (tradeoff framing)
4. final-guidance.md (final practical judgment)
