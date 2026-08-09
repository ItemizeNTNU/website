package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ItemizeNTNU/website/internal/auth"
	"github.com/ItemizeNTNU/website/internal/events"
)

func checkInMux(t *testing.T, repo *stubRepo) *http.ServeMux {
	t.Helper()
	return newAPI(t, apiConfig{repo: repo})
}

func seededRepo(t *testing.T) *stubRepo {
	t.Helper()
	return &stubRepo{byCode: map[string]events.Event{testCode: pizzakveld(t)}}
}

// Reading the register hands out the check-in code itself — the credential that
// registers attendance — together with every attendee's name and FusionAuth
// identifier. The previous version served that to anyone who knew a code. The
// board gate is the fix, and these are the two ways past it that must not work.
//
// The status is 401 for both, including for a signed-in member without the
// role, because clients key off the message and the previous API answered that
// way. It reads oddly next to 403 but changing it is a silent break.
func TestGetCheckInRequiresTheBoard(t *testing.T) {
	for _, tc := range []struct {
		name string
		user *auth.User
		want string
	}{
		{"an anonymous caller", nil, "You are not logged in"},
		{"a member without the board role", member, "Permission denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededRepo(t)
			rec := getAs(t, checkInMux(t, repo), "/api/checkin/"+testCode, tc.user)

			wantStatus(t, rec, http.StatusUnauthorized)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != tc.want {
				t.Errorf("the refusal reads %q, want %q", got, tc.want)
			}
			if strings.Contains(rec.Body.String(), member.ID) {
				t.Error("a refused request still leaked an attendee identifier")
			}
			if repo.gotCode != "" {
				t.Error("storage was queried before the caller was refused; the gate " +
					"must come first, not merely hide the answer")
			}
		})
	}
}

// What the board actually gets: the event plus the register, in the shape the
// check-in page reads.
func TestGetCheckInReturnsTheRegister(t *testing.T) {
	repo := seededRepo(t)
	rec := getAs(t, checkInMux(t, repo), "/api/checkin/"+testCode, styret)

	wantStatus(t, rec, http.StatusOK)
	wantJSON(t, rec)
	if got := strings.TrimSuffix(rec.Body.String(), "\n"); got != pizzakveldStyretJSON {
		t.Errorf("the register does not match the shape the check-in page reads.\n got: %s\nwant: %s",
			got, pizzakveldStyretJSON)
	}
	if repo.gotCode != testCode {
		t.Errorf("storage was asked for code %q, want %q", repo.gotCode, testCode)
	}
}

// An event whose register is still empty must come back with the code and
// without an attendances key — the page shows "nobody yet" from the absence,
// and an event with no code at all still has to render rather than 500.
func TestGetCheckInWithAnEmptyRegister(t *testing.T) {
	repo := &stubRepo{byCode: map[string]events.Event{
		testCode: {Name: "Nytt arrangement", CheckIn: events.CheckIn{Code: testCode}},
	}}

	rec := getAs(t, checkInMux(t, repo), "/api/checkin/"+testCode, styret)

	wantStatus(t, rec, http.StatusOK)
	body := strings.TrimSpace(rec.Body.String())
	if !strings.Contains(body, `"check_in":{"code":"`+testCode+`"}`) {
		t.Errorf("an unused register did not encode as a bare code: %s", body)
	}
	if strings.Contains(body, "attendances") {
		t.Errorf("an empty register emitted an attendances key: %s", body)
	}
}

// A code that matches nothing is a 404 with the JSON envelope, not the site's
// HTML error page — the check-in screen fetches this and parses the answer.
func TestGetCheckInUnknownCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
	}{
		{"a code that matches no event", "/api/checkin/ingen-slik-kode"},
		{"a Norwegian code", "/api/checkin/" + url.PathEscape("blåbærsyltetøy")},
		{"a single character", "/api/checkin/x"},
		{"a code that looks like a path traversal", "/api/checkin/" + url.PathEscape("../../etc/passwd")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := getAs(t, checkInMux(t, seededRepo(t)), tc.path, styret)

			wantStatus(t, rec, http.StatusNotFound)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != "Event not found" {
				t.Errorf("the message is %q, want %q", got, "Event not found")
			}
		})
	}
}

