package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ItemizeNTNU/website/internal/auth"
)

// userPath addresses the member fixture, whose identifier is UUID-shaped so
// that these tests exercise the lookup rather than fusionauth.ValidID.
var userPath = "/api/user/" + member.ID

// jsonUser replies with a FusionAuth user envelope.
func jsonUser(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

// The directory carries an email address and a legal name, and identifiers leak
// into places members can see. Serving it to anybody — which the previous site
// did — made the whole directory readable to anyone who collected one
// identifier.
func TestGetUserRequiresALogin(t *testing.T) {
	fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
	mux := newAPI(t, apiConfig{fusion: fusion})

	rec := getAs(t, mux, userPath, nil)

	wantStatus(t, rec, http.StatusUnauthorized)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != "You are not logged in" {
		t.Errorf("the refusal reads %q, want %q", got, "You are not logged in")
	}
	if spy.snapshot().calls != 0 {
		t.Error("an anonymous request was forwarded to FusionAuth; the gate must " +
			"come before the upstream call, not after it")
	}
}

// A deployment without an API key must say so plainly rather than failing in a
// way that looks like the member does not exist. A contributor running the site
// locally hits this constantly, and 503 is what tells them why.
func TestGetUserWithoutAConfiguredDirectory(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  apiConfig
	}{
		{"no API token", apiConfig{}},
		{"no client at all", apiConfig{nilFusion: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := getAs(t, newAPI(t, tc.cfg), userPath, member)

			wantStatus(t, rec, http.StatusServiceUnavailable)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "User directory is unavailable" {
				t.Errorf("the message is %q, want %q", got, "User directory is unavailable")
			}
		})
	}
}

// An identifier that is not a UUID is refused here, before it can be
// concatenated into an upstream URL. Escaping already prevents the traversal;
// the shape check means a malformed identifier never becomes a request at all,
// so the assertion that matters is the call count rather than the status.
func TestGetUserRefusesMalformedIdentifiersWithoutCallingUpstream(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
	}{
		{"not a UUID", "user-1"},
		{"a bare word", "kari"},
		{"a query-parameter injection", "abc%3Fkey%3Dsecret"},
		{"a traversal", "..%2F..%2Fapi%2Fkey"},
		{"a UUID with a trailing character", "22222222-3333-4444-8555-666666666666x"},
		{"a UUID missing a group", "22222222-3333-4444-666666666666"},
		{"Norwegian text", "%C3%A6%C3%B8%C3%A5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := getAs(t, mux, "/api/user/"+tc.id, member)

			wantStatus(t, rec, http.StatusNotFound)
			// Deliberately the same answer as a real miss: telling the two apart
			// would confirm the identifier format to anyone probing.
			if got := messageOf(t, rec); got != "User not found" {
				t.Errorf("the message is %q, want %q", got, "User not found")
			}
			if calls := spy.snapshot().calls; calls != 0 {
				t.Errorf("the malformed identifier produced %d upstream requests; it "+
					"must be refused before it reaches a URL carrying the admin key", calls)
			}
		})
	}
}

