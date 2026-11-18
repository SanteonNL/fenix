# Documentation Writing Style

## Language

- Pages in `docs/` (resources, applications, ADRs, README) are written in **Dutch**.
- Script documentation follows the language of the script and its comments (see script-documentation.md).
- Agent guidance in `.agent/` is written in English.

## Plain Language

Write so that a colleague who does not read code can understand the page and say "correct" or "not correct".

- Use short sentences, one idea per sentence.
- Use active voice: "De refresh kopieert …", not "Er wordt gekopieerd …".
- Avoid jargon. Explain a technical term the first time you use it, or leave it out.
- One idea per paragraph. Use lists for options and numbered lists for steps.

Poor: "Via een geparametriseerde deployment-template wordt de modus geïnjecteerd, waarna conditionele filtering plaatsvindt."

Better: "De vrijdagtest start de indicatoren in de modus `autotest`. Dan verwerken ze alleen de `autotest-`-containers."

## Testable

A rule must be testable: someone can check it against what the application does.

- Poor: "Codes worden netjes gecontroleerd."
- Better: "Een vertaalde code die niet in het officiële codesysteem staat, telt als niet geverifieerd." (example from HipsETL)

If you do not know the exact behaviour, do not write a vague sentence. Leave `{invullen: <question>}`.

## Concrete Examples

- In `docs/`: examples with real values, in the form `input` → `outcome`, such as `1984-07-23` → `1984-07-01`. Use real names: `autotest-cwz-sb-copd`, not "een container".
- In script documentation: examples that run, such as a complete command for running the script locally.

## Names and Terms

- Use the name colleagues use, and the real name in Azure, so a search finds the page: `ciwesanhipsindicatoreno-vrijdagtest`, `data-refresh-t-naar-o`.
- Use the same term for the same thing everywhere. Write "ontwikkelomgeving", "testomgeving" and "productieomgeving" in full, or `o`, `t` and `p` in tables and code.
- Link to the page of every resource or application you mention.

## The User's Words

When you write down what the user told you (a reason, a context, a rule):

- keep their meaning and their examples;
- shorten and clarify, but do not add content;
- show the result to the user before you consider it done.

## Brevity

- Write for a busy reader. Cut words that add nothing.
- Do not repeat what is on another page. Link to it.
- A short page that is correct is better than a long page that is complete.
