package queryvalidator

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/converter"
)

// Validator checks SQL text against the FHIR model and the loaded profiles.
// It never touches a database: everything is derived from the SELECT lists.
type Validator struct {
	Profile *converter.ProfileService // optional: without it only base R4 structure is checked
}

// stmtInfo is one parsed statement.
type stmtInfo struct {
	n     int
	path  string
	items []selectItem // data columns (metadata columns removed)
}

var metaColumns = []string{"resource_id", "id", "parent_id", "fhir_path"}

func isMeta(alias string) bool {
	a := strings.ToLower(alias)
	for _, m := range metaColumns {
		if a == m {
			return true
		}
	}
	return false
}

// ValidateSQL validates one (already template-rendered) SQL file.
func (v *Validator) ValidateSQL(name, sql string) *FileReport {
	c := newCollector()
	stmts := converter.SplitStatements(sql)
	rep := &FileReport{File: name, Statements: len(stmts)}

	byPath := map[string][]*stmtInfo{}
	var paths []string
	for i, s := range stmts {
		si := v.parseStatement(i+1, s, c)
		if si == nil || si.path == "" {
			continue
		}
		if _, ok := byPath[si.path]; !ok {
			paths = append(paths, si.path)
		}
		byPath[si.path] = append(byPath[si.path], si)
	}
	v.checkTree(paths, byPath, c)
	rep.Findings = c.findings()
	return rep
}

func (v *Validator) parseStatement(n int, stmt string, c *collector) *stmtInfo {
	items, star, ok := selectList(stmt)
	if !ok {
		c.add(Error, CodeSQLNoSelect, n, "", "", "statement has no SELECT; every statement must return rows", 0, "")
		return nil
	}
	if star {
		c.add(Warning, CodeSQLSelectStar, n, "", "", "SELECT * makes the output depend on the table layout and cannot be checked; list the columns explicitly", 0, "")
	}

	si := &stmtInfo{n: n}
	seen := map[string]bool{}
	have := map[string]bool{}
	var pathItem *selectItem
	for idx := range items {
		it := items[idx]
		if it.Alias == "" {
			c.add(Warning, CodeSQLUnnamed, n, "", "", fmt.Sprintf("expression %q has no AS alias; its column name is database dependent", trunc(it.Expr, 40)), 0, "")
			continue
		}
		l := strings.ToLower(it.Alias)
		if seen[l] {
			c.add(Error, CodeSQLDupCol, n, "", it.Alias, "column appears more than once in the SELECT list", 0, "")
			continue
		}
		seen[l] = true
		if isMeta(it.Alias) {
			have[l] = true
			if l == "fhir_path" {
				pathItem = &items[idx]
			}
			continue
		}
		si.items = append(si.items, it)
	}
	if !star {
		for _, m := range metaColumns {
			if !have[m] {
				sev := Error
				if m == "parent_id" {
					sev = Warning // fine for root rows, children cannot attach without it
				}
				c.add(sev, CodeSQLMeta, n, "", m, "required metadata column is missing; the converter cannot place the rows", 0, "")
			}
		}
	}
	if pathItem != nil {
		if lit, ok := pathItem.literal(); ok {
			si.path = strings.TrimSpace(lit)
		} else {
			c.add(Warning, CodeSQLDynamicPath, n, "", "fhir_path", "fhir_path is not a string literal, so this statement cannot be checked against the model", 0, "")
		}
	}
	return si
}

// walkResult is the outcome of resolving a (possibly dotted) name inside a struct.
type walkResult struct {
	leaf    field
	owner   *structInfo // struct that declares leaf
	elem    string      // profile path with choice variants as name[x]
	prefix  string      // dotted part before the leaf, "" for a direct column
	bad     string      // first segment that does not resolve
	reason  string      // why: "unknown", "primitive"
	suggest string
	cased   []string // "wrong -> right" case mismatches
	ok      bool
}

func walk(t reflect.Type, elem string, segs []string) walkResult {
	res := walkResult{elem: elem}
	for k, seg := range segs {
		si := structOf(t)
		if si == nil {
			res.bad, res.reason = seg, "primitive"
			return res
		}
		f, exact, found := si.lookup(seg)
		if !found {
			res.bad, res.reason = seg, "unknown"
			res.suggest = closest(seg, si.names())
			return res
		}
		if !exact {
			res.cased = append(res.cased, seg+" -> "+f.Name)
		}
		key := f.Name
		if f.ChoiceBase != "" {
			key = f.ChoiceBase + "[x]"
		}
		res.elem += "." + key
		if k == len(segs)-1 {
			res.leaf, res.owner, res.ok = f, si, true
			res.prefix = strings.Join(segs[:k], ".")
			return res
		}
		t = f.Elem
	}
	return res
}