// The mapping from a FusionAuth record to what the site publishes. The
// fallbacks are the point: the previous version assigned displayName =
// fullName || fullName, which discarded the display name entirely and showed
// everyone's legal name instead.
func TestGetUserMapsTheRecord(t *testing.T) {
	for _, tc := range []struct {
		name     string
		upstream string
		want     userDTO
	}{
		{
			name: "a complete record",
			upstream: `{"user":{"id":"u1","email":"kari@example.no","fullName":"Kari Nordmann",
				"imageUrl":"https://cdn.example/kari.png",
				"data":{"displayName":"Kari","type":"student",
				"discord":{"username":"kari#1234"}}}}`,
			want: userDTO{
				ID: "u1", Email: "kari@example.no", FullName: "Kari Nordmann",
				Name: "Kari", ImageURL: "https://cdn.example/kari.png",
				Type: "student", Discord: "kari#1234",
			},
		},
		{
			name:     "no avatar falls back to the logo",
			upstream: `{"user":{"id":"u2","fullName":"Ola Nordmann","data":{"displayName":"Ola"}}}`,
			want: userDTO{
				ID: "u2", FullName: "Ola Nordmann", Name: "Ola",
				ImageURL: defaultProfileImage,
			},
		},
		{
			name:     "no display name falls back to the legal name",
			upstream: `{"user":{"id":"u3","fullName":"Åse Øverland","data":{"type":"alumni"}}}`,
			want: userDTO{
				ID: "u3", FullName: "Åse Øverland", Name: "Åse Øverland",
				ImageURL: defaultProfileImage, Type: "alumni",
			},
		},
		{
			name:     "an empty display name is not a display name",
			upstream: `{"user":{"id":"u4","fullName":"Per Hansen","data":{"displayName":""}}}`,
			want: userDTO{
				ID: "u4", FullName: "Per Hansen", Name: "Per Hansen",
				ImageURL: defaultProfileImage,
			},
		},
		{
			name:     "no data block at all",
			upstream: `{"user":{"id":"u5","fullName":"Kim Berg"}}`,
			want: userDTO{
				ID: "u5", FullName: "Kim Berg", Name: "Kim Berg",
				ImageURL: defaultProfileImage,
			},
		},
		{
			name:     "a record with nothing in it",
			upstream: `{"user":{}}`,
			want:     userDTO{ImageURL: defaultProfileImage},
		},
		{
			// Data is caller-influenced through the registration form and the
			// profile page, so a wrongly-typed value must be dropped rather than
			// crash the handler.
			name: "wrongly typed data values are ignored",
			upstream: `{"user":{"id":"u6","fullName":"Nils Dahl",
				"data":{"displayName":42,"type":["student"],"discord":"kari#1234"}}}`,
			want: userDTO{
				ID: "u6", FullName: "Nils Dahl", Name: "Nils Dahl",
				ImageURL: defaultProfileImage,
			},
		},
		{
			name: "a Discord block without a username",
			upstream: `{"user":{"id":"u7","fullName":"Siri Vik",
				"data":{"discord":{"id":"123"}}}}`,
			want: userDTO{
				ID: "u7", FullName: "Siri Vik", Name: "Siri Vik",
				ImageURL: defaultProfileImage,
			},
		},
		{
			name: "a Discord username that is not a string",
			upstream: `{"user":{"id":"u8","fullName":"Tor Lie",
				"data":{"discord":{"username":123}}}}`,
			want: userDTO{
				ID: "u8", FullName: "Tor Lie", Name: "Tor Lie",
				ImageURL: defaultProfileImage,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(tc.upstream))
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := getAs(t, mux, userPath, member)

			wantStatus(t, rec, http.StatusOK)
			wantJSON(t, rec)

			var got userDTO
			decodeBody(t, rec, &got)
			if got != tc.want {
				t.Errorf("the published profile is\n %+v\nwant\n %+v", got, tc.want)
			}

			// The identifier goes in the path, and the API key in the header
			// without a Bearer prefix — adding one produces a 401 from
			// FusionAuth, which looks like a permissions problem instead.
			snap := spy.snapshot()
			if snap.path != "/api/user/"+member.ID {
				t.Errorf("upstream was asked for %q, want %q", snap.path, "/api/user/"+member.ID)
			}
			if snap.auth != "test-api-key" {
				t.Errorf("the API key was sent as %q, want the bare key", snap.auth)
			}
		})
	}
}

// The optional fields must stay out of the JSON when empty. A profile card that
// renders a Discord badge whenever the key is present would show an empty badge
// for every member who has not linked an account.
func TestGetUserOmitsEmptyOptionalFields(t *testing.T) {
	fusion, _ := fakeFusion(t, jsonUser(`{"user":{"id":"u1","fullName":"Ola"}}`))
	mux := newAPI(t, apiConfig{fusion: fusion})

	rec := getAs(t, mux, userPath, member)
	body := strings.TrimSpace(rec.Body.String())

	for _, key := range []string{`"type"`, `"discord"`} {
		if strings.Contains(body, key) {
			t.Errorf("an unset field emitted %s: %s", key, body)
		}
	}
	// The required ones stay, even when empty: a client reading `email` must not
	// have to distinguish "absent" from "not given".
	for _, key := range []string{`"id"`, `"email"`, `"fullName"`, `"name"`, `"imageUrl"`} {
		if !strings.Contains(body, key) {
			t.Errorf("the required field %s is missing: %s", key, body)
		}
	}
}