// A database that will not answer is reported as "not found" here. That is the
// current contract and it is pinned deliberately: the alternative — a 500 that
// distinguishes the two — would tell a caller probing for valid codes which of
// their guesses hit a real event during an outage. The cost is that a genuine
// outage looks like a mistyped code, which the report notes.
func TestGetCheckInReportsStorageFailureAsNotFound(t *testing.T) {
	repo := seededRepo(t)
	repo.byCodeErr = errRepo

	rec := getAs(t, checkInMux(t, repo), "/api/checkin/"+testCode, styret)

	wantStatus(t, rec, http.StatusNotFound)
	if got := messageOf(t, rec); got != "Event not found" {
		t.Errorf("the message is %q, want %q", got, "Event not found")
	}
	if strings.Contains(rec.Body.String(), errRepo.Error()) {
		t.Error("the internal failure was echoed to the caller")
	}
}

// Registering attendance only needs the caller to be somebody — but it does
// need that. The handler reads user.ID without a nil check, so this middleware
// is not merely a policy: without it an anonymous scan is a nil dereference and
// a panic in the request goroutine.
func TestPostCheckInRequiresALogin(t *testing.T) {
	repo := seededRepo(t)
	rec := do(t, checkInMux(t, repo), http.MethodPost, "/api/checkin/"+testCode, nil, nil)

	wantStatus(t, rec, http.StatusUnauthorized)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != "You are not logged in" {
		t.Errorf("the refusal reads %q, want %q", got, "You are not logged in")
	}
	if repo.adds != 0 {
		t.Error("an anonymous request reached storage")
	}
}

// The happy path, and the one thing about it that matters afterwards: the
// register is read by people, so it wants the legal name rather than the
// identifier.
func TestPostCheckInRegistersAttendance(t *testing.T) {
	repo := seededRepo(t)
	rec := do(t, checkInMux(t, repo), http.MethodPost, "/api/checkin/"+testCode, nil, member)

	wantStatus(t, rec, http.StatusOK)
	wantJSON(t, rec)
	if got := messageOf(t, rec); got != "Success" {
		t.Errorf("the confirmation reads %q, want %q", got, "Success")
	}
	if repo.adds != 1 {
		t.Fatalf("storage recorded %d attendances, want 1", repo.adds)
	}
	if repo.gotCode != testCode {
		t.Errorf("attendance was recorded against code %q, want %q", repo.gotCode, testCode)
	}
	if repo.gotAttendance.Name != member.FullName {
		t.Errorf("the register would show %q, want the member's full name %q",
			repo.gotAttendance.Name, member.FullName)
	}
	if repo.gotAttendance.UserID != member.ID {
		t.Errorf("the attendance is attributed to %q, want %q", repo.gotAttendance.UserID, member.ID)
	}
}

// The name is what somebody reads off the list afterwards, so an incomplete
// token must not produce a blank row — which is what writing fullName
// unconditionally used to do, unnoticed until the list was read.
func TestNameFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		user *auth.User
		want string
	}{
		{"nil user", nil, ""},
		{"full name wins", &auth.User{FullName: "Kari Nordmann", Name: "Kari"}, "Kari Nordmann"},
		{"display name when the legal name is missing", &auth.User{Name: "Kari"}, "Kari"},
		{"email as a last resort", &auth.User{Email: "kari@example.no"}, "kari@example.no"},
		{"nothing at all", &auth.User{}, ""},
		{"Norwegian characters are untouched", &auth.User{FullName: "Åse Øverland-Æsberg"}, "Åse Øverland-Æsberg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nameFor(tc.user); got != tc.want {
				t.Errorf("nameFor gave %q, want %q", got, tc.want)
			}
		})
	}
}

