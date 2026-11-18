# Agent Guidance Router

## Purpose

This repository uses one short, always-on router file for agent instructions.
The detailed guidance stays in `.agent/` so the router remains stable and easy to maintain.

## Instruction Source of Truth

- Start at `.agent/index.md`.
- Follow the most relevant domain guidance:
  - coding: `.agent/coding/index.md`
  - documentation: `.agent/documentation/index.md`

## Working Rule

Carefully assess which instruction files you need to read. Try to balance the amount of context you take in with the completeness of the context. Not every task needs all files, but when writing documentation you should at least read the main documentation instructions and similarly, when writing code you should read at least the main coding instructions.

## Always: Domain Knowledge

These rules apply in every conversation, also when you read no other guidance file.

- **Keep the decision log.** When the developer decides something, gives a reason, states a rule or names a dependent outside the repo, add one line to `.beslissingen/<branch>.md`. End your reply with one short line saying what you logged. Details: `.agent/documentation/decision-log.md`.
- **Do not update `docs/` after every prompt.** At the end of a finished code change, check whether `docs/` must change and offer `/docs-bijwerken`. Details: `.agent/documentation/update-docs.md`.
- **Never invent the why.** Reasons, business rules and dependents outside the repo come from the developer. Ask, or leave `{invullen: <question>}`.

## Extension Rule

When adding guidance, prefer extending `.agent/` files and indexes.
Keep this `AGENTS.md` file short and focused on routing.