// What the caller is told when the lookup does not produce a user. The two
// upstream failures are told apart deliberately: a missing member is the
// caller's problem, an unreachable directory is ours, and a monitor watching
// for 5xx should only see the second.
func TestGetUserUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		handler    http.HandlerFunc
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "no such member",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantStatus: http.StatusNotFound,
			wantMsg:    "User not found",
		},
		{
			name:       "the directory is broken",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantStatus: http.StatusBadGateway,
			wantMsg:    "Error fetching user",
		},
		{
			name:       "the API key was rejected",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			wantStatus: http.StatusBadGateway,
			wantMsg:    "Error fetching user",
		},
		{
			name:       "a reply that is not JSON",
			handler:    jsonUser(`<html>maintenance</html>`),
			wantStatus: http.StatusBadGateway,
			wantMsg:    "Error fetching user",
		},
		{
			name:       "a reply that is cut short",
			handler:    jsonUser(`{"user":{"id":`),
			wantStatus: http.StatusBadGateway,
			wantMsg:    "Error fetching user",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, _ := fakeFusion(t, tc.handler)
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := getAs(t, mux, userPath, member)

			wantStatus(t, rec, tc.wantStatus)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != tc.wantMsg {
				t.Errorf("the message is %q, want %q", got, tc.wantMsg)
			}
		})
	}
}

// A directory that cannot be reached at all — the machine is down, DNS is
// wrong — is the same class of problem as one answering 500, and must not be
// reported as a missing member.
func TestGetUserWhenTheDirectoryIsUnreachable(t *testing.T) {
	mux := newAPI(t, apiConfig{fusion: deadFusion(t)})

	rec := getAs(t, mux, userPath, member)

	wantStatus(t, rec, http.StatusBadGateway)
	if got := messageOf(t, rec); got != "Error fetching user" {
		t.Errorf("the message is %q, want %q", got, "Error fetching user")
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Error("the transport error was echoed to the caller, which discloses the " +
			"directory's address")
	}
}

// Registration exists to create an account, so a caller who already has one is
// almost certainly a confused client rather than a member — and letting it
// through would send a password-setting email to whatever address it named.
func TestRegisterUserRejectsSignedInCallers(t *testing.T) {
	fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
	mux := newAPI(t, apiConfig{fusion: fusion})

	rec := putJSON(t, mux, "/api/user", validRegistration(t, "student"), member)

	wantStatus(t, rec, http.StatusBadRequest)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != "You are already registered" {
		t.Errorf("the message is %q, want %q", got, "You are already registered")
	}
	if spy.snapshot().calls != 0 {
		t.Error("a signed-in caller still caused an upstream account creation")
	}
}

// Without an API key nothing can be created, and saying so is better than the
// generic failure a contributor would otherwise spend an evening on.
func TestRegisterUserWithoutAConfiguredDirectory(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  apiConfig
	}{
		{"no API token", apiConfig{}},
		{"no client at all", apiConfig{nilFusion: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := putJSON(t, newAPI(t, tc.cfg), "/api/user", validRegistration(t, "student"), nil)

			wantStatus(t, rec, http.StatusServiceUnavailable)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "Registration is unavailable" {
				t.Errorf("the message is %q, want %q", got, "Registration is unavailable")
			}
		})
	}
}

