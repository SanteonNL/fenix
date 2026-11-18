# Abstraction Guidelines

Abstraction can improve code when it:

- removes duplication
- encapsulates complexity
- represents a meaningful concept
- improves clarity

Prefer abstractions that describe a real responsibility.

Good:

```go
invoice.CalculateTotal()
```

Less meaningful:

```go
CalculationService.Process()
```

Create abstractions when they make the system easier to understand. Abstractions can remove repetition, but this is not always useful. Small repetition is often preferable to over-coupling.

## Cases where abstraction is not worth it

- abstraction only to share data without meaningful behavior
- abstraction that does not meaningfully reduce complexity

## Cases where abstraction is worth it

- multiple implementations share intricate initialization or validation logic
- a repeated pattern clearly represents a real domain concept

Simple code is often preferable to unnecessary indirection.
