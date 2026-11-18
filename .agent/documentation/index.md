# Documentation Skills Router

This folder tells an agent how to apply the documentation architecture of this repository.
The architecture itself is defined in `docs/` and is the source of truth:

- `docs/README.md`: agreements, folder structure, PR checklist.
- Templates: `docs/resources/_template.md`, `docs/applications/_template.md`, `docs/applications/_template-onderdeel.md` and `docs/adr/_template.md`.
- Older pages in the root of `docs/` predate this structure. Do not use them as an example for new pages.

If guidance in this folder conflicts with `docs/`, follow `docs/` and point out the conflict to the user.

## Quick Start Order

1. Read overview.md.
2. Read `docs/README.md`.
3. Read the topic-specific file(s) and the template for the page you write.
4. Check final-review.md before finishing.

## Skill Routing By Occasion

- File: overview.md
- Use when: starting any documentation task, or preparing a PR (to decide whether `docs/` must change in the same PR).
- Primary focus: what we document and what not, the impact check before a PR.

- File: structure-and-content.md
- Use when: writing or updating a page in `docs/resources/` or `docs/applications/`.
- Primary focus: choosing the page type and template, what belongs in each section, what an agent must never fill in on its own.

- File: writing-style.md
- Use when: writing any documentation text.
- Primary focus: Dutch plain language, testable rules, concrete examples.

- File: script-documentation.md
- Use when: documenting a non-trivial script in a file next to the script.
- Primary focus: usage documentation beside the code, linking to `docs/` for the why.

- File: adr.md
- Use when: a change contains an architecture decision, the user runs `/adr`, or a page needs "Waarom niet anders?".
- Primary focus: when an ADR is needed, what you may fill in yourself, the interview with the user.

- File: decision-log.md
- Use when: always, during any conversation in which the developer makes decisions, gives reasons or states rules.
- Primary focus: what to log in `.beslissingen/<branch>.md`, and what not.

- File: update-docs.md
- Use when: the user runs `/docs-bijwerken`, step 0 of `/pr-check`, or a finished code change touches something that has a page in `docs/`.
- Primary focus: turning the decision log and the diff into page changes, and asking what you cannot know.

- File: final-review.md
- Use when: finishing any documentation change.
- Primary focus: PR checklist, links, open placeholders, consistency with `docs/`.

## Multi-File Triggers

- Preparing a PR, or `/docs-bijwerken`: overview.md, update-docs.md, decision-log.md, final-review.md. If a page must change, continue with the trigger below that fits.
- New or changed resource page: overview.md, structure-and-content.md, writing-style.md, final-review.md, `docs/resources/_template.md`.
- New or changed application or business rule: overview.md, structure-and-content.md, writing-style.md, final-review.md, `docs/applications/_template.md` (and `_template-onderdeel.md` for a large part).
- New API: treat it as a part (onderdeel) of an application, or as an application of its own. See structure-and-content.md.
- New non-trivial script: script-documentation.md, writing-style.md, final-review.md.
- Architecture decision (or `/adr`): adr.md, writing-style.md, `docs/adr/_template.md`.

## When NOT to Write Documentation

- How the code works technically: that is in the code.
- Which code depends on a resource: ask per PR, do not write it in `docs/`.
- Simple helper functions, configuration files and trivial scripts: a header or inline comment is enough.
- Catching up on pages that the current change does not touch.

## Conflict Resolution Priority

If guidance appears to conflict, apply this priority order:

1. `docs/README.md` and the templates
2. overview.md
3. Topic-specific file for the current documentation type
4. writing-style.md
5. final-review.md
