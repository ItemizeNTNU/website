// Package auth handles login against FusionAuth, the session cookie, and
// role-based access control.
package auth

import (
	"context"
	"net/http"
)

// User is the authenticated member as the rest of the program sees them.
// It is populated from the OIDC id token and carried in the session cookie.
type User struct {
	ID       string   `json:"sub"`
	Name     string   `json:"n"`
	FullName string   `json:"fn"`
	Email    string   `json:"e"`
	ImageURL string   `json:"img"`
	Roles    []string `json:"r"`
}

// RoleStyret is the board role. It gates event administration and check-in.
const RoleStyret = "Styret"

// HasRole reports whether the user holds the named role. A nil user never
// does, so callers do not need a separate nil check.
func (u *User) HasRole(role string) bool {
	if u == nil {
		return false
	}
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsStyret is shorthand for the only role the site actually checks.
func (u *User) IsStyret() bool { return u.HasRole(RoleStyret) }

// DisplayName is the name to show in the interface, falling back through the
// available fields rather than rendering an empty string.
func (u *User) DisplayName() string {
	if u == nil {
		return ""
	}
	if u.Name != "" {
		return u.Name
	}
	if u.FullName != "" {
		return u.FullName
	}
	return u.Email
}

type ctxKey int

const userKey ctxKey = iota

// WithUser returns a copy of ctx carrying u.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// FromContext returns the authenticated user, or nil when the request is
// anonymous.
func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// FromRequest is FromContext for a request.
func FromRequest(r *http.Request) *User { return FromContext(r.Context()) }