func (v *Validator) rule(elem string) (converter.ElementRule, bool) {
	if v.Profile == nil {
		return converter.ElementRule{}, false
	}
	return v.Profile.Element(elem)
}

func (v *Validator) checkTree(paths []string, byPath map[string][]*stmtInfo, c *collector) {
	sort.Slice(paths, func(i, j int) bool {
		di, dj := strings.Count(paths[i], "."), strings.Count(paths[j], ".")
		if di != dj {
			return di < dj
		}
		return paths[i] < paths[j]
	})

	var roots []string
	for _, p := range paths {
		if !strings.Contains(p, ".") {
			roots = append(roots, p)
		}
	}
	if len(roots) == 0 {
		c.add(Error, CodePathNoRoot, 0, "", "", "no statement has a root fhir_path (e.g. 'Patient'); no resource can be built", 0, "")
	}
	if len(roots) > 1 {
		c.add(Warning, CodePathMixedRoots, 0, "", "", fmt.Sprintf("file mixes resource types %s; the output file will hold several types", strings.Join(roots, ", ")), 0, "")
	}

	noProfile := map[string]bool{}
	for _, p := range paths {
		root := strings.SplitN(p, ".", 2)[0]
		stmt := byPath[p][0].n

		if !resourceTypes[root] {
			msg := fmt.Sprintf("%q is not a FHIR R4 resource type", root)
			if s := closest(root, keys(resourceTypes)); s != "" {
				msg += fmt.Sprintf(" (did you mean %s?)", s)
			}
			c.add(Error, CodePathUnknownType, stmt, p, "", msg, 0, "")
			continue
		}
		res, err := converter.NewResource(root)
		if err != nil {
			c.add(Warning, CodePathUntyped, stmt, p, "", "FENIX has no typed model for "+root+": the resource is passed through unvalidated, and de-identification refuses it", 0, "")
			continue
		}
		if strings.Contains(p, ".") && !contains(roots, root) {
			c.add(Error, CodePathNoRootStmt, stmt, p, "", "no statement produces the root "+root+", so these rows have nothing to attach to and are dropped", 0, "")
		}
		if v.Profile != nil && !v.Profile.HasProfileFor(root) && !noProfile[root] {
			noProfile[root] = true
			c.add(Info, CodeProfNone, 0, root, "", "no loaded profile describes "+root+"; only base FHIR R4 structure is checked", 0, "")
		}

		// Resolve the fhir_path itself.
		rootT := reflect.TypeOf(res).Elem()
		pt, elem := rootT, root
		if strings.Contains(p, ".") {
			w := walk(rootT, root, strings.Split(p, ".")[1:])
			if !w.ok || !w.leaf.Struct {
				v.reportBadPath(c, stmt, p, w)
				continue
			}
			pt, elem = w.leaf.Elem, w.elem
			for _, cs := range w.cased {
				c.add(Warning, CodePathUnknown, stmt, p, "", "fhir_path is case-sensitive in FHIR: "+cs, 0, "")
			}
		}

		var items []selectItem
		for _, si := range byPath[p] {
			items = append(items, si.items...)
		}
		v.checkColumns(c, stmt, p, pt, elem, items)
		v.checkCoverage(c, stmt, p, pt, elem, items, paths)
	}
}

func (v *Validator) reportBadPath(c *collector, stmt int, p string, w walkResult) {
	if w.reason == "primitive" {
		c.add(Error, CodePathUnknown, stmt, p, "", fmt.Sprintf("%q is a primitive element and cannot have children", w.bad), 0, "")
		return
	}
	if w.reason == "" { // resolved, but to a primitive
		c.add(Error, CodePathUnknown, stmt, p, "", "fhir_path ends at a primitive element; rows must describe a complex element or resource", 0, "")
		return
	}
	parent := p
	if segs := strings.Split(p, "."); len(segs) > 1 {
		for i := 1; i < len(segs); i++ {
			if segs[i] == w.bad {
				parent = strings.Join(segs[:i], ".")
				break
			}
		}
	}
	msg := fmt.Sprintf("%s has no element %q", parent, w.bad)
	if w.suggest != "" {
		msg += fmt.Sprintf(" (did you mean %s?)", w.suggest)
	}
	c.add(Error, CodePathUnknown, stmt, p, "", msg, 0, "")
}

