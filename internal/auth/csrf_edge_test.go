package auth

// Edge cases for the double-submit CSRF guard, the token it issues, and the
// constant-time comparison both it and the OIDC callback depend on.

import (
	"encoding/base64"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// withSecureCookies pins the package-level flag for one test and puts it back
// afterwards, so tests that assert on the CSRF cookie's Secure attribute do not
// depend on which sealer another test happened to build last.
func withSecureCookies(t *testing.T, secure bool) {
	t.Helper()
	previous := secureCookies
	t.Cleanup(func() { SetSecureCookies(previous) })
	SetSecureCookies(secure)
}

// formPost builds a state-changing request. Unlike the helper in csrf_test.go
// this one can set an *empty* cookie value, which is the case that matters
// most below.
func formPost(method string, cookie *string, fetchSite, body string) *http.Request {
	r := httptest.NewRequest(method, "/arrangementer", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		r.AddCookie(&http.Cookie{Name: csrfCookie, Value: *cookie})
	}
	if fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	return r
}

func encodedField(value string) string {
	return url.Values{CSRFField: {value}}.Encode()
}

// reached reports whether the guarded handler ran. A CSRF test that only checks
// the status code can pass while the handler has already had its side effect.
func reached(t *testing.T, r *http.Request) (int, bool) {
	t.Helper()
	var ran bool
	rec := httptest.NewRecorder()
	CSRF(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)
	return rec.Code, ran
}

// ── The empty-token bypass ────────────────────────────────────────────────

// This is the single most important case in this file.
//
// constantTimeEqual("", "") is true — subtle.ConstantTimeCompare returns 1 for
// two zero-length slices. So if the guard ever stops rejecting an empty cookie
// before it compares, every cross-site form post that simply omits the token
// and arrives with an empty cookie would pass the comparison. The emptiness
// check in csrf.go is the only thing standing between here and a universal
// bypass, and this test exists to make removing it fail loudly.
func TestCSRFRejectsAnEmptyCookieAgainstAnEmptyField(t *testing.T) {
	if !constantTimeEqual("", "") {
		t.Fatal("constantTimeEqual no longer treats two empty strings as equal; the reasoning " +
			"in this test needs rewriting, but check first that the CSRF guard is still correct")
	}

	empty := ""
	code, ran := reached(t, formPost(http.MethodPost, &empty, "same-origin", encodedField("")))
	if ran || code != http.StatusForbidden {
		t.Errorf("a post with an empty CSRF cookie and an empty token field was allowed "+
			"(status %d, handler ran %v); because an empty-vs-empty comparison succeeds, "+
			"this is a complete bypass of the double-submit check", code, ran)
	}
}

// The same hole from the other direction: a cookie that is present but empty
// must never match, whatever the body says.
func TestCSRFRejectsAnEmptyCookieAgainstAnyField(t *testing.T) {
	empty := ""
	for _, field := range []string{"", "abc", " "} {
		code, ran := reached(t, formPost(http.MethodPost, &empty, "same-origin", encodedField(field)))
		if ran || code != http.StatusForbidden {
			t.Errorf("an empty CSRF cookie was accepted against field %q (status %d)", field, code)
		}
	}
}

// ── Methods ───────────────────────────────────────────────────────────────

// Every method that can change state must be guarded, not only POST. A route
// added later with PUT or DELETE must not be unprotected by default.
func TestCSRFGuardsEveryUnsafeMethod(t *testing.T) {
	for _, method := range []string{
		http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
		http.MethodConnect, "PROPFIND", "FROBNICATE",
	} {
		t.Run(method, func(t *testing.T) {
			code, ran := reached(t, formPost(method, nil, "same-origin", ""))
			if ran || code == http.StatusOK {
				t.Errorf("%s passed the CSRF guard without a token (status %d); any route "+
					"mounted on this method would be forgeable from another site", method, code)
			}
		})
	}
}

// Reads must never be blocked, including the ones an older browser sends with
// no Sec-Fetch-Site header and no cookie at all.
func TestCSRFExemptsSafeMethods(t *testing.T) {
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace,
	} {
		t.Run(method, func(t *testing.T) {
			r := httptest.NewRequest(method, "/", nil)
			// Even a genuinely cross-site read must go through: a link from
			// another site to an event page is normal traffic.
			r.Header.Set("Sec-Fetch-Site", "cross-site")
			if code, ran := reached(t, r); !ran || code != http.StatusOK {
				t.Errorf("%s was blocked (status %d); ordinary browsing would break", method, code)
			}
		})
	}
}