// A body the decoder cannot read is answered before anything is created. The
// wrong content type is in the table because the handler never looks at the
// header — a form-encoded submission is refused by the decoder rather than by a
// content negotiation the endpoint does not do.
func TestRegisterUserRejectsUnreadableBodies(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"an empty body", ""},
		{"whitespace", "   "},
		{"truncated JSON", `{"email":`},
		{"not JSON at all", "hello"},
		{"a bare string", `"hello"`},
		{"an array where an object belongs", `[]`},
		{"a number", `42`},
		{"a form submission", "fullName=Kari&email=kari%40example.no"},
		{"a wrongly typed field", `{"fullName":123}`},
		{"a wrongly typed nested block", `{"data":"student"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := putJSON(t, mux, "/api/user", tc.body, nil)

			wantStatus(t, rec, http.StatusBadRequest)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "Invalid request body" {
				t.Errorf("the message is %q, want %q", got, "Invalid request body")
			}
			if spy.snapshot().calls != 0 {
				t.Error("an unreadable body still reached FusionAuth")
			}
		})
	}
}

// The body is capped at a megabyte. Without the cap an unauthenticated caller
// could hold the process's memory open by streaming a body that never ends,
// which needs no credentials at all — the endpoint is public by necessity.
func TestRegisterUserRejectsAnOversizedBody(t *testing.T) {
	fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
	mux := newAPI(t, apiConfig{fusion: fusion})

	// Comfortably past 1<<20, in a field the decoder has to read through.
	huge := `{"fullName":"` + strings.Repeat("a", 1<<20+64) + `"}`

	rec := putJSON(t, mux, "/api/user", huge, nil)

	wantStatus(t, rec, http.StatusBadRequest)
	if got := messageOf(t, rec); got != "Invalid request body" {
		t.Errorf("the message is %q, want %q", got, "Invalid request body")
	}
	if spy.snapshot().calls != 0 {
		t.Error("an oversized body still reached FusionAuth")
	}
}

// A body that decodes but says nothing must fail validation rather than create
// an empty member. The message is the first field in sorted order, which is why
// it is the same one every time — a message that varied between identical
// requests would be untestable and bewildering to support.
func TestRegisterUserRejectsAnEmptyRegistration(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"an empty object", `{}`},
		{"a JSON null", `null`},
		{"only unknown fields", `{"nickname":"kari","admin":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := putJSON(t, mux, "/api/user", tc.body, nil)

			wantStatus(t, rec, http.StatusBadRequest)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "Visningsnavn må fylles ut." {
				t.Errorf("the message is %q, want the first validation failure in "+
					"sorted field order", got)
			}
			if spy.snapshot().calls != 0 {
				t.Error("an invalid registration still reached FusionAuth")
			}
		})
	}
}