// expectedPrimitive decides which FHIR primitive a column must contain.
func (v *Validator) expectedPrimitive(w walkResult, rule converter.ElementRule, hasRule bool) string {
	if p := primitiveFromChoice(w.leaf.ChoiceType); p != "" {
		return p
	}
	if hasRule && len(rule.Types) == 1 {
		if p := primitiveFromRule(rule.Types[0]); p != "" {
			return p
		}
	}
	if p, ok := leafPrimitives[w.owner.typ.Name()+"."+w.leaf.Name]; ok {
		return p
	}
	return w.leaf.Prim
}

func (v *Validator) checkColumns(c *collector, stmt int, path string, pt reflect.Type, elem string, items []selectItem) {
	owners := map[string]map[string]selectItem{} // dotted prefix → leaf name → item
	owners[""] = map[string]selectItem{}
	ownerType := map[string]string{"": pt.Name()}
	choiceSeen := map[string][]string{} // prefix+base → variant columns
	choiceCase := map[string]bool{}     // prefix+base → some variant is not a CASE expression

	for _, it := range items {
		segs := strings.Split(it.Alias, ".")
		w := walk(pt, elem, segs)
		if !w.ok {
			msg := fmt.Sprintf("column is not an element of %s", path)
			if w.reason == "primitive" {
				msg = fmt.Sprintf("%q is a primitive element and has no sub-element", w.bad)
			} else if w.suggest != "" {
				msg += fmt.Sprintf(" (did you mean %s?)", w.suggest)
			}
			code := CodeColUnknown
			if w.reason == "primitive" {
				code = CodeColPrimitive
			}
			c.add(Error, code, stmt, path, it.Alias, msg, 0, "")
			continue
		}
		for _, cs := range w.cased {
			c.add(Warning, CodeColCase, stmt, path, it.Alias, "FHIR names are case-sensitive; Go's decoder tolerates it but other consumers will not: "+cs, 0, "")
		}
		f := w.leaf
		rule, hasRule := v.rule(w.elem)

		own := owners[w.prefix]
		if own == nil {
			own = map[string]selectItem{}
			owners[w.prefix] = own
			ownerType[w.prefix] = w.owner.typ.Name()
		}
		own[f.Name] = it

		if hasRule && rule.Prohibited() {
			c.add(Error, CodeColProhibited, stmt, path, it.Alias, "profile prohibits this element (max = 0)", 0, "")
			continue
		}
		if f.ChoiceBase != "" {
			key := w.prefix + "|" + f.ChoiceBase
			choiceSeen[key] = append(choiceSeen[key], it.Alias)
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(it.Expr)), "case") {
				choiceCase[key] = true
			}
			if hasRule && len(rule.Types) > 0 && !containsFold(rule.Types, f.ChoiceType) {
				c.add(Warning, CodeColChoiceType, stmt, path, it.Alias,
					fmt.Sprintf("profile allows %s[x] only as %s, not %s", f.ChoiceBase, strings.Join(rule.Types, "/"), f.ChoiceType), 0, "")
			}
		}
		if f.Struct {
			c.add(Error, CodeColComplex, stmt, path, it.Alias,
				fmt.Sprintf("%s is a complex %s element; a single value cannot fill it. Use dotted columns (%s.<field>) or a separate statement with fhir_path '%s.%s'",
					f.Name, f.Elem.Name(), f.Name, path, f.Name), 0, "")
			continue
		}

		typ := v.expectedPrimitive(w, rule, hasRule)
		required := (hasRule && rule.Min >= 1) || (f.Required && f.ChoiceBase == "")
		v.checkExpression(c, stmt, path, it, f, typ, rule, hasRule, required)
		if w.owner.typ.Name() == "Reference" && f.Name == "reference" {
			v.checkReference(c, stmt, path, it, strings.TrimSuffix(w.elem, ".reference"))
		}
	}

	for key, cols := range choiceSeen {
		// CASE variants are normally written to be mutually exclusive, which
		// cannot be proven statically: only flag when a plain column is involved.
		if len(cols) > 1 && choiceCase[key] {
			base := key[strings.Index(key, "|")+1:]
			sort.Strings(cols)
			c.add(Warning, CodeColChoiceMulti, stmt, path, strings.Join(cols, ", "),
				fmt.Sprintf("%s[x] may only hold one type, but the query selects %d variants; rows filling more than one are invalid", base, len(cols)), 0, "")
		}
	}

	for prefix, cols := range owners {
		v.checkOwner(c, stmt, path, prefix, ownerType[prefix], cols)
	}
}