// ── Sec-Fetch-Site ────────────────────────────────────────────────────────

// Only "cross-site" is a refusal. "none" means the visitor typed the URL or
// used a bookmark, and "same-site" covers a subdomain — both are legitimate and
// blocking them would break real submissions.
func TestCSRFSecFetchSiteHandling(t *testing.T) {
	tests := []struct {
		site string
		want int
		why  string
	}{
		{"same-origin", http.StatusOK, "the normal case: a form on our own page"},
		{"same-site", http.StatusOK, "a subdomain of itemize.no is trusted"},
		{"none", http.StatusOK, "a bookmark or typed URL is not an attack"},
		{"", http.StatusOK, "a browser too old to send the header falls back to the token"},
		{"CROSS-SITE", http.StatusOK, "the header is lower-case by specification; an " +
			"upper-case value is not something a browser sends, and the token still gates it"},
		{"cross-site", http.StatusForbidden, "the attack this header exists to stop"},
	}

	for _, tt := range tests {
		t.Run(tt.site, func(t *testing.T) {
			token := "matching-token"
			code, _ := reached(t, formPost(http.MethodPost, &token, tt.site, encodedField(token)))
			if code != tt.want {
				t.Errorf("Sec-Fetch-Site: %q gave %d, want %d — %s", tt.site, code, tt.want, tt.why)
			}
		})
	}
}

// A cross-site request must be refused before its body is even read, so a
// forged post cannot be used to make the server buffer a large body.
func TestCSRFRefusesCrossSiteBeforeReadingTheBody(t *testing.T) {
	token := "abc"
	body := &countingReader{inner: strings.NewReader(encodedField(token))}

	r := httptest.NewRequest(http.MethodPost, "/arrangementer", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	r.Header.Set("Sec-Fetch-Site", "cross-site")

	rec := httptest.NewRecorder()
	CSRF(okHandler()).ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	if body.reads > 0 {
		t.Error("the body of a cross-site request was read; it should be refused on the " +
			"header alone so an attacker cannot make us buffer anything")
	}
}

type countingReader struct {
	inner io.Reader
	reads int
}

func (c *countingReader) Read(p []byte) (int, error) {
	c.reads++
	return c.inner.Read(p)
}

// ── Bodies ────────────────────────────────────────────────────────────────

// The body is capped so one client cannot occupy Go's 10 MB default per
// request across many connections. Over the cap the request must fail rather
// than be truncated into a form that happens to parse.
func TestCSRFRejectsAnOversizedBody(t *testing.T) {
	token := "abc"
	// Valid form encoding, but far past maxFormBytes.
	body := encodedField(token) + "&filler=" + strings.Repeat("x", maxFormBytes+1)

	code, ran := reached(t, formPost(http.MethodPost, &token, "same-origin", body))
	if ran {
		t.Error("the handler ran on an over-sized body; the size cap is not being enforced")
	}
	if code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 for a body over the %d-byte cap", code, maxFormBytes)
	}
}

// A body just under the cap must still work, or the limit would be rejecting
// legitimate submissions.
func TestCSRFAcceptsABodyJustUnderTheCap(t *testing.T) {
	token := "abc"
	filler := strings.Repeat("x", maxFormBytes-len(encodedField(token))-len("&filler=")-1)
	body := encodedField(token) + "&filler=" + filler

	if code, ran := reached(t, formPost(http.MethodPost, &token, "same-origin", body)); !ran {
		t.Errorf("a body of %d bytes, inside the %d-byte cap, was rejected with %d",
			len(body), maxFormBytes, code)
	}
}

// An unparseable body is a 400, not a 403: the difference matters because a 403
// tells the visitor to reload the page, which would not help.
func TestCSRFReportsAnUnparseableBodyAsABadRequest(t *testing.T) {
	token := "abc"
	code, ran := reached(t, formPost(http.MethodPost, &token, "same-origin", "%zz=%zz"))
	if ran {
		t.Error("the handler ran on a body ParseForm could not read")
	}
	if code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", code)
	}
}

