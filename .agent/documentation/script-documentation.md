# Script Documentation

Technical documentation for a script, in a file next to the script. Unlike the pages in `docs/`, this documentation explains how the script works and how to use it. Example (from HipsETL): `pipelines/scripts/Copy-BlobsBetweenStorageAccounts.md`.

## When

- **Non-trivial script:** many settings, several steps, authentication, or failure modes someone must be able to diagnose. Write a separate file.
- **Short or simple script:** a header with its purpose and a usage example is enough (see `.agent/coding/comments-and-documentation.md`). No separate file.

## Where

- Next to the script, as `<script name>.md`, such as `Copy-BlobsBetweenStorageAccounts.md`.
- The header of the script refers to this file.
- Helper modules of the script do not get their own file. They refer to the same file.

## Structure

Start with the TL;DR. Leave out the other sections when they do not apply.

1. **TL;DR:** one or two sentences: what the script does and why it exists. A developer knows at once whether this is the script they need.
2. **Used by:** which pipelines or people run it, and when.
3. **Features:** what it can do, including safeguards such as a dry run or refusing to delete outside development.
4. **How a run works:** the steps in order. A table helps for decisions per case, such as "folder has files, replacement off → skipped".
5. **Authentication:** who logs in, with which identity or service connection, and why that one.
6. **Settings:** a table with the setting, the parameter or environment variable, the default and what it does.
7. **Running locally:** a working example command.
8. **Troubleshooting:** a table with the message, the cause and what to do.
9. **Files:** the script and its helper modules, one line each.

## Boundary With docs/

- Technique belongs here, including the reason for a technical choice, such as "copied through the build agent because of the network rules".
- Who depends on the script or its pipeline, and the business rules it implements, belong on a page in `docs/`. Link to that page instead of repeating it. Example (from HipsETL): "data is never copied to production" is a rule on the page of the storage containers.
- An architecture decision with real alternatives gets an ADR (see adr.md).

## Writing

- Script documentation may be in English or Dutch; follow the language of the script and its comments.
- Keep it accurate: when the script changes, update this file in the same PR. Schedules, setting names and pipeline names go out of date first.
