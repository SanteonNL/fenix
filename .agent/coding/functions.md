# Function Guidelines

## Document function purpose

When creating a new function:

- Add a concise one- or two-line explanation.
- Describe the function responsibility.
- Keep documentation accurate as implementation evolves.

Example:

```go
// Creates a retry policy based on service configuration.
func CreateRetryPolicy(cfg Config) RetryPolicy {
    // ...
}
```

Prefer describing purpose rather than implementation details.

## Prefer focused functions

Prefer functions that remain concise and focused.

Practical guideline: a function should approximately not exceed 100 lines.

When a function grows large:

- Extract cohesive sections.
- Use clearly named helper functions.
- Keep the main flow easy to read.

Exceptions are acceptable when splitting would reduce clarity.

## Keep parameter lists focused

Functions should accept only the information they need.

Prefer:

```go
SendEmail(recipient, subject)
```

Instead of:

```go
SendEmail(user)
```

when only a small subset of data is used.

Keep parameter lists relevant, intentional, and focused on function responsibility.