// multipartPost builds a multipart/form-data post carrying one token field, the
// shape a form with a file input would submit.
func multipartPost(cookie, field string) *http.Request {
	body := "--X\r\n" +
		`Content-Disposition: form-data; name="` + CSRFField + `"` + "\r\n\r\n" +
		field + "\r\n--X--\r\n"

	r := httptest.NewRequest(http.MethodPost, "/arrangementer", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: cookie})
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return r
}

// ParseForm does not read a multipart body — it leaves PostForm empty — so the
// guard has to parse one itself or the token would be invisible and every
// multipart post would be refused as expired. No form on the site uses
// multipart today; this test is what will keep the first file upload from
// failing with a message telling the visitor to reload a page that was fine.
func TestCSRFReadsATokenFromAMultipartBody(t *testing.T) {
	code, ran := reached(t, multipartPost("abc", "abc"))
	if !ran {
		t.Errorf("a multipart post carrying the right token was rejected with %d; "+
			"the token in a multipart body must be found, or a form with a file input "+
			"can never be submitted", code)
	}
}

// The multipart path is not a way around the check: a body that carries the
// wrong token must be refused exactly as an urlencoded one is. If it were not,
// an attacker could bypass the guard by changing the form's enctype.
func TestCSRFRejectsAWrongTokenInAMultipartBody(t *testing.T) {
	code, ran := reached(t, multipartPost("abc", "wrong"))
	if ran {
		t.Error("a multipart post with a token that did not match the cookie reached the " +
			"handler; switching a form to enctype=multipart/form-data would then defeat " +
			"the whole double-submit guard")
	}
	if code != http.StatusForbidden {
		t.Errorf("got %d, want 403", code)
	}
}

// The guard parses the body before the handler does, so the handler has to
// still find its own fields there. If parsing consumed the body without
// populating PostForm, every field on a multipart form would arrive empty and
// the handler would silently save nothing.
func TestCSRFLeavesMultipartFieldsReadableByTheHandler(t *testing.T) {
	body := "--X\r\n" +
		`Content-Disposition: form-data; name="` + CSRFField + `"` + "\r\n\r\n" +
		"abc\r\n--X\r\n" +
		`Content-Disposition: form-data; name="tittel"` + "\r\n\r\n" +
		"Julebord\r\n--X--\r\n"

	r := httptest.NewRequest(http.MethodPost, "/arrangementer", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	r.Header.Set("Sec-Fetch-Site", "same-origin")

	var got string
	CSRF(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.FormValue("tittel")
	})).ServeHTTP(httptest.NewRecorder(), r)

	if got != "Julebord" {
		t.Errorf("the handler read %q for the tittel field, want %q; the guard consumed the "+
			"multipart body without leaving its values behind", got, "Julebord")
	}
}

// A multipart body is bounded by the same cap as any other. Parsing one must
// not be a way to make the server hold more than maxFormBytes.
func TestCSRFRejectsAnOversizedMultipartBody(t *testing.T) {
	body := "--X\r\n" +
		`Content-Disposition: form-data; name="` + CSRFField + `"` + "\r\n\r\n" +
		"abc\r\n--X\r\n" +
		`Content-Disposition: form-data; name="filler"; filename="f.bin"` + "\r\n\r\n" +
		strings.Repeat("x", maxFormBytes+1) + "\r\n--X--\r\n"

	r := httptest.NewRequest(http.MethodPost, "/arrangementer", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	r.Header.Set("Sec-Fetch-Site", "same-origin")

	code, ran := reached(t, r)
	if ran {
		t.Error("the handler ran on an over-sized multipart body; the size cap is not " +
			"applied to multipart, so one client could occupy far more memory than the " +
			"urlencoded path allows")
	}
	if code != http.StatusBadRequest {
		t.Errorf("got %d, want 400 for a body over the %d-byte cap", code, maxFormBytes)
	}
}

// A multipart header the parser cannot make sense of is a 400, not a 403 — the
// same distinction the urlencoded path makes, and for the same reason: telling
// the visitor to reload would not help.
func TestCSRFReportsAMalformedMultipartBodyAsABadRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/arrangementer", strings.NewReader("not multipart"))
	// No boundary parameter, so there is nothing to split the body on.
	r.Header.Set("Content-Type", "multipart/form-data")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "abc"})
	r.Header.Set("Sec-Fetch-Site", "same-origin")

	code, ran := reached(t, r)
	if ran {
		t.Error("the handler ran on a multipart body that could not be parsed")
	}
	if code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", code)
	}
}

