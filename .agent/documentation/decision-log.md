# Decision Log

During a conversation, the developer makes decisions and gives reasons that are not in the code. You keep track of them in a decision log, so the developer does not have to remember what to document. The log is the input for `/docs-bijwerken` (see update-docs.md).

## Where

- One file per branch: `.beslissingen/<branch>.md`, with `/` in the branch name replaced by `-`. Example: `feature/pc-ai-werkwijze` → `.beslissingen/feature-pc-ai-werkwijze.md`.
- Create the file and the folder when you write the first entry.
- The folder is in `.gitignore`. The log never goes into a PR; the docs do.

## When to Write an Entry

Write an entry when the developer, in the conversation:

- **decides** between options: "we use separate containers, not the test environment";
- **gives a reason**: "because the analysts work on that data";
- **states, confirms or corrects a rule**: "`wo` gets no autotest copy";
- **names a dependent outside the repo**: "Power BI reads that folder too";
- **answers a question you asked** about intent.

Do not write an entry for:

- what you inferred from the code yourself. That is a guess about intent: ask the developer, and only log the answer;
- technical details: how the code works is in the code;
- something that is already on a page in `docs/` and still correct;
- your own suggestions that the developer has not confirmed.

## How to Write an Entry

One line per entry, in Dutch, in the developer's words. Shorten, but do not add content.

```markdown
- [beslissing] Autotests krijgen eigen containers, niet de testomgeving: daar wordt echte data aangeleverd. → adr
- [regel] Domein `wo` krijgt geen autotest-kopie, want voor `wo` worden geen indicatoren berekend. → docs/resources/storage-containers.md
- [afhankelijke] Power BI-rapporten lezen de containers en moeten `autotest-` overslaan. → docs/resources/storage-containers.md
- [open] Welke mappen leest Power BI? → docs/resources/storage-containers.md
```

- Types: `beslissing`, `regel`, `reden`, `afhankelijke`, `open`.
- After the arrow: the page where it probably belongs, or `adr`.
- A later entry that contradicts an earlier one replaces it. Remove the old line, so the log always shows the current state.

## Telling the Developer

End your reply with one short line listing what you logged, such as: "Genoteerd: regel over `wo`, afhankelijke Power BI." Then the developer can correct it right away. Do not mention the log when you logged nothing.

## What Not to Do

- Do not change pages in `docs/` after every prompt. Work in a PR is iterative: what is decided now may change in ten minutes. See update-docs.md for when the docs are written.
- Do not ask documentation questions in the middle of coding work, unless the answer changes what you build. Log them as `open`; they are asked in `/docs-bijwerken`.
