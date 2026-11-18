# Composition vs Inheritance

## Prefer composition over inheritance

Composition reduces coupling and usually makes code easier to change. Inheritance tightly couples child and parent structures, and exceptions often require broad refactoring.

## Use composition by passing dependencies explicitly

Rather than inheriting shared behavior, pass required types as arguments or fields.

- `JpgImage` does not extend `Image`; it takes an `Image` and adds load/save.
- `ImageDraw` does not extend `Image`; it takes an `Image` and adds drawing.

This allows adding new types without modifying existing ones.

## Use interfaces to abstract, not parent classes

When code should not depend on concrete types, define interfaces with essential methods.

- Prefer an `ImageFile` interface with `load()` and `save()` over an abstract base class.
- Pass interfaces as dependencies instead of hardcoding concrete types.

This is dependency injection in practice.

## When embedding is acceptable in Go

Struct embedding can be useful when many types share significant identical boilerplate.

When using embedding:

- access embedded state through embedded methods, not direct field reach-through
- keep embedded fields unexported where possible
- document that embedding is for deliberate boilerplate sharing

General heuristic: if new requirements repeatedly force hierarchy restructuring, the design is likely too rigid.