// The same fallback, reached the way it actually happens: a member whose token
// carries no legal-name claim scans the code at the door.
func TestPostCheckInFallsBackToTheDisplayName(t *testing.T) {
	repo := seededRepo(t)
	user := &auth.User{ID: "33333333-4444-4555-8666-777777777777", Name: "Øyvind"}

	do(t, checkInMux(t, repo), http.MethodPost, "/api/checkin/"+testCode, nil, user)

	if repo.gotAttendance.Name != "Øyvind" {
		t.Errorf("the register would show %q, want the display name; a blank row is "+
			"a person nobody can identify afterwards", repo.gotAttendance.Name)
	}
}

// Every branch the write path can take, and the status each one has to keep. A
// second scan is a conflict rather than an error, because scanning twice is
// what people do and the response is shown to them at the door.
func TestPostCheckInErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name       string
		addErr     error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "a second scan of the same code",
			addErr:     events.ErrAlreadyCheckedIn,
			wantStatus: http.StatusConflict,
			wantMsg:    "You have already registered your attendance for this event",
		},
		{
			name:       "a code that matches no event",
			addErr:     events.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantMsg:    `Event not found with check_in code "` + testCode + `"`,
		},
		{
			name:       "storage is unavailable",
			addErr:     errRepo,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    "Something broke :/",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededRepo(t)
			repo.addErr = tc.addErr

			rec := do(t, checkInMux(t, repo), http.MethodPost, "/api/checkin/"+testCode, nil, member)

			wantStatus(t, rec, tc.wantStatus)
			wantJSON(t, rec)
			if got := messageOf(t, rec); got != tc.wantMsg {
				t.Errorf("the message reads %q, want %q", got, tc.wantMsg)
			}
		})
	}
}

// The sentinel errors are matched with errors.Is, so a repository that wraps
// them for context must still land on the right status. Unwrapping by equality
// would turn a duplicate scan into a 500 at the door.
func TestPostCheckInMatchesWrappedSentinels(t *testing.T) {
	for _, tc := range []struct {
		name       string
		addErr     error
		wantStatus int
	}{
		{
			"a wrapped duplicate",
			fmt.Errorf("recording attendance: %w", events.ErrAlreadyCheckedIn),
			http.StatusConflict,
		},
		{
			"a wrapped miss",
			fmt.Errorf("looking up the code: %w", events.ErrNotFound),
			http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededRepo(t)
			repo.addErr = tc.addErr

			rec := do(t, checkInMux(t, repo), http.MethodPost, "/api/checkin/"+testCode, nil, member)

			wantStatus(t, rec, tc.wantStatus)
		})
	}
}

// The rejected code is echoed back in the message, so a code carrying a quote
// or a tag must not break out of the JSON string. This is the one place in the
// package where caller-controlled text reaches a response body, and the
// envelope is what keeps it inert.
func TestPostCheckInEscapesTheEchoedCode(t *testing.T) {
	for _, code := range []string{
		`"; DROP TABLE`,
		`<script>alert(1)</script>`,
		`æøå`,
		`back\slash`,
	} {
		t.Run(code, func(t *testing.T) {
			repo := seededRepo(t)
			repo.addErr = events.ErrNotFound

			rec := do(t, checkInMux(t, repo), http.MethodPost,
				"/api/checkin/"+url.PathEscape(code), nil, member)

			wantStatus(t, rec, http.StatusNotFound)
			// Decoding is the assertion: a body that has broken out of the
			// string would not parse, and the raw tag must not appear in it.
			got := messageOf(t, rec)
			if want := `Event not found with check_in code "` + code + `"`; got != want {
				t.Errorf("the message decoded to %q, want %q", got, want)
			}
			if strings.Contains(rec.Body.String(), "<script>") {
				t.Error("a raw script tag reached the response body")
			}
		})
	}
}
