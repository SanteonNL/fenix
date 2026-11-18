# Infrastructure and Bicep Guidelines

## Tag Azure resources consistently

When creating Bicep resources:

- include relevant tags
- group resources by functional ownership
- use concise and meaningful tag values

Example:

```bicep
tags: {
  capability: "logShipper"
  description: "Resources used for local log forwarding"
}
```

Consistent tagging improves:

- discoverability
- ownership tracking
- cost analysis
- operational maintenance
