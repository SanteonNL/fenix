# Architecture Decision Records

An ADR records one architecture decision: the context, the decision, the alternatives and the consequences.
ADRs live in `docs/adr/<number>-<short-name>.md` and follow `docs/adr/_template.md`. They are written in Dutch.

## When an ADR Is Needed

Write an ADR when at least one of these is true:

- the decision is hard to reverse;
- it affects more than one application or resource;
- there were real alternatives;
- someone will later ask "why is it built like this?".

No ADR for bug fixes, small implementation choices, or choices that follow directly from an existing ADR.

## ADRs and Pages

- A resource or application page describes the current state. An ADR describes one decision at one moment.
- A business rule on an application page has a short reason next to it. That is not an ADR. An ADR tells the full story of a design choice: the problem, the alternatives, the trade-off and the consequences.
- A changed business rule needs an ADR only if the change itself is a decision: there were alternatives, or it has consequences such as comparability or privacy. Then the page gets the new rule, its short reason and a link to the ADR, and the ADR refers to the rule number ("wijzigt R3"). A clarification or small correction needs no ADR; its history is in git.
- Rule of thumb: if it can be tested against how the application works, it belongs on the application page. If there were alternatives and someone will ask why this one, it needs an ADR.
- On a page, "Waarom niet anders?" contains only a link to the ADR, not the reasoning itself.
- An accepted ADR is never edited, except its status. A new decision gets a new ADR, and the old one gets the status `Vervangen door ADR-<number>`.

## Status

- `Voorgesteld`: every new ADR starts here.
- `Geaccepteerd`: set by the user just before the PR, when the team agrees. Never set this yourself.
- The `datum` in the header is the day the status last changed.
- `Vervangen door ADR-<number>`.

## What You May and May Not Fill In

The context, the reasons, the alternatives and the trade-offs come from people. They are not in the code.

| Section                 | You may                                                         | You may not                    |
| ----------------------- | --------------------------------------------------------------- | ------------------------------ |
| Beslissing              | Describe *what* changed, based on the code. Ask the user to confirm. | Add *why* it changed      |
| Raakt                   | List the resources and applications the change touches, from the code | —                        |
| Context                 | Write down what the user tells you                              | Infer or invent it             |
| Overwogen alternatieven | Write down what the user tells you. You may ask about an alternative you see, as a question. | Present your own alternatives as considered |
| Gevolgen                | Name facts that follow from the code, and mark them as such. Ask the user for the rest. | Invent trade-offs     |

If the user does not know an answer, leave `{invullen: <the question>}` and list it at the end.

## The Interview (/adr)

1. **Choose the subject.** If the user gave one, use it. Otherwise read the diff of the branch against `main` and propose the architecture decisions you see as a numbered list. The user picks one, or names another subject. Do not start the interview before the user has chosen.
2. **Check existing ADRs.** Search `docs/adr/` for the subject, and for the names of the resources and applications the decision touches. Show the user the ADRs that the new decision may replace, with one sentence each on why. The user confirms which ADR is replaced. Then:
   - set the old ADR to `status: Vervangen door ADR-<new number>` and `datum` to today. This is the only change an accepted ADR may get;
   - give the new ADR `vervangt: ADR-<old number>` in its header;
   - if only part of the old decision changes, the new ADR still replaces it as a whole, and briefly repeats what still holds.
3. **Collect the facts.** Draft "Beslissing" (what) and "Raakt" from the code, and show them to the user.
4. **Ask, one or two questions at a time**, in Dutch, and wait for the answer. Use these questions as a starting point:
   - Context: "Welk probleem moest dit oplossen?" "Waarom was het juist nu nodig?" "Welke eisen of beperkingen speelden mee?"
   - Alternatives: "Welke andere oplossingen hebben jullie overwogen?" "Waarom vielen die af?" "Hebben jullie ook <alternative you see> overwogen?"
   - Consequences: "Wat wordt hierdoor makkelijker?" "Welke nadelen nemen jullie bewust voor lief?" "Waar moet een collega voortaan op letten?"
   - People: "Wie waren bij deze beslissing betrokken?"
5. **Write in the user's words.** Shorten and clarify, following writing-style.md, but do not add content.
6. **Save the ADR** with the next free number and status `Voorgesteld`.
7. **Link it from the pages.** On every page under "Raakt", replace the content of "Waarom niet anders?" with a link to the ADR. Is there reasoning on the page that is not yet in the ADR? Move it into the ADR first, and show the user.
8. **Finish** with a list of the remaining `{invullen}` placeholders.