var (
	reCast      = regexp.MustCompile(`(?is)^\s*(?:try_)?cast\s*\(.*\s+as\s+(\w+)`)
	reConvert   = regexp.MustCompile(`(?is)^\s*(?:try_)?convert\s*\(\s*(\w+)`)
	reRefPrefix = regexp.MustCompile(`(?is)^\s*(?:concat\s*\(\s*)?'([^'/]*)/`)
)

func castClass(t string) string {
	switch strings.ToLower(t) {
	case "int", "integer", "bigint", "smallint", "tinyint":
		return "int"
	case "real", "float", "double", "decimal", "numeric", "number", "money":
		return "num"
	case "text", "varchar", "nvarchar", "char", "nchar", "string", "clob":
		return "text"
	case "date":
		return "date"
	case "datetime", "datetime2", "timestamp", "smalldatetime", "datetimeoffset":
		return "datetime"
	case "bit", "boolean", "bool":
		return "bool"
	}
	return ""
}

func primitiveClass(typ string) string {
	switch typ {
	case "integer", "positiveInt", "unsignedInt":
		return "int"
	case "decimal":
		return "num"
	case "date":
		return "date"
	case "dateTime", "instant":
		return "datetime"
	case "boolean":
		return "bool"
	case "string", "code", "id", "uri", "url", "canonical", "markdown", "oid", "uuid":
		return "text"
	}
	return ""
}

func (v *Validator) checkExpression(c *collector, stmt int, path string, it selectItem, f field, typ string,
	rule converter.ElementRule, hasRule, required bool) {

	if it.isNull() {
		if required {
			c.add(Warning, CodeLitNull, stmt, path, it.Alias, "required element is always NULL", 0, "")
		}
		return
	}

	// CAST / CONVERT target against the element type.
	var target string
	if m := reCast.FindStringSubmatch(it.Expr); m != nil {
		target = castClass(m[1])
	} else if m := reConvert.FindStringSubmatch(it.Expr); m != nil {
		target = castClass(m[1])
	}
	if want := primitiveClass(typ); target != "" && target != "text" && want != "" && want != "text" && target != want && !(want == "num" && target == "int") {
		c.add(Warning, CodeLitCast, stmt, path, it.Alias,
			fmt.Sprintf("expression casts to %s but the element is %s (%s)", target, typ, want), 0, trunc(it.Expr, 60))
	}

	lit, ok := it.literal()
	if !ok {
		return
	}
	if hasRule && rule.Fixed != "" && lit != rule.Fixed {
		c.add(Error, CodeLitFixed, stmt, path, it.Alias, fmt.Sprintf("profile fixes this element to %q but the query selects %q", rule.Fixed, lit), 0, "")
	}
	if len(f.Enum) > 0 && !contains(f.Enum, lit) {
		c.add(Error, CodeLitCode, stmt, path, it.Alias,
			fmt.Sprintf("%q is not a valid code (allowed: %s); only a ConceptMap could still translate it", lit, strings.Join(f.Enum, ", ")), 0, "")
		return
	}
	if msg := checkLiteral(typ, lit); msg != "" {
		c.add(Error, CodeLitValue, stmt, path, it.Alias, fmt.Sprintf("literal %q is not a valid FHIR %s: %s", lit, typ, msg), 0, "")
	}
}

