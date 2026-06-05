package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Runtime template fetch — the `specscore <type> new` scaffolders pull the
// canonical template from the published gallery (specscore.md/new/<type>.md)
// for a *bare* scaffold, falling back to the embedded scaffolder offline.
// Spec: spec/features/cli-template-runtime-fetch (in the specscore repo).

// templateFetchTimeout bounds the network wait before the CLI abandons the
// fetch and uses the embedded template. #req:bounded-timeout. A var (not a
// const) so tests can lower it to exercise the timeout path quickly.
var templateFetchTimeout = 3 * time.Second

// templateHTTPClient is the client used for template fetches; a package var so
// tests can swap it. Per-request deadlines come from a context, not this field.
var templateHTTPClient = &http.Client{}

// templateBaseURL resolves the gallery base URL. Defaults to the public site;
// override via SPECSCORE_TEMPLATE_BASE_URL (testing / self-hosted mirrors).
// #req:url-mapping.
func templateBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("SPECSCORE_TEMPLATE_BASE_URL")); v != "" {
		return v
	}
	return "https://specscore.md"
}

// fetchTemplate issues GET <base>/new/<typ>.md within templateFetchTimeout.
// #req:fetch-at-create-time, #req:url-mapping.
func fetchTemplate(typ string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), templateFetchTimeout)
	defer cancel()

	url := strings.TrimRight(templateBaseURL(), "/") + "/new/" + typ + ".md"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := templateHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// resolveBareTemplate fetches the published template for typ and applies the
// given placeholder→value substitutions. ok is false on ANY failure (network
// error, timeout, non-200), signalling the caller to use the embedded
// scaffolder. #req:field-substitution, #req:embedded-fallback.
func resolveBareTemplate(typ string, repl map[string]string) (body []byte, ok bool) {
	raw, err := fetchTemplate(typ)
	if err != nil {
		return nil, false
	}
	s := string(raw)
	for k, v := range repl {
		s = strings.ReplaceAll(s, k, v)
	}
	return []byte(s), true
}

// fellBackWarning is the one-line stderr notice printed when a fetch was
// attempted but failed and the embedded template was used. #req:fallback-notice.
const fellBackWarning = "warning: specscore.md unreachable, used built-in template"

// bareOrEmbedded returns the body for a scaffold. When bare, it fetches the
// published template for typ and applies repl; on any fetch failure it prints
// the fallback warning to errw and uses embeddedFn. When not bare (the caller
// supplied authored content the static template can't carry) it always uses
// embeddedFn, with no fetch and no warning.
func bareOrEmbedded(bare bool, typ string, repl map[string]string, errw io.Writer, embeddedFn func() ([]byte, error)) ([]byte, error) {
	if bare {
		if t, ok := resolveBareTemplate(typ, repl); ok {
			return t, nil
		}
		_, _ = fmt.Fprintln(errw, fellBackWarning)
	}
	return embeddedFn()
}

// --- shared field defaults, matching the embedded scaffolders' behaviour ---

func templateTitleOrSlug(title, slug string) string {
	if strings.TrimSpace(title) != "" {
		return title
	}
	return templateTitleCaseFromSlug(slug)
}

func templateOwnerOrUnknown(owner string) string {
	if strings.TrimSpace(owner) != "" {
		return owner
	}
	return "unknown"
}

func templateTodayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}

func templateNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// templateTitleCaseFromSlug mirrors pkg/idea.titleCaseFromSlug so the fetched
// bare template gets the same default title as the embedded scaffolder.
func templateTitleCaseFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
