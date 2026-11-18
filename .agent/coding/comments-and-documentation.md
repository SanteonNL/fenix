# Comments and Documentation

Documentation should improve understanding. Keep comments to a minimum unless they provide high-value context.

## Prefer self-explaining code first

Prefer:

- clear names
- good structure
- meaningful abstractions

Comments are best when they provide information code cannot express directly.

## Use comments to explain intent and reasoning

Comments add value when they explain:

- why something exists
- why an approach was chosen
- important assumptions
- non-obvious behavior
- tradeoffs or constraints

Example:

```go
// Retry is limited to avoid duplicate payment requests.
```

## Write code that reads like a comment

Before adding a comment, make code clearer by:

- extracting a well-named constant or variable for magic values
- extracting a well-named function for complex conditions

If code needs a comment to explain what it does, simplify or refactor first.

## Use types to eliminate explanatory comments

A well-chosen type communicates intent better than a comment and is compiler-enforced.

Prefer:

- optional return types over sentinel values like `-1`
- typed ownership wrappers over memory-management comments

## Treat comments as a liability

Comments are not verified by tests, compilers, or linters and can become stale. A wrong comment is worse than no comment.

## Write documentation, not comments

Prefer documentation for:

- public APIs and interfaces
- module and type responsibilities
- expected behavior (thread safety, error conditions, state transitions)

Keep docs close to the code so updates happen together.

## Domain knowledge goes to docs/

Comments explain the code. Who depends on a resource, what an application must do (business rules), and why it is built this way (architecture decisions) belong in `docs/`, not in comments.

When a change alters a business rule, a resource name, a schedule or something else that has a page in `docs/`, say so in your reply: the docs need an update in the same PR. See `.agent/documentation/index.md`.

## Document new files

When creating a new file:

- add a concise explanation at the top
- explain file purpose
- keep explanation accurate over time

Example:

```go
// Synchronizes local cache state with the remote API.
```

## Scripts should include usage examples

When creating scripts:

- include a short purpose description
- include usage examples
- keep usage docs accurate when the script changes
- for a non-trivial script, write a separate file next to it (see `.agent/documentation/script-documentation.md`)

Example:

```bash
go run ./cmd/sync-users --source users.json
```
