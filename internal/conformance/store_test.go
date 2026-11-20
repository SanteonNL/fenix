package conformance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
)

func newTestStore(t *testing.T, resources ...string) *Store {
	t.Helper()
	s := NewStore(zerolog.Nop())
	for i, r := range resources {
		if _, err := s.Add([]byte(r), fmt.Sprintf("res-%d", i)); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	return s
}

const (
	vsCodes = `{"resourceType":"ValueSet","id":"codes","url":"https://ex.org/ValueSet/codes","version":"1.0.0","status":"active",
		"compose":{"include":[{"system":"http://loinc.org","concept":[{"code":"1-1","display":"One"},{"code":"2-2","display":"Two"}]}]}}`
	vsCodesV2 = `{"resourceType":"ValueSet","id":"codes-v2","url":"https://ex.org/ValueSet/codes","version":"2.0.0","status":"active",
		"compose":{"include":[{"system":"http://loinc.org","concept":[{"code":"3-3"}]}]}}`
	csComplete = `{"resourceType":"CodeSystem","id":"cs","url":"https://ex.org/cs","content":"complete",
		"concept":[{"code":"A","display":"a","concept":[{"code":"A1"}]},{"code":"B"}]}`
	vsNested = `{"resourceType":"ValueSet","id":"nested","url":"https://ex.org/ValueSet/nested",
		"compose":{"include":[{"valueSet":["https://ex.org/ValueSet/codes|1.0.0"]},{"system":"https://ex.org/cs"}],
		"exclude":[{"system":"https://ex.org/cs","concept":[{"code":"B"}]}]}}`
	vsECL = `{"resourceType":"ValueSet","id":"smoking","url":"https://ex.org/ValueSet/smoking",
		"compose":{"include":[{"system":"http://snomed.info/sct","filter":[{"property":"constraint","op":"=","value":"<< 77176002"}]}]}}`
)

func TestStoreSearchAndResolve(t *testing.T) {
	s := newTestStore(t, vsCodes, vsCodesV2, csComplete)

	if e, ok := s.Resolve("ValueSet", "https://ex.org/ValueSet/codes"); !ok || e.Version != "2.0.0" {
		t.Fatalf("without version the highest should win, got %+v", e)
	}
	if e, ok := s.Resolve("ValueSet", "https://ex.org/ValueSet/codes|1.0.0"); !ok || e.ID != "codes" {
		t.Fatalf("pinned version, got %+v", e)
	}
	if _, ok := s.Resolve("ValueSet", "ValueSet/codes-v2"); !ok {
		t.Fatal("reference by id should resolve")
	}
	found, err := s.Search("ValueSet", map[string]string{"url": "https://ex.org/ValueSet/codes"})
	if err != nil || len(found) != 2 {
		t.Fatalf("search by url: %d, %v", len(found), err)
	}
	found, _ = s.Search("ValueSet", map[string]string{"url": "https://ex.org/ValueSet/codes|1.0.0"})
	if len(found) != 1 {
		t.Fatalf("search by url|version: %d", len(found))
	}
	if _, err := s.Search("ValueSet", map[string]string{"publisher": "x"}); err == nil {
		t.Fatal("unsupported search parameter should be refused")
	}
	if ok, _ := s.Add([]byte(`{"resourceType":"Patient","id":"p"}`), "p"); ok {
		t.Fatal("non-conformance resources are not stored")
	}
}

func TestExpandLocal(t *testing.T) {
	s := newTestStore(t, vsCodes, vsCodesV2, csComplete, vsNested)
	x, err := s.Expand("https://ex.org/ValueSet/nested")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range x.Concepts {
		got = append(got, c.System+"|"+c.Code)
	}
	want := "http://loinc.org|1-1,http://loinc.org|2-2,https://ex.org/cs|A,https://ex.org/cs|A1"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
	if x.Source != "local" {
		t.Fatalf("source %q", x.Source)
	}
	m, _ := s.Members("https://ex.org/ValueSet/codes|1.0.0")
	if !m.Contains("http://loinc.org", "1-1") || m.Contains("http://snomed.info/sct", "1-1") || !m.Contains("", "2-2") {
		t.Fatalf("membership wrong: %+v", m)
	}
}

func TestExpandNeedsServer(t *testing.T) {
	s := newTestStore(t, vsECL)
	if _, err := s.Expand("https://ex.org/ValueSet/smoking"); err == nil || !strings.Contains(err.Error(), "no terminology server") {
		t.Fatalf("a filter without a server should fail clearly, got %v", err)
	}
	if _, err := s.Expand("https://ex.org/ValueSet/unknown"); err == nil || !strings.Contains(err.Error(), "is not loaded") {
		t.Fatalf("unknown ValueSet, got %v", err)
	}
}

// fakeTerminologyServer answers ValueSet/$expand in pages of 2 and requires a token from the password grant.
func fakeTerminologyServer(t *testing.T, calls *int32) *httptest.Server {
	codes := []string{"77176002", "8392000", "8517006"}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != "password" || r.PostForm.Get("username") != "u" || r.PostForm.Get("password") != "p" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
	})
	mux.HandleFunc("/fhir/ValueSet/$expand", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(calls, 1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		end := offset + 2
		if end > len(codes) {
			end = len(codes)
		}
		var contains []map[string]string
		for _, c := range codes[offset:end] {
			contains = append(contains, map[string]string{"system": "http://snomed.info/sct", "code": c})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"resourceType": "ValueSet",
			"expansion": map[string]any{"total": len(codes), "contains": contains}})
	})
	return httptest.NewServer(mux)
}

