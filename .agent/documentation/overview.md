# Documentation Guidelines Overview

## Purpose

`docs/` records what cannot be derived from the code:

- **who or what depends on a resource**, and why the resource exists;
- **what an application should do**, and why: the business rules;
- **architecture choices**: why something looks the way it does, and which alternatives were rejected.

Two things get lost when nobody writes them down: the reasoning behind a choice, and dependencies that are invisible in the code. An example is a Power Automate flow outside the repo that reads the logs of a container instance by name. An agent reading the code cannot see that flow. It also cannot see what the code is supposed to do, so it will treat a bug as the intended behaviour.

The agreements are in `docs/README.md`.

## What We Do Not Write in docs/

- How the code works technically, including settings and parameters.
- Lists of functions or scripts.
- Which code depends on a resource. That is asked per PR, because it changes with every PR.

## Where Documentation Is Stored

| Content                                          | Location                                         |
| ------------------------------------------------ | ------------------------------------------------ |
| Who depends on a resource, and why it exists     | `docs/resources/<resource>.md`                   |
| What an application should do, and why           | `docs/applications/<app>/overview.md`            |
| Business rules of a large part of an application | `docs/applications/<app>/<onderdeel>.md`         |
| How to use a non-trivial script                  | Next to the script (see script-documentation.md) |

## While Coding

Do not run the impact check or change `docs/` after every edit. Work in a PR is iterative.

- Keep the decision log while you work (decision-log.md).
- At the end of a finished code change, check whether `docs/` must change, and offer `/docs-bijwerken` (update-docs.md).
- After a large change, such as a new resource or a new part of an application, it is fine to write the docs right away. Later iterations can still change things, so step 0 of `/pr-check` always checks whether those docs still match the final code of the branch.

## Impact Check Before a PR

Do this when the user asks to prepare a PR, or says the work is ready to merge. It covers all changes on the branch, not just the last edit. This is the agent version of the PR checklist in `docs/README.md`.

1. **List the names of what the branch touches.** For example: resource names (in Azure and as page file names), application names, folder names such as `20_verified`, container name prefixes, log codes and schedules.
2. **Search the docs for each name** from the repository root: `grep -ril "<name>" docs/`.
3. **On each hit, read "Wie of wat hangt van deze resource af?"** (or "... van deze applicatie af?"). Check whether the change can break one of those dependents, and report what you found to the user.
4. **Decide whether a page must change.** A page must change in the same PR when:
   - something new depends on a resource, or a dependent no longer does;
   - the reason a resource exists has changed;
   - a business rule is new, changed or removed;
   - a fact on the page is now wrong, such as a name, a schedule or a mode.
5. **Ask about things outside the repo.** Did the user create or change a flow, report or other consumer outside the repo? You cannot see that, so ask when the change touches something that has dependents outside the repo.
6. **No hits does not mean nothing is affected.** It only means the docs do not know. If you find a dependency that is not documented, propose adding it.

Prepare the documentation change yourself. The user reviews it.

## Core Principles

### 1. Never invent the why

Reasons, business rules and architecture choices come from people, not from the code. If the reason is not stated by the user or in existing docs, ask. If the user does not know either, leave a placeholder in the form `{invullen: <question>}` and mention it in your reply.

### 2. Code and intent can differ

The code shows what happens, the docs show what should happen. When they differ, do not silently align the docs with the code: the code may contain a bug. Report the difference and let the user decide.

### 3. Write when you touch it

No catch-up work on pages that the current change does not touch. A short page is better than no page. Prefer updating an existing page over creating a new one; resources that belong together share one page.

### 4. Keep it small and readable

No page per function, no technical detail. A colleague who does not read code must be able to read a page and say "correct" or "not correct".

### 5. Write the docs in Dutch

All pages in `docs/` are written in Dutch, following writing-style.md. Agent guidance in `.agent/` stays in English.