// The endpoint runs the same validation as the form, so the rules that matter
// to a member are enforced whichever entry point they arrive through — the
// whole reason the JSON body is flattened into form values first.
func TestRegisterUserValidation(t *testing.T) {
	next := time.Now().Year() + 1

	for _, tc := range []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "a student address is refused, with the reason",
			body:    registrationBody("Kari Nordmann", "kari@stud.ntnu.no", "Kari", "student", next),
			wantMsg: "Vennligst ikke bruk din stud e-post adresse, da du mister tilgang til denne etter fullført utdannelse.",
		},
		{
			name:    "an address that is not one",
			body:    registrationBody("Kari Nordmann", "kari-at-example", "Kari", "student", next),
			wantMsg: "E-postadressen ser ikke gyldig ut.",
		},
		{
			name:    "a missing address",
			body:    registrationBody("Kari Nordmann", "", "Kari", "student", next),
			wantMsg: "E-postadresse må fylles ut.",
		},
		{
			name:    "a display name of two characters",
			body:    registrationBody("Kari Nordmann", "kari@example.no", "Ka", "student", next),
			wantMsg: "Visningsnavn må være minst 3 tegn.",
		},
		{
			name:    "a display name past the limit",
			body:    registrationBody("Kari Nordmann", "kari@example.no", strings.Repeat("æ", 33), "student", next),
			wantMsg: "Visningsnavn kan ikke være lengre enn 32 tegn.",
		},
		{
			name:    "a membership type that is not one of the three",
			body:    registrationBody("Kari Nordmann", "kari@example.no", "Kari", "styremedlem", next),
			wantMsg: "Medlemstype er ikke et gyldig valg.",
		},
		{
			name:    "an expected finish year in the past",
			body:    registrationBody("Kari Nordmann", "kari@example.no", "Kari", "student", 2001),
			wantMsg: fmt.Sprintf("Forventet ferdig år kan ikke være mindre enn %d.", time.Now().Year()),
		},
		{
			name:    "an expected finish year beyond the horizon",
			body:    registrationBody("Kari Nordmann", "kari@example.no", "Kari", "student", time.Now().Year()+16),
			wantMsg: fmt.Sprintf("Forventet ferdig år kan ikke være større enn %d.", time.Now().Year()+15),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"x"}}`))
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := putJSON(t, mux, "/api/user", tc.body, nil)

			wantStatus(t, rec, http.StatusBadRequest)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != tc.wantMsg {
				t.Errorf("the member would be told %q, want %q", got, tc.wantMsg)
			}
			if spy.snapshot().calls != 0 {
				t.Error("an invalid registration still reached FusionAuth")
			}
		})
	}
}

// The happy path for each membership type, checked at the upstream request
// rather than the response: what is created is a permanent record, and the
// fields belonging to the other two types must be absent rather than empty —
// an alumnus arriving with a blank study year would carry it for good.
func TestRegisterUserCreatesTheAccount(t *testing.T) {
	next := time.Now().Year() + 1

	for _, tc := range []struct {
		name    string
		body    string
		want    map[string]any
		absent  []string
		present []string
	}{
		{
			name: "a student",
			body: registrationBody("Kari Nordmann", "kari@example.no", "Kari", "student", next),
			want: map[string]any{
				"displayName": "Kari",
				"type":        "student",
				"study": map[string]any{
					"program":            "Datateknologi",
					"year":               float64(2),
					"expectedFinishYear": fmt.Sprintf("%04d-06-01T00:00:00Z", next),
				},
			},
			absent: []string{"alumni", "employee"},
		},
		{
			name: "an alumnus",
			body: fmt.Sprintf(`{"fullName":"Ola Nordmann","email":"ola@example.no",
				"data":{"displayName":"Ola","type":"alumni",
				"study":{"program":"Datateknologi"},
				"alumni":{"joinYear":%d}}}`, time.Now().Year()),
			want: map[string]any{
				"displayName": "Ola",
				"type":        "alumni",
				"study":       map[string]any{"program": "Datateknologi"},
				"alumni":      map[string]any{"joinYear": float64(time.Now().Year())},
			},
			absent: []string{"employee"},
		},
		{
			name: "an employee",
			body: `{"fullName":"Åse Øverland","email":"aase@example.no",
				"data":{"displayName":"Åse","type":"employee",
				"employee":{"title":"Førsteamanuensis"}}}`,
			want: map[string]any{
				"displayName": "Åse",
				"type":        "employee",
				"employee":    map[string]any{"title": "Førsteamanuensis"},
			},
			absent: []string{"study", "alumni"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"new-user"}}`))
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := putJSON(t, mux, "/api/user", tc.body, nil)

			wantStatus(t, rec, http.StatusOK)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "Success" {
				t.Errorf("the confirmation reads %q, want %q", got, "Success")
			}

			snap := spy.snapshot()
			if snap.calls != 1 {
				t.Fatalf("FusionAuth saw %d requests, want exactly 1", snap.calls)
			}
			if snap.method != http.MethodPost || snap.path != "/api/user" {
				t.Errorf("upstream was called as %s %s, want POST /api/user", snap.method, snap.path)
			}

			var sent struct {
				SendSetPasswordEmail bool `json:"sendSetPasswordEmail"`
				User                 struct {
					Email    string         `json:"email"`
					FullName string         `json:"fullName"`
					Data     map[string]any `json:"data"`
				} `json:"user"`
			}
			if err := json.Unmarshal([]byte(snap.body), &sent); err != nil {
				t.Fatalf("the upstream request is not JSON: %v; body was %s", err, snap.body)
			}

			// Without this the member is created but never receives the link
			// that lets them set a password, and the account is unusable.
			if !sent.SendSetPasswordEmail {
				t.Error("the account was created without asking FusionAuth to send " +
					"the password-setting email, so the member can never sign in")
			}
			for key, want := range tc.want {
				if got := sent.User.Data[key]; !sameJSON(got, want) {
					t.Errorf("data[%q] was sent as %#v, want %#v", key, got, want)
				}
			}
			for _, key := range tc.absent {
				if _, ok := sent.User.Data[key]; ok {
					t.Errorf("data carries %q, which belongs to a different membership "+
						"type and would be stored permanently", key)
				}
			}
		})
	}
}

