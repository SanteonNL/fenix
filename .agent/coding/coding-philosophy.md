# General Coding Philosophy

## Balancing performance, velocity, and adaptability

Good code must balance three competing forces: **performance**, **velocity**, and **adaptability**.

- **Performance** is how fast the system executes and how efficiently it uses resources.
- **Velocity** is how quickly you can add new features and make changes.
- **Adaptability** is how well the system can evolve to meet new requirements and changing data sources.

These three forces exist in tension with each other.

Prioritizing pure velocity can create immediate speed but accumulates technical debt that eventually slows development. Prioritizing adaptability can enable rapid changes later but can waste effort on flexibility that is never needed. Adding adaptability through abstraction can introduce indirection that may hurt performance.

For this ETL platform, emphasize:

1. Adaptability: data sources, transformations, and requirements change frequently across organizations and use cases.
2. Performance: the tool processes potentially large datasets; efficient execution matters for user experience and Azure compute cost.
3. Velocity: development speed matters, but is secondary to adaptability and performance.

Prefer clear, extensible designs over quick hacks, and optimize performance deliberately rather than speculatively.

## Optimize for readability

Code is read more often than it is written. Prefer code that explains intent clearly, is easy to understand, makes responsibilities visible, and reduces cognitive load for future contributors.

## Code should explain itself

Prefer code that communicates intent through structure and naming. Choose representations that express meaning directly rather than requiring readers to decode implementation details.

## Abstraction is a tool

Abstraction improves code when it removes duplication, encapsulates complexity, represents a meaningful concept, or improves clarity. Prefer abstractions that describe real responsibilities. Simple code is often preferable to unnecessary indirection.

## Prefer composition over inheritance

Composition often produces more flexible and maintainable systems than inheritance. Prefer building behavior from collaborating parts over class hierarchies.
