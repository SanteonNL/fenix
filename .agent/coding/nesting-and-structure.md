# Nesting and Code Structure

Prefer code with shallow nesting.

Early returns and helper functions often improve readability significantly.

## Keep nesting manageable

Prefer:

```go
if user == nil {
    return
}

ProcessUser(user)
```

Over:

```go
if user != nil {
    ProcessUser(user)
}
```

Prefer early returns and helper functions over deeply nested control flow.

Practical guideline: keep nesting relatively shallow and avoid nesting more than approximately 3 levels across functions, conditionals, loops, and nested data structures.