func TestExpandOnTerminologyServer(t *testing.T) {
	var calls int32
	ts := fakeTerminologyServer(t, &calls)
	defer ts.Close()
	t.Setenv("TS_USER", "u")
	t.Setenv("TS_PASS", "p")

	s := newTestStore(t, vsECL, vsCodes)
	s.AddRemote(&RemoteServer{Name: "nts", BaseURL: ts.URL + "/fhir", Systems: []string{"http://snomed.info/sct"},
		Auth: Auth{Type: "password", TokenURL: ts.URL + "/token", ClientID: "cli", UsernameEnv: "TS_USER", PasswordEnv: "TS_PASS"}})

	// loaded locally but needs a filter: goes to the server, paged
	x, err := s.Expand("https://ex.org/ValueSet/smoking")
	if err != nil {
		t.Fatal(err)
	}
	if x.Source != "nts" || len(x.Concepts) != 3 || x.Entry == nil {
		t.Fatalf("got %+v", x)
	}
	if calls != 2 {
		t.Fatalf("expected 2 pages, got %d calls", calls)
	}
	// cached
	if _, err := s.Expand("https://ex.org/ValueSet/smoking"); err != nil || calls != 2 {
		t.Fatalf("second expand should come from the cache (calls %d, err %v)", calls, err)
	}
	// an implicit SNOMED ValueSet is routed by its system
	if x, err := s.Expand("http://snomed.info/sct?fhir_vs=ecl/<<77176002"); err != nil || x.Source != "nts" {
		t.Fatalf("implicit ValueSet: %+v, %v", x, err)
	}
	// a local ValueSet with listed codes never goes to the server
	if x, err := s.Expand("https://ex.org/ValueSet/codes"); err != nil || x.Source != "local" {
		t.Fatalf("local ValueSet: %+v, %v", x, err)
	}
}

func TestRemoteRoutingByValueSetPrefix(t *testing.T) {
	s := newTestStore(t)
	r := &RemoteServer{Name: "decor", BaseURL: "http://unused", ValueSets: []string{"http://decor.nictiz.nl/"}}
	s.AddRemote(r)
	if got := s.remoteFor("http://decor.nictiz.nl/fhir/ValueSet/2.16.840", ""); got != r {
		t.Fatal("prefix should route to the server")
	}
	if got := s.remoteFor("https://ig.santeon.nl/sim-on-fhir/ValueSet/x", ""); got != nil {
		t.Fatal("other urls should not be routed")
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"1.10.0", "1.9.0", 1}, {"1.0.0", "1.0.0", 0}, {"0.1.0", "0.2.0", -1}, {"1.0", "1.0.1", -1}} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