func (v *Validator) checkReference(c *collector, stmt int, path string, it selectItem, refElem string) {
	m := reRefPrefix.FindStringSubmatch(it.Expr)
	if m == nil {
		return // dynamic: nothing to say statically
	}
	rt := m[1]
	if !resourceTypes[rt] {
		msg := fmt.Sprintf("reference prefix %q is not a FHIR resource type", rt+"/")
		if s := closest(rt, keys(resourceTypes)); s != "" {
			msg += fmt.Sprintf(" (did you mean %s?)", s)
		}
		c.add(Error, CodeLitReference, stmt, path, it.Alias, msg, 0, "")
		return
	}
	rule, ok := v.rule(refElem)
	if !ok || len(rule.TargetProfiles) == 0 {
		return
	}
	var allowed []string
	for _, tp := range rule.TargetProfiles {
		name := tp[strings.LastIndex(tp, "/")+1:]
		if name == "Resource" || name == rt {
			return
		}
		allowed = append(allowed, name)
	}
	c.add(Warning, CodeLitReference, stmt, path, it.Alias,
		fmt.Sprintf("profile allows %s to reference only %s, not %s", refElem, strings.Join(allowed, "/"), rt), 0, "")
}

// checkOwner applies group rules to the columns that belong to one complex
// element (a path itself, or a dotted prefix such as "valueQuantity").
func (v *Validator) checkOwner(c *collector, stmt int, path, prefix, typeName string, cols map[string]selectItem) {
	has := func(n string) bool { _, ok := cols[n]; return ok }
	label := path
	if prefix != "" {
		label += "." + prefix
	}
	switch typeName {
	case "Coding":
		if has("code") && !has("system") {
			c.add(Warning, CodeSemCoding, stmt, label, "", "Coding has a code but no system; the code cannot be interpreted unambiguously", 0, "")
		}
		if has("display") && !has("code") {
			c.add(Warning, CodeSemCoding, stmt, label, "", "Coding has a display but no code", 0, "")
		}
		if has("system") && !has("code") {
			c.add(Warning, CodeSemCoding, stmt, label, "", "Coding has a system but no code", 0, "")
		}
	case "Identifier":
		if has("value") && !has("system") {
			c.add(Warning, CodeSemIdentifier, stmt, label, "", "Identifier has a value but no system; the value is only unique within its namespace", 0, "")
		}
		if has("system") && !has("value") {
			c.add(Warning, CodeSemIdentifier, stmt, label, "", "Identifier has a system but no value", 0, "")
		}
	case "Quantity", "Age", "Duration", "Distance", "Count":
		if has("value") && !has("unit") && !has("code") {
			c.add(Warning, CodeSemQuantity, stmt, label, "", "Quantity has a value but neither unit nor code; the number has no meaning", 0, "")
		}
		if has("code") && !has("system") {
			c.add(Warning, CodeSemQuantity, stmt, label, "", "Quantity has a code but no system (normally http://unitsofmeasure.org)", 0, "")
		}
		if has("system") && !has("code") {
			c.add(Warning, CodeSemQuantity, stmt, label, "", "Quantity has a system but no code", 0, "")
		}
	case "Reference":
		if has("display") && !has("reference") && !has("identifier") {
			c.add(Info, CodeSemReference, stmt, label, "", "Reference has only a display: valid, but it cannot be resolved to a resource", 0, "")
		}
	}
}