// The study year arrives as a JSON number from a modern client and as a string
// from whatever is still posting the previous API's shape. Both have to reach
// validation as the same digits, or one of the two callers is rejected for a
// field they filled in correctly.
func TestRegisterUserAcceptsNumbersAsStringsOrNumbers(t *testing.T) {
	next := time.Now().Year() + 1

	for _, tc := range []struct {
		name string
		year string
	}{
		{"a JSON number", `2`},
		{"a JSON string", `"2"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, spy := fakeFusion(t, jsonUser(`{"user":{"id":"new-user"}}`))
			mux := newAPI(t, apiConfig{fusion: fusion})

			body := fmt.Sprintf(`{"fullName":"Kari Nordmann","email":"kari@example.no",
				"data":{"displayName":"Kari","type":"student",
				"study":{"program":"Datateknologi","year":%s,"expectedFinishYear":"%d-06-01T00:00:00Z"}}}`,
				tc.year, next)

			rec := putJSON(t, mux, "/api/user", body, nil)

			wantStatus(t, rec, http.StatusOK)
			if !strings.Contains(spy.snapshot().body, `"year":2`) {
				t.Errorf("the study year did not reach FusionAuth as 2: %s", spy.snapshot().body)
			}
		})
	}
}

// FusionAuth's own rejections are passed through, because the one that actually
// happens is "email already in use" and the member needs to read it. Anything
// else it says is at least closer to the truth than a generic failure.
func TestRegisterUserSurfacesUpstreamRejections(t *testing.T) {
	fusion, _ := fakeFusion(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"fieldErrors":{"user.email":[{"code":"[duplicate]",
			"message":"A User with email 'kari@example.no' already exists."}]}}`)
	})
	mux := newAPI(t, apiConfig{fusion: fusion})

	rec := putJSON(t, mux, "/api/user", validRegistration(t, "student"), nil)

	wantStatus(t, rec, http.StatusBadRequest)
	wantJSON(t, rec)
	const want = "A User with email 'kari@example.no' already exists."
	if got := messageOf(t, rec); got != want {
		t.Errorf("the member would be told %q, want %q", got, want)
	}
}

// wantUpstreamDown is what a member sees when the failure is ours rather than
// theirs: no service address, no status code, and an instruction that is worth
// following — the form they filled in was fine.
const wantUpstreamDown = "Innloggingstjenesten svarer ikke akkurat nå. Prøv igjen om litt."