// The token may appear only in the body. A value in the query string must not
// satisfy the check, because a URL is exactly what an attacker controls when
// they get a browser to issue a request.
func TestCSRFIgnoresATokenInTheQueryString(t *testing.T) {
	token := "abc"
	r := httptest.NewRequest(http.MethodPost, "/arrangementer?"+encodedField(token), strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	r.Header.Set("Sec-Fetch-Site", "same-origin")

	if code, ran := reached(t, r); ran {
		t.Errorf("a token supplied in the URL satisfied the guard (status %d); an attacker "+
			"controls the URL of a request they cause, so only the body may count", code)
	}
}

// ── Token issuance ────────────────────────────────────────────────────────

// The token has to be unguessable: the whole double-submit argument is that an
// attacker cannot learn the cookie's value, so a predictable one defeats it.
func TestCSRFTokensAreUnpredictableAndFullLength(t *testing.T) {
	withSecureCookies(t, true)

	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		token := CSRFToken(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		if token == "" {
			t.Fatal("CSRFToken returned an empty string; every form on the page would then " +
				"carry an empty token and be rejected")
		}
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("token %q is not raw base64url and cannot go in a cookie: %v", token, err)
		}
		if len(raw) != 32 {
			t.Fatalf("token carries %d bytes of entropy, want 32", len(raw))
		}
		if seen[token] {
			t.Fatal("CSRFToken returned a value it had already issued; the token generator " +
				"is not random and the double-submit check is worthless")
		}
		seen[token] = true
	}
}

// The cookie's attributes are deliberate and each one is explained in csrf.go.
// HttpOnly in particular must stay off: the double-submit pattern needs the
// value readable by same-origin script, and the token is not a secret from a
// page that is already same-origin.
func TestCSRFCookieAttributes(t *testing.T) {
	for _, secure := range []bool{true, false} {
		name := "secure deployment"
		if !secure {
			name = "plain-HTTP development"
		}
		t.Run(name, func(t *testing.T) {
			withSecureCookies(t, secure)

			rec := httptest.NewRecorder()
			token := CSRFToken(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			c := cookieNamed(t, rec, csrfCookie)
			if c.Value != token {
				t.Errorf("the cookie carries %q but the form would carry %q, so every post "+
					"would be refused", c.Value, token)
			}
			if c.HttpOnly {
				t.Error("the CSRF cookie is HttpOnly; the double-submit pattern requires " +
					"same-origin script to be able to read it")
			}
			if c.Secure != secure {
				t.Errorf("Secure = %v, want %v; a Secure cookie is discarded over plain HTTP "+
					"and every form post in development would then 403", c.Secure, secure)
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, want Lax", c.SameSite)
			}
			if c.Path != "/" {
				t.Errorf("Path = %q, want \"/\"; a form on another path would get a second, "+
					"different token", c.Path)
			}
		})
	}
}

// Re-rendering a page must not mint a second token, or the two forms on it
// would disagree with the cookie. Nothing may be written to the response when
// the cookie is already there.
func TestCSRFTokenSetsNoCookieWhenOneExists(t *testing.T) {
	withSecureCookies(t, true)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: "already-issued"})

	rec := httptest.NewRecorder()
	if got := CSRFToken(rec, r); got != "already-issued" {
		t.Errorf("CSRFToken returned %q rather than the value already in the cookie", got)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("a second CSRF cookie was set on a request that already had one; the two " +
			"forms on the page would end up carrying different tokens")
	}
}

// An empty cookie value is treated as "no token", so a browser holding a
// cleared cookie gets a fresh one rather than being wedged into permanent 403s.
func TestCSRFTokenReplacesAnEmptyCookie(t *testing.T) {
	withSecureCookies(t, true)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: csrfCookie, Value: ""})

	rec := httptest.NewRecorder()
	if got := CSRFToken(rec, r); got == "" {
		t.Fatal("no token was issued to a request holding an empty CSRF cookie, so the " +
			"visitor could never submit a form again")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Error("no replacement cookie was set")
	}
}

