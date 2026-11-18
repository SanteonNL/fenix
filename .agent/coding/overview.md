# Coding Guidelines Overview

## Purpose

These guidelines help agents and developers create code that is readable, maintainable, and aligned with repository standards.

Apply these guidelines primarily to new code and new files. Legacy code does not need to be refactored solely to satisfy these guidelines. When modifying existing code, improve it when it naturally supports the functional work being done.

Use judgment. These guidelines describe preferred practices, not rigid laws.

## Agent Operating Principles

### Follow repository standards

When working in this repository:

- Follow existing repository conventions and patterns.
- Prefer consistency with nearby code when appropriate.
- Prioritize correctness, readability, and maintainability.
- Consider long-term maintainability over short-term convenience.

### Respond in the user's language

Respond in the same language as the user's prompt unless explicitly asked otherwise. However, always prefer idiomatic language for code.

### Ask for missing information

When important information or specifications are missing:

- Ask clarifying questions.
- Explain what information is needed.
- Avoid making high-impact assumptions.

Reasonable low-risk assumptions are acceptable when clearly communicated.

### Respect legacy code

These guidelines primarily apply to new code.

When modifying legacy code:

- Avoid refactoring solely for style compliance.
- Refactor when it supports the functional change.
- Improve nearby code opportunistically when doing so improves clarity or maintainability.

### Always check your work

When you have finished writing code, always check for errors. Common errors include:

- Syntax errors
- Unused imports
- Unused functions
- Unused variables
- Usage of non-existing library functions
