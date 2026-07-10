package probe

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// stubHTTP replaces HTTPGetFn for the duration of a test with a function
// dispatching on the requested URL.
func stubHTTP(t *testing.T, fn func(url string) (*http.Response, error)) {
	t.Helper()
	old := HTTPGetFn
	HTTPGetFn = fn
	t.Cleanup(func() { HTTPGetFn = old })
}

// okResponse builds a minimal *http.Response with the given status code and a
// non-nil body (probeDomain closes it).
func okResponse(code int) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}
}

// declaredServesStatus is a declared serves-status fact for a domain, the raw
// material the domain kind reads its targets from.
func declaredServesStatus(domain, status, ecosystem string) fact.Fact {
	return fact.Fact{
		Subject:    domain,
		Predicate:  "serves-status",
		Object:     status,
		Evidence:   fact.Evidence{Class: fact.Declared, Pointer: "domains.json"},
		Adapter:    fact.Adapter{ID: "registries", Version: "0.1.0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		VerifiedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  ecosystem,
	}
}

func TestRunDomain_HTTPSAnswered(t *testing.T) {
	stubHTTP(t, func(url string) (*http.Response, error) {
		if url != "https://example.app/" {
			t.Fatalf("unexpected URL %q — https should be tried first", url)
		}
		return okResponse(200), nil
	})
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	res := RunDomain([]fact.Fact{declaredServesStatus("example.app", "200", "demo")}, "9.9.9", now)

	if got := res.Kinds; len(got) != 1 || got[0] != KindDomain {
		t.Fatalf("Kinds = %v, want [%s]", got, KindDomain)
	}
	if len(res.Facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(res.Facts))
	}
	f := res.Facts[0]
	if f.Subject != "example.app" || f.Predicate != "serves-status" || f.Object != "200" {
		t.Errorf("triple = (%s,%s,%s), want (example.app,serves-status,200)", f.Subject, f.Predicate, f.Object)
	}
	if f.Class != fact.VerifiedBehavior {
		t.Errorf("class = %s, want verified-behavior", f.Class)
	}
	if f.Pointer != "https://example.app/" {
		t.Errorf("pointer = %q, want the probed https URL", f.Pointer)
	}
	if f.Adapter.ID != DomainAdapterID || f.Adapter.Version != "9.9.9" {
		t.Errorf("adapter = %+v, want {probe-domain 9.9.9}", f.Adapter)
	}
	want := now.Format(time.RFC3339)
	if f.ObservedAt != want || f.VerifiedAt != want {
		t.Errorf("stamps = (%s,%s), want both %s", f.ObservedAt, f.VerifiedAt, want)
	}
	if f.Ecosystem != "demo" {
		t.Errorf("ecosystem = %q, want demo (carried from the declared fact)", f.Ecosystem)
	}
}

func TestRunDomain_5xxIsAResponseNotDown(t *testing.T) {
	stubHTTP(t, func(string) (*http.Response, error) { return okResponse(503), nil })
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	res := RunDomain([]fact.Fact{declaredServesStatus("example.app", "200", "demo")}, "1", now)

	if res.Facts[0].Object != "503" {
		t.Errorf("object = %q, want 503 (a 5xx is a response, not down)", res.Facts[0].Object)
	}
}

func TestRunDomain_HTTPFallbackAfterHTTPSTransportFailure(t *testing.T) {
	var tried []string
	stubHTTP(t, func(url string) (*http.Response, error) {
		tried = append(tried, url)
		if strings.HasPrefix(url, "https://") {
			return nil, errors.New("tls handshake timeout")
		}
		return okResponse(200), nil
	})
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	res := RunDomain([]fact.Fact{declaredServesStatus("http-only.test", "200", "demo")}, "1", now)

	if len(tried) != 2 || tried[0] != "https://http-only.test/" || tried[1] != "http://http-only.test/" {
		t.Fatalf("scheme order = %v, want https then http", tried)
	}
	f := res.Facts[0]
	if f.Object != "200" {
		t.Errorf("object = %q, want 200 from the http fallback", f.Object)
	}
	if f.Pointer != "http://http-only.test/" {
		t.Errorf("pointer = %q, want the http URL that actually answered", f.Pointer)
	}
}

