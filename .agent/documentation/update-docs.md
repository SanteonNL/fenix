# Updating the Docs (/docs-bijwerken)

This is how the domain knowledge from a branch ends up in `docs/`. The developer gives the decisions and reasons; you write the pages and ask what you cannot know.

## When

- **The developer runs `/docs-bijwerken`.** For example when a feature is done, or before a break.
- **Step 0 of `/pr-check`** runs this procedure first. So the docs are always updated before a PR, even if the developer forgot.
- **At the end of a finished code change**, check whether it changes something that has a page in `docs/`: a business rule, a dependent, the reason a resource exists, or a fact such as a name, schedule or mode. If it does, say so in one line and offer to run `/docs-bijwerken`. Do not run it while the developer is still trying out approaches.

## Steps

1. **Collect the input.**
   - The decision log `.beslissingen/<branch>.md` (see decision-log.md), if it exists.
   - The diff of the branch against `origin/main`: run `git fetch origin main` first, then `git diff origin/main...HEAD`, plus uncommitted changes. Do not use the local `main`: it is often behind, and then the diff also contains other people's merged PRs.
   - The pages in `docs/` that this branch already changed: `git diff origin/main...HEAD -- docs/`. Docs written earlier in the branch may be out of date after later iterations. Compare every fact on them with the final code of the branch, and fix what no longer matches.
2. **Do the impact check** from overview.md. Pay extra attention to **silent assumptions** in code that the branch touches or that depends on it:
   - code that lists "everything" and filters out what does not belong, such as all containers except those matching a pattern;
   - code that assumes a fixed naming convention or a fixed list;
   - code that assumes something always exists, or is always empty.

   A silent assumption that the change makes untrue is a finding. A silent assumption that still holds becomes a "leunt op" on the resource page, or a rule on the application page.
3. **Make a plan** and show it to the developer: which pages change, which pages are new, and whether an ADR is needed (see adr.md). Per page, list what changes in one line each.
4. **Ask what you cannot know**, one or two questions at a time, in Dutch. Start with the `open` entries from the log. Typical questions:
   - **Wat:** "In de code zie ik dat X gebeurt. Is dat de bedoeling, of een fout?"
   - **Waarom:** "Waarom moet dit zo?"
   - **Wie leunt erop:** "Gebruikt iets buiten de repo deze naam, map of logcode?"
   - **Alternatieven:** "Hebben jullie ook Y overwogen? Waarom viel dat af?" Real alternatives mean an ADR: continue with the `/adr` interview, and use the log entries so you only ask what is still missing.

   If the developer does not know, leave `{invullen: <vraag>}`.
5. **Write the pages**, following structure-and-content.md, writing-style.md and the templates. Use the developer's words from the log and the answers.
6. **Check** with final-review.md.
7. **Clean up the log.** Remove every entry that is now on a page or in an ADR. What remains is still open; tell the developer.
8. **Report** the files you changed, every `{invullen}` you left, and what you could not verify.

## Rules

- Never invent the why. A reason, rule, owner or dependent outside the repo comes from the developer, the log or an existing page.
- A rule you inferred from the code is a question, not a fact. The code may contain a bug.
- Change only pages that this branch touches. No catch-up work on other pages.
