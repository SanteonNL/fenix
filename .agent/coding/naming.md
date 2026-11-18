# Naming Guidelines

## Use full, descriptive variable names

Name every variable after what it holds. Write names in full: no single letters and no abbreviations.

- `usrMgr` -> `userManager`
- `calcBtn` -> `calculateButton`
- `cfg` -> `configuration`

## Let the type declaration speak for itself

Name variables after what they represent, not what type they are.

- `userList` -> `users`
- `nameString` -> `name`
- `isValidBool` -> `isValid`

## Always include units for numeric values

Append the unit to numeric variables where ambiguity is possible.

Examples: `delaySeconds`, `widthPixels`, `distanceMeters`.

Where the language offers typed wrappers (for example, time duration types), prefer those over naming conventions.

## Give parent types the plain domain name

Use the simple domain name for the parent and the specific qualifier for the child.

- `BaseTruck` (parent) should be `Truck`
- `Truck` (child) should be `TrailerTruck`

Difficulty naming a parent often indicates the hierarchy should be redesigned.

## Do not name things `utils` or `helper`

`utils` or `helper` modules usually indicate unclear responsibilities.

Prefer:

- methods on the domain type they belong to
- dedicated types for specific responsibilities
- small, purpose-driven packages

Examples:

- paging logic -> `Paginator`
- cookie logic -> `Cookie`
- movie operations -> `MovieLibrary`

General heuristic: if naming is hard, the code structure likely needs rethinking.