// Who is at fault decides both the status and the wording, and FusionAuth's
// error parser makes that easy to get wrong: it wraps every non-2xx reply, a
// 5xx included, in the same *APIError the handler reads validation messages
// out of. Matching on the type alone told a member whose registration was
// perfectly valid that it was not, sent them back to correct a form with
// nothing wrong with it, and hid the outage from anything watching for 5xx.
func TestRegisterUserSeparatesItsOwnFailuresFromTheMembers(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "a rejection the member can act on",
			status:     http.StatusBadRequest,
			body:       `{"generalErrors":[{"code":"[duplicate]","message":"E-posten er allerede i bruk."}]}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "E-posten er allerede i bruk.",
		},
		{
			name:       "a conflict is still the member's to resolve",
			status:     http.StatusConflict,
			body:       `{"generalErrors":[{"code":"[duplicate]","message":"E-posten er allerede i bruk."}]}`,
			wantStatus: http.StatusBadRequest,
			wantMsg:    "E-posten er allerede i bruk.",
		},
		{
			name:       "the directory is broken",
			status:     http.StatusInternalServerError,
			body:       `{}`,
			wantStatus: http.StatusBadGateway,
			wantMsg:    wantUpstreamDown,
		},
		{
			name:       "the directory is restarting",
			status:     http.StatusServiceUnavailable,
			body:       `{}`,
			wantStatus: http.StatusBadGateway,
			wantMsg:    wantUpstreamDown,
		},
		{
			// The worst of the lot before the fix: FusionAuth's fallback
			// message is "Uventet svar fra innloggingstjenesten (HTTP 502)",
			// which was handed to the member as a 400 — a validation error
			// telling them about an HTTP status.
			name:       "something in front of the directory is broken",
			status:     http.StatusBadGateway,
			body:       `{}`,
			wantStatus: http.StatusBadGateway,
			wantMsg:    wantUpstreamDown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fusion, _ := fakeFusion(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			mux := newAPI(t, apiConfig{fusion: fusion})

			rec := putJSON(t, mux, "/api/user", validRegistration(t, "student"), nil)

			wantStatus(t, rec, tc.wantStatus)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != tc.wantMsg {
				t.Errorf("FusionAuth answered %d and the member was told %q, want %q",
					tc.status, got, tc.wantMsg)
			}
		})
	}
}

// A directory that answers with a server error is ours to fix, not the
// member's: their registration is untouched and retrying it is the right
// advice, so the failure must not arrive as a 400 that blames their input.
func TestRegisterUserWhenTheDirectoryErrors(t *testing.T) {
	fusion, _ := fakeFusion(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{}`)
	})
	mux := newAPI(t, apiConfig{fusion: fusion})

	rec := putJSON(t, mux, "/api/user", validRegistration(t, "student"), nil)

	wantStatus(t, rec, http.StatusBadGateway)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != wantUpstreamDown {
		t.Errorf("the member would be told %q, want %q", got, wantUpstreamDown)
	}
	// The fallback FusionAuth builds for a body it cannot read is a status code
	// in Norwegian prose. It is a fine thing to log and a useless thing to show
	// somebody trying to sign up.
	if strings.Contains(rec.Body.String(), "HTTP 500") {
		t.Errorf("the upstream status was shown to the member: %s", rec.Body.String())
	}
}

// A directory that cannot be reached at all is the same class of problem as one
// answering 500 — ours, and probably temporary — so it gets the same status and
// the same wording, without naming the service or the address.
func TestRegisterUserWhenTheDirectoryIsUnreachable(t *testing.T) {
	mux := newAPI(t, apiConfig{fusion: deadFusion(t)})

	rec := putJSON(t, mux, "/api/user", validRegistration(t, "student"), nil)

	wantStatus(t, rec, http.StatusBadGateway)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != wantUpstreamDown {
		t.Errorf("the message is %q, want %q", got, wantUpstreamDown)
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Error("the transport error was echoed to the caller")
	}
}

// Creating an account makes FusionAuth send mail to an address the caller
// chooses, so an unthrottled endpoint is a way to send mail from our domain to
// arbitrary people and to fill the directory with junk. Neither needs any
// access: the endpoint is public by necessity.
//
// This test burns its whole mux's allowance, which is why it builds its own.
func TestRegisterUserIsRateLimited(t *testing.T) {
	mux := newAPI(t, apiConfig{})
	body := validRegistration(t, "student")

	// The limiter counts every attempt, not only the successful ones — an
	// unconfigured directory still answers 503 rather than passing through.
	for i := range 5 {
		rec := putJSON(t, mux, "/api/user", body, nil)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was throttled; the allowance is five", i+1)
		}
	}

	rec := putJSON(t, mux, "/api/user", body, nil)

	wantStatus(t, rec, http.StatusTooManyRequests)
	if got := strings.TrimSpace(rec.Body.String()); got != "For mange forsøk. Vent litt og prøv igjen." {
		t.Errorf("the throttle message is %q, want the Norwegian one", got)
	}
	// Without this a client has no idea whether to retry in a second or an hour.
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After is %q, want %q", got, "60")
	}
}

