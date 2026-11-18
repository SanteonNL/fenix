# Documentation Final Review

Check this list before you present a documentation change as finished. It covers one documentation change. For a whole PR, the user runs `/pr-check` (`docs/pr-check.md`).

## Content

- [ ] Pages in `docs/` contain only what the code cannot tell: dependents, business rules, reasons, decisions. No technical explanation.
- [ ] Every reason, rule, owner and dependent outside the repo comes from the user or from an existing page. Anything else is a `{invullen: <question>}`.
- [ ] Every business rule is testable, and has a reason, an example with real values, and what happens when a value is missing or invalid.
- [ ] "Wie of wat hangt af?" lists only who depends on this page's subject, names applications and resources (no scripts or functions), and says on every line what the dependent leans on.

## Facts

- [ ] Every fact matches the code as it is now: names, schedules, modes, log codes, folder names, pipeline names. Check them in the code, not from memory.
- [ ] The same fact is the same on every page. Search for it: `grep -ril --include=*.md "<term>" .`. Schedules and names often appear on several pages.

## Links and Names

- [ ] Every relative link points to a file that exists.
- [ ] Every resource or application with its own page is linked where it is mentioned.
- [ ] The real name in Azure (or the tool outside the repo) is on the page.
- [ ] If you renamed or moved something, no page still uses the old name or path. Search for the old name.

## Pages and ADRs

- [ ] Template comments are removed, and the header is filled in or has a `{invullen}`.
- [ ] `laatst-gecontroleerd` changed only if the whole page was compared with reality.
- [ ] "Waarom niet anders?" contains only a link to an ADR, and that ADR lists the page under "Raakt".
- [ ] A new ADR has the next free number and status `Voorgesteld`. A replaced ADR has status `Vervangen door ADR-<number>`.

## Your Reply

End with:

- the files you changed or created;
- every `{invullen}` you left, with its question;
- what you could not verify, and every assumption you made.
