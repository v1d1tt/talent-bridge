package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
)

// Access says who may call a route. The zero value lets nobody in, so a route
// registered without a rule is denied: deny by default.
type Access struct {
	public   bool
	optional bool   // anyone; the session is loaded when there is one
	signedIn bool   // any signed-in user, also one who has not chosen a role
	roles    []Role // signed in with one of these roles
}

var (
	// Public routes need no sign-in (health check, the sign-in flow itself).
	Public = Access{public: true}
	// Optional routes answer everyone, and see the session if there is one
	// (GET /api/me: "who is signed in", null for nobody).
	Optional = Access{optional: true}
	// SignedIn routes need an auth session; the role may still be unset.
	SignedIn = Access{signedIn: true}
)

// Roles allows signed-in users with one of the given roles.
func Roles(roles ...Role) Access { return Access{roles: roles} }

// String names the rule, for test output.
func (a Access) String() string {
	switch {
	case a.public:
		return "public"
	case a.optional:
		return "optional"
	case a.signedIn:
		return "signed in"
	case len(a.roles) > 0:
		return "roles " + fmt.Sprint(a.roles)
	}
	return "nobody"
}

// Guard checks every request to next against the rule, in this order:
//
//	no rule                       -> 403 forbidden
//	no live auth session          -> 401 signed_out (Optional: go on signed out)
//	banned user                   -> 403 banned (all their sessions are deleted)
//	state-changing, no CSRF token -> 403 csrf
//	role not allowed              -> 403 forbidden
//
// Responses of guarded routes are never cached.
func (s *Service) Guard(a Access, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.public {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if !a.optional && !a.signedIn && len(a.roles) == 0 {
			slog.ErrorContext(r.Context(), "auth: route has no access rule", "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		sess, err := s.load(r)
		switch {
		case errors.Is(err, errSignedOut), errors.Is(err, errBanned):
			if _, cerr := r.Cookie(SessionCookie); cerr == nil {
				clearCookie(w, SessionCookie) // a dead or banned session: drop the cookie
			}
			switch {
			case a.optional:
				next.ServeHTTP(w, r) // no session in the context
			case errors.Is(err, errBanned):
				slog.WarnContext(r.Context(), "auth: banned user refused", "path", r.URL.Path)
				writeError(w, http.StatusForbidden, "banned")
			default:
				writeError(w, http.StatusUnauthorized, "signed_out")
			}
			return
		case err != nil:
			slog.ErrorContext(r.Context(), "auth: load session", "err", err)
			writeError(w, http.StatusServiceUnavailable, "unavailable")
			return
		}

		if !safeMethod(r.Method) && !sess.validCSRF(r.Header.Get(CSRFHeader)) {
			slog.WarnContext(r.Context(), "auth: CSRF token missing or wrong", "user", sess.User.ID, "method", r.Method, "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "csrf")
			return
		}
		if len(a.roles) > 0 && !slices.Contains(a.roles, sess.User.Role) {
			slog.WarnContext(r.Context(), "auth: role refused", "user", sess.User.ID, "role", string(sess.User.Role), "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

// safeMethod reports methods that must not change state (RFC 9110 9.2.1).
// Everything else needs the CSRF token.
func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