// num renders the numbers that arrive as JSON, which decode as float64. A
// decimal tail would reach validation as "2.000000" and be rejected as not a
// whole number, so the member sees an error for a field they filled in.
func TestNum(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"a JSON number", float64(3), "3"},
		{"zero", float64(0), "0"},
		{"a fractional year is truncated", float64(3.7), "3"},
		{"a negative number", float64(-1), "-1"},
		{"an already-string number", "4", "4"},
		{"an empty string", "", ""},
		{"a non-numeric string is passed through for validation to reject", "fjerde", "fjerde"},
		{"absent", nil, ""},
		{"a boolean", true, ""},
		{"an object", map[string]any{"year": 2}, ""},
		{"an array", []any{2}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := num(tc.in); got != tc.want {
				t.Errorf("num(%#v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The previous API sent the expected finish year as a full ISO date, and
// clients built against it still do. Both forms have to reduce to the year, or
// a returning client is told its date is not a number.
func TestYearOf(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"", ""},
		{"2030", "2030"},
		{"2030-06-01T00:00:00Z", "2030"},
		{"2030-06-01", "2030"},
		// Too short to hold a year: passed through so validation rejects it
		// rather than this helper inventing one.
		{"203", "203"},
		{"20", "20"},
		{"tjuetretti", "tjue"},
	} {
		t.Run(strconv.Quote(tc.in), func(t *testing.T) {
			if got := yearOf(tc.in); got != tc.want {
				t.Errorf("yearOf(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// str reads a value out of FusionAuth's free-form data block, which is not a
// shape this code controls. Anything that is not a string has to come back
// empty rather than panic.
func TestStr(t *testing.T) {
	data := map[string]any{
		"displayName": "Kari",
		"type":        42,
		"empty":       "",
		"null":        nil,
	}
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"displayName", "Kari"},
		{"type", ""},
		{"empty", ""},
		{"null", ""},
		{"missing", ""},
	} {
		if got := str(data, tc.key); got != tc.want {
			t.Errorf("str(data, %q) = %q, want %q", tc.key, got, tc.want)
		}
	}
	if got := str(nil, "displayName"); got != "" {
		t.Errorf("str(nil, ...) = %q, want the empty string; a member with no data "+
			"block must not crash the handler", got)
	}
}

// registrationBody builds a student registration with the given values.
func registrationBody(fullName, email, displayName, memberType string, finishYear int) string {
	body := map[string]any{
		"fullName": fullName,
		"email":    email,
		"data": map[string]any{
			"displayName": displayName,
			"type":        memberType,
			"study": map[string]any{
				"program":            "Datateknologi",
				"year":               2,
				"expectedFinishYear": fmt.Sprintf("%04d-06-01T00:00:00Z", finishYear),
			},
		},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// validRegistration is a body that passes validation today and will keep doing
// so: the expected finish year is relative to the current year rather than a
// literal, so the tests do not start failing on New Year's Eve.
func validRegistration(t *testing.T, memberType string) string {
	t.Helper()
	return registrationBody("Kari Nordmann", "kari@example.no", "Kari", memberType, time.Now().Year()+1)
}

// sameJSON compares decoded JSON values, which are maps and float64s rather
// than the types they were written as.
func sameJSON(got, want any) bool {
	gotEncoded, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantEncoded, err := json.Marshal(want)
	if err != nil {
		return false
	}
	return string(gotEncoded) == string(wantEncoded)
}

// The registration endpoint takes no notice of who the caller claims to be
// beyond "nobody", so an anonymous request is the only one that proceeds. This
// pins that the check is on presence rather than on a role — a board member
// creating a second account for themselves is refused just the same.
func TestRegisterUserRejectsAnySignedInCaller(t *testing.T) {
	for _, u := range []*auth.User{member, styret} {
		rec := putJSON(t, newAPI(t, apiConfig{}), "/api/user", validRegistration(t, "student"), u)

		wantStatus(t, rec, http.StatusBadRequest)
		if got := messageOf(t, rec); got != "You are already registered" {
			t.Errorf("%s was told %q, want %q", u.Name, got, "You are already registered")
		}
	}
}
