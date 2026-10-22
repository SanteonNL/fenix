package fhirserver

import (
	"fmt"
	"net/url"
	"strconv"
)

// maxPageSize caps _count. It also keeps a page's id list well inside the
// size querycompiler.RestrictToResources accepts.
const maxPageSize = 5000

// parsePaging reads _count and _offset from the request's search params.
// count 0 means the request is not paged (the whole result is returned, as
// before paging existed); a count above maxPageSize is lowered to it.
func parsePaging(params map[string]string) (count, offset int, err error) {
	if v, ok := params["_count"]; ok {
		count, err = strconv.Atoi(v)
		if err != nil || count < 0 {
			return 0, 0, fmt.Errorf("invalid _count %q: want a non-negative integer", v)
		}
		count = min(count, maxPageSize)
	}
	if v, ok := params["_offset"]; ok {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("invalid _offset %q: want a non-negative integer", v)
		}
	}
	return count, offset, nil
}

// slicePage returns the resources[offset:offset+count] window, empty when offset is past the end.
func slicePage(resources []interface{}, offset, count int) []interface{} {
	if offset >= len(resources) {
		return []interface{}{}
	}
	return resources[offset:min(offset+count, len(resources))]
}

// pagingLinks builds the Bundle.link entries for a page: self, and next while
// more results follow. The links keep the request's own path and params, only
// _offset and _count change.
func pagingLinks(req *url.URL, offset, count, total int) []map[string]string {
	link := func(relation string, off int) map[string]string {
		q := req.Query()
		q.Set("_count", strconv.Itoa(count))
		q.Set("_offset", strconv.Itoa(off))
		u := url.URL{Path: req.Path, RawQuery: q.Encode()}
		return map[string]string{"relation": relation, "url": u.String()}
	}
	links := []map[string]string{link("self", offset)}
	if offset+count < total {
		links = append(links, link("next", offset+count))
	}
	return links
}