// A token issued by CSRFToken must actually satisfy CSRF. This closes the loop
// between the two halves of the pattern, which are otherwise only tested apart.
func TestAnIssuedTokenSatisfiesTheGuard(t *testing.T) {
	withSecureCookies(t, false)

	issuing := httptest.NewRecorder()
	token := CSRFToken(issuing, httptest.NewRequest(http.MethodGet, "/", nil))

	r := httptest.NewRequest(http.MethodPost, "/arrangementer",
		strings.NewReader(encodedField(token)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for _, c := range issuing.Result().Cookies() {
		r.AddCookie(c)
	}

	if code, ran := reached(t, r); !ran {
		t.Errorf("a freshly issued token was refused by the guard with %d; no form on the "+
			"site could be submitted", code)
	}
}

// ── Rendering ─────────────────────────────────────────────────────────────

// CSRFInput returns template.HTML, which the template engine trusts verbatim.
// Anything unescaped in it is stored XSS on every page with a form, so the
// escaping here is the only thing between a token value and script execution.
func TestCSRFInputEscapesItsToken(t *testing.T) {
	tests := map[string]struct {
		token   string
		absent  []string
		present []string
	}{
		"an ordinary token": {
			token:   "abc-_123",
			present: []string{`name="` + CSRFField + `"`, `value="abc-_123"`, `type="hidden"`},
		},
		"a token that tries to close the attribute": {
			token:  `"><script>alert(1)</script>`,
			absent: []string{`"><script>`, `</script>`},
		},
		"a token containing a single quote and an angle bracket": {
			token:  `'<img src=x onerror=alert(1)>`,
			absent: []string{`<img`, `onerror=alert(1)>`},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := string(CSRFInput(tt.token))
			for _, want := range tt.present {
				if !strings.Contains(got, want) {
					t.Errorf("rendered field %q is missing %q", got, want)
				}
			}
			for _, bad := range tt.absent {
				if strings.Contains(got, bad) {
					t.Errorf("rendered field %q contains %q unescaped, which is script "+
						"execution on every page carrying a form", got, bad)
				}
			}
		})
	}

	// The declared type matters as much as the content: a plain string would be
	// escaped again by the template engine and render as visible markup.
	var _ template.HTML = CSRFInput("x")
}

// ── Constant-time comparison ──────────────────────────────────────────────

// Token comparison must not stop at the first differing byte. It is used for
// the CSRF token and for the OIDC state parameter, and in both cases a timing
// oracle lets an attacker recover the value one byte at a time.
func TestConstantTimeEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", "abc123", "abc123", true},
		{"different values", "abc123", "xyz789", false},
		{"differing only in the last byte", "abc123", "abc124", false},
		{"differing only in the first byte", "abc123", "bbc123", false},
		{"a prefix of the other", "abc", "abc123", false},
		{"differing case", "ABC", "abc", false},
		{"one empty", "", "abc", false},
		{"the other empty", "abc", "", false},
		// Surprising, and the reason csrf.go rejects an empty cookie before it
		// ever reaches this function. Pinned so the assumption stays visible.
		{"both empty", "", "", true},
		{"unicode, identical", "æøå", "æøå", true},
		{"unicode, different", "æøå", "æøa", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := constantTimeEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("constantTimeEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
			if got := ConstantTimeEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("the exported ConstantTimeEqual disagrees with the unexported one "+
					"for (%q, %q)", tt.a, tt.b)
			}
		})
	}
}

// Timing cannot be measured reliably in a unit test, so this reads the source
// instead. Replacing subtle.ConstantTimeCompare with == would leave every test
// above passing while quietly reintroducing the oracle.
func TestTokenComparisonIsConstantTimeInSource(t *testing.T) {
	src, err := os.ReadFile("middleware.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "subtle.ConstantTimeCompare(") {
		t.Error("constantTimeEqual no longer uses subtle.ConstantTimeCompare; CSRF tokens " +
			"and the OIDC state parameter would become recoverable a byte at a time")
	}
}
