# Page Types and Content

This file covers the pages in `docs/`. Those record what cannot be derived from the code, and contain no technical explanation. Technical documentation next to a script, such as `Copy-BlobsBetweenStorageAccounts.md` in HipsETL, has another purpose and structure: see script-documentation.md.

The templates in `docs/` define the sections of each page and explain them in comments. Follow the template; do not invent extra sections. This file covers what agents get wrong: choosing the page type, the rules per section, and what you must never fill in yourself.

## Choosing the Page Type

| What you document                                                                 | Page                                                            | Template                                  |
| --------------------------------------------------------------------------------- | --------------------------------------------------------------- | ----------------------------------------- |
| Something others depend on: a container, a storage account, a flow, an alert, a notification | `docs/resources/<name>.md`                          | `docs/resources/_template.md`             |
| What an application must do, and why: its parts and business rules                | `docs/applications/<app>/overview.md`                           | `docs/applications/_template.md`          |
| A part of an application with many or precise rules                               | `docs/applications/<app>/<part>.md`                             | `docs/applications/_template-onderdeel.md` |
| Why something is built this way, when there were alternatives                     | `docs/adr/<number>-<name>.md`                                   | `docs/adr/_template.md`, see adr.md       |
| How to use a non-trivial script                                                   | Next to the script                                              | see script-documentation.md               |
| How code works, or which code depends on what                                     | Nothing. That is in the code, or asked per PR.                  | —                                         |

Guidelines:

- **Resource or application?** Code with business rules is an application. Something that runs, stores or triggers, and that others depend on, is a resource. Things outside the repo (a Power Automate flow, an Azure DevOps notification) are resources.
- **An API** is a part of the application that offers it. It becomes an application of its own only if it runs separately: its own resource, its own release, or another owning team. Its consumers go under "Wie of wat hangt van deze applicatie af?".
- **Related resources share one page**, such as the two indicatoren container instances.
- **Prefer updating an existing page** over creating a new one. Search first: `grep -ril "<name>" docs/`.
- **File names** use lower case and hyphens, and the name colleagues use. Put the real name in Azure on the page itself, so a search on that name finds the page.

## Rules per Section

**Header**
- `eigenaar`: never guess. Ask, or leave `{invullen}`.
- `laatst-gecontroleerd`: only set it to today if the whole page was compared with reality, not when you changed one line.

**"Wie of wat hangt van deze resource/applicatie af?"**
- One direction only: who depends on *this*, not what this depends on.
- Applications and resources, never functions or scripts.
- Every line says what the dependent leans on, in plain words: a name, a log code, the folder `20_verified`.
- Link to the dependent's page if it exists. Mark things outside the repo with "(buiten de repo)".
- You cannot see dependents outside the repo. Ask the user about them.

**Business rules** (application and part pages)
- Each rule is testable: a colleague can say "the code does this" or "it does not".
- Each rule has a short reason, an example with real values (`input` → `outcome`), and what happens when a value is missing or invalid.
- On a part page, rules are numbered (R1, R2, …) so code, tests, PRs and ADRs can refer to them.
- A rule you derived from the code is a guess about the intent: the code may contain a bug. Present it to the user as a question, and write it down only after they confirm.

**"Waarom niet anders?"** (resource pages)
- Only a link to the ADR that explains the choice. Is there a choice but no ADR? Propose `/adr`.

**"Regels"** (resource pages, optional)
- Rules about the resource itself: environments, naming, retention, refresh. What an application does belongs on its application page.

**Finally**
- Remove the template comments.
- Use relative links between pages.

## Never Fill In Yourself

- Reasons, intent and history ("why").
- Owners and the people involved in a decision.
- Business rules you only inferred from code, until the user confirms them.
- Dependents outside the repo.

Leave a placeholder instead, in the form `{invullen: <question>}`, and list every placeholder you left in your reply.
