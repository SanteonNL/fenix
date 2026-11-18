# Performance and Premature Optimization

Performance is important, but usually not the first problem to solve.

Prefer code that is:

- correct
- readable
- maintainable
- appropriate for expected workload

## Consider expected usage patterns

When runtime measurement is not available, make a reasonable engineering assessment based on context.

Consider:

- execution frequency
- expected data size
- request path versus batch or background context
- likelihood of high-scale or performance-sensitive use

Repository note: test datasets in `tests/Datasetgenerator_input` are intentionally small and do not represent production-scale data, but they do give an idea about the shape of the data. Keep in mind that production-scale tends to have millions of rows.

## Prefer meaningful performance improvements

When optimization is needed, prioritize high-impact changes:

- better data structures
- better algorithms
- reducing repeated work
- removing clear bottlenecks

Prefer these over micro-optimizations.

## Avoid speculative micro-optimization

Avoid adding complexity based only on assumptions.

Examples:

- minor operator-level tweaks
- removing helper functions for theoretical gains
- replacing clear code with complex code without evidence

## Choose data structures deliberately

Select structures based on:

- data size
- lookup frequency
- mutation frequency
- ordering and uniqueness needs
- readability and simplicity

Example:

- use a map for frequent lookups on larger datasets
- use a slice for small collections or infrequent lookups

## Document performance-driven decisions

When performance needs drive a non-obvious design choice, document:

- why the approach was chosen
- assumptions
- expected workload
- tradeoffs

Example:

```go
// Uses a map because this lookup occurs for every event
// in a high-volume ingestion path.
```

## Ask for clarification when performance affects design

If performance requirements materially influence design and context is unclear, ask for:

- expected scale
- execution frequency
- latency or throughput requirements
- whether the path is performance-sensitive

Avoid major performance assumptions without enough context.

## Final optimization order

Prefer this order:

1. Correctness
2. Readability
3. Appropriate design
4. Deliberate optimization when justified