// checkCoverage reports elements the model or profile asks for that the
// query does not provide. Elements count as provided by a direct column, a
// dotted column prefix, or a statement for a child path.
func (v *Validator) checkCoverage(c *collector, stmt int, path string, pt reflect.Type, elem string, items []selectItem, allPaths []string) {
	leafSet, dotted := map[string]bool{}, map[string]bool{}
	add := func(name string, isLeaf bool) {
		parts := strings.Split(strings.ToLower(name), ".")
		for k := range parts {
			key := strings.Join(parts[:k+1], ".")
			if k == len(parts)-1 && isLeaf {
				leafSet[key] = true
			} else {
				dotted[key] = true
			}
		}
	}
	for _, it := range items {
		add(it.Alias, true)
	}
	children := map[string]bool{}
	for _, p := range allPaths {
		if strings.HasPrefix(p, path+".") {
			rel := p[len(path)+1:]
			add(rel, false)
			for k := range strings.Split(strings.ToLower(rel), ".") {
				children[strings.Join(strings.Split(strings.ToLower(rel), ".")[:k+1], ".")] = true
			}
		}
	}
	provided := func(name string) bool {
		n := strings.ToLower(name)
		return leafSet[n] || dotted[n] || children[n]
	}

	source := "FHIR R4"
	if v.Profile != nil && v.Profile.HasProfileFor(strings.SplitN(path, ".", 2)[0]) {
		source = "the profile"
	}

	var visit func(label string, t reflect.Type, prefix string, depth int)
	visit = func(label string, t reflect.Type, prefix string, depth int) {
		si := structOf(t)
		if si == nil || depth > 6 {
			return
		}
		doneChoice := map[string]bool{}
		for _, f := range si.fields {
			names := []string{f.Name}
			key := f.Name
			if f.ChoiceBase != "" {
				if doneChoice[f.ChoiceBase] {
					continue
				}
				doneChoice[f.ChoiceBase] = true
				key, names = f.ChoiceBase+"[x]", nil
				for _, i := range si.choice[f.ChoiceBase] {
					names = append(names, si.fields[i].Name)
				}
			}
			if prefix == "" && f.Name == "id" {
				continue // supplied by the id column
			}
			rule, has := v.rule(label + "." + key)
			required := (has && rule.Min >= 1) || (!has && f.Required && f.ChoiceBase == "")
			prov := false
			for _, n := range names {
				if provided(prefix + n) {
					prov = true
				}
			}
			where := label + "." + key
			switch {
			case !prov && required:
				c.add(Warning, CodeProfRequired, stmt, path, key,
					fmt.Sprintf("%s requires %s (min 1) but the query does not select it", source, where), 0, "")
			case !prov && has && rule.Populate() != "":
				c.add(Warning, CodeProfObligation, stmt, path, key,
					fmt.Sprintf("profile obligation %s for the exposing system, but the query does not select %s", rule.Populate(), where), 0, "")
			case !prov && has && rule.MustSupport:
				c.add(Info, CodeProfMustSupport, stmt, path, key,
					fmt.Sprintf("profile marks %s as mustSupport but the query does not select it", where), 0, "")
			case prov && f.Struct && f.ChoiceBase == "" && dotted[strings.ToLower(prefix+f.Name)] && !children[strings.ToLower(prefix+f.Name)]:
				visit(label+"."+key, f.Elem, prefix+f.Name+".", depth+1)
			}
		}
	}
	visit(elem, pt, "", 0)
}

// checkLiteral validates a constant against a FHIR primitive; "" means valid
// or not checkable.
func checkLiteral(typ, v string) string {
	m := func(re, msg string) string {
		if regexp.MustCompile(re).MatchString(v) {
			return ""
		}
		return msg
	}
	switch typ {
	case "date":
		return m(`^\d{4}(-(0[1-9]|1[0-2])(-(0[1-9]|[12]\d|3[01]))?)?$`, "expected YYYY, YYYY-MM or YYYY-MM-DD")
	case "dateTime":
		return m(`^\d{4}(-(0[1-9]|1[0-2])(-(0[1-9]|[12]\d|3[01])(T([01]\d|2[0-3]):[0-5]\d:([0-5]\d|60)(\.\d+)?(Z|[+-]((0\d|1[0-3]):[0-5]\d|14:00)))?)?)?$`,
			"expected YYYY-MM-DD or YYYY-MM-DDThh:mm:ss+hh:mm (a time needs 'T' and a timezone)")
	case "instant":
		return m(`^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])T([01]\d|2[0-3]):[0-5]\d:([0-5]\d|60)(\.\d+)?(Z|[+-]((0\d|1[0-3]):[0-5]\d|14:00))$`, "expected YYYY-MM-DDThh:mm:ss+hh:mm")
	case "time":
		return m(`^([01]\d|2[0-3]):[0-5]\d:([0-5]\d|60)(\.\d+)?$`, "expected hh:mm:ss")
	case "boolean":
		return m(`^(true|false)$`, "expected true or false")
	case "integer", "unsignedInt", "positiveInt":
		return m(`^-?(0|[1-9]\d*)$`, "expected a whole number")
	case "decimal":
		return m(`^-?(0|[1-9]\d*)(\.\d+)?([eE][+-]?\d+)?$`, "expected a number with a decimal point, not a comma")
	case "code":
		return m(`^[^\s]+(\s[^\s]+)*$`, "no leading/trailing or repeated whitespace allowed")
	case "id":
		return m(`^[A-Za-z0-9\-\.]{1,64}$`, "1-64 characters from A-Z a-z 0-9 - .")
	case "uri", "url", "canonical":
		if strings.ContainsAny(v, " \t\r\n") {
			return "must not contain whitespace"
		}
	}
	return ""
}

func trunc(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