func TestRunDomain_BothSchemesFailIsDown(t *testing.T) {
	stubHTTP(t, func(string) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection\nrefused")
	})
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	res := RunDomain([]fact.Fact{declaredServesStatus("dead.example", "200", "demo")}, "1", now)

	f := res.Facts[0]
	if f.Object != DownObject {
		t.Errorf("object = %q, want down", f.Object)
	}
	if f.Class != fact.VerifiedBehavior {
		t.Errorf("class = %s, want verified-behavior", f.Class)
	}
	if !strings.HasPrefix(f.Pointer, "https://dead.example/") {
		t.Errorf("pointer = %q, want the attempted https URL prefix", f.Pointer)
	}
	if !strings.Contains(f.Pointer, "connection refused") {
		t.Errorf("pointer = %q, want the one-lined failure reason in its detail", f.Pointer)
	}
}

func TestRunDomain_TargetsFromServesStatusOnly(t *testing.T) {
	stubHTTP(t, func(string) (*http.Response, error) { return okResponse(200), nil })
	now := time.Now()

	facts := []fact.Fact{
		declaredServesStatus("b.example", "200", "demo"),
		declaredServesStatus("a.example", "200", "demo"),
		{Subject: "prod", Predicate: "fronts", Object: "b.example",
			Evidence: fact.Evidence{Class: fact.Declared}, Ecosystem: "demo"},
	}
	res := RunDomain(facts, "1", now)

	if len(res.Facts) != 2 {
		t.Fatalf("got %d facts, want 2 (one per distinct serves-status subject)", len(res.Facts))
	}
	// Deterministic sorted order: a before b.
	if res.Facts[0].Subject != "a.example" || res.Facts[1].Subject != "b.example" {
		t.Errorf("subjects = [%s %s], want sorted [a.example b.example]",
			res.Facts[0].Subject, res.Facts[1].Subject)
	}
}

func TestRunDomain_DistinctDomainProbedOnce(t *testing.T) {
	var calls int
	stubHTTP(t, func(string) (*http.Response, error) {
		calls++
		return okResponse(200), nil
	})
	facts := []fact.Fact{
		declaredServesStatus("dup.example", "200", "demo"),
		declaredServesStatus("dup.example", "500", "demo"), // second fact, same subject
	}
	res := RunDomain(facts, "1", time.Now())

	if len(res.Facts) != 1 {
		t.Fatalf("got %d facts, want 1 for a single distinct domain", len(res.Facts))
	}
	if calls != 1 {
		t.Errorf("HTTPGetFn called %d times, want 1 (probe each domain once)", calls)
	}
}

func TestRunDomain_NoTargetsNoFacts(t *testing.T) {
	stubHTTP(t, func(string) (*http.Response, error) {
		t.Fatal("HTTPGetFn should not be called when there are no serves-status targets")
		return nil, nil
	})
	res := RunDomain([]fact.Fact{
		{Subject: "x", Predicate: "imports", Object: "y", Ecosystem: "demo"},
	}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0", len(res.Facts))
	}
	if len(res.Kinds) != 1 || res.Kinds[0] != KindDomain {
		t.Errorf("Kinds = %v, want [domain] even with no targets", res.Kinds)
	}
}

// TestDefaultHTTPGetFn exercises the real (unstubbed) seam against a closed
// port so the default client path is covered without a network dependency:
// the request fails fast with a transport error and no panic.
func TestDefaultHTTPGetFn(t *testing.T) {
	// 127.0.0.1:0 is never a listening port; the dial fails immediately.
	resp, err := HTTPGetFn("http://127.0.0.1:0/")
	if err == nil {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("expected a transport error dialing a closed port")
	}
}
