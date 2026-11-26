package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/v1d1tt/talent-bridge/internal/store"
)

// SessionCookie holds the auth session id. The __Host- prefix makes the
// browser accept it only with Secure, Path=/ and no Domain, so a subdomain
// cannot set or overwrite it.
const SessionCookie = "__Host-tb_session"

// CSRFHeader is the request header that carries the CSRF token.
const CSRFHeader = "X-CSRF-Token"

// User is the signed-in user.
type User struct {
	ID          string
	Email       string
	DisplayName string
	Role        Role
}

// Session is a live auth session, put in the request context by Guard.
type Session struct {
	User      User
	csrfToken string
	idHash    []byte
}

type ctxKey struct{}

// SessionFrom returns the auth session that Guard found for this request.
func SessionFrom(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(ctxKey{}).(*Session)
	return s, ok
}

var (
	errSignedOut = errors.New("no live auth session")
	errBanned    = errors.New("user is banned")
)

// newSessionID returns a random 256-bit id (sent in the cookie) and its
// SHA-256 hash (stored in the database).
func newSessionID() (id string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	sum := sha256.Sum256(b)
	return base64.RawURLEncoding.EncodeToString(b), sum[:]
}

// hashSessionID decodes the cookie value and hashes it. A value that is not
// exactly 32 random bytes cannot be a session id we issued.
func hashSessionID(id string) ([]byte, bool) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(b) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(b)
	return sum[:], true
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true, // page scripts cannot read it
		Secure:   true, // only over HTTPS (browsers treat localhost as secure)
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(w http.ResponseWriter, name string) { setCookie(w, name, "", -1) }

// startSession creates a new auth session for the user and sets the cookie.
func (s *Service) startSession(ctx context.Context, w http.ResponseWriter, userID pgtype.UUID) error {
	id, hash := newSessionID()
	err := s.q.CreateAuthSession(ctx, store.CreateAuthSessionParams{
		IDHash:          hash,
		UserID:          userID,
		CsrfToken:       rand.Text(),
		LifetimeSeconds: int32(s.Lifetime.Seconds()),
	})
	if err != nil {
		return err
	}
	setCookie(w, SessionCookie, id, int(s.Lifetime.Seconds()))
	return nil
}

// load finds the live auth session for the request's cookie and marks it as
// used. A banned user's sessions are all deleted here.
func (s *Service) load(r *http.Request) (*Session, error) {
	if !s.enabled {
		return nil, errSignedOut
	}
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil, errSignedOut
	}
	hash, ok := hashSessionID(c.Value)
	if !ok {
		return nil, errSignedOut
	}
	row, err := s.q.TouchAuthSession(r.Context(), store.TouchAuthSessionParams{
		IDHash: hash, IdleSeconds: int32(s.IdleTimeout.Seconds()),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errSignedOut
	}
	if err != nil {
		return nil, err
	}
	if row.BannedAt.Valid {
		if err := s.q.DeleteUserAuthSessions(r.Context(), row.UserID); err != nil {
			slog.ErrorContext(r.Context(), "auth: delete banned user's sessions", "err", err)
		}
		return nil, errBanned
	}
	role := roleOf(row.Role)
	if role == RoleAdmin && !slices.Contains(s.cfg.AdminEmails, strings.ToLower(row.Email)) {
		// Removed from ADMIN_EMAILS: the admin role ends now, not at the next
		// sign-in. (The stored email of an admin was verified at sign-in.)
		role = RoleNone
	}
	return &Session{
		User: User{
			ID:          row.UserID.String(),
			Email:       row.Email,
			DisplayName: row.DisplayName,
			Role:        role,
		},
		csrfToken: row.CsrfToken,
		idHash:    hash,
	}, nil
}

func (sess *Session) validCSRF(token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(sess.csrfToken)) == 1
}

func roleOf(r store.NullUserRole) Role {
	if !r.Valid {
		return RoleNone
	}
	return Role(r.UserRole)
}

// meJSON is the body of GET /api/me. Signed out, it is {"user": null}.
type meJSON struct {
	User      *userJSON `json:"user"`
	CSRFToken string    `json:"csrf_token,omitempty"`
}

type userJSON struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	Role        *string `json:"role"` // null until chosen
}

func (sess *Session) toJSON() meJSON {
	u := &userJSON{ID: sess.User.ID, Email: sess.User.Email, DisplayName: sess.User.DisplayName}
	if sess.User.Role != RoleNone {
		r := string(sess.User.Role)
		u.Role = &r
	}
	return meJSON{User: u, CSRFToken: sess.csrfToken}
}

// Me serves GET /api/me: who is signed in, the role and the CSRF token, or
// {"user": null}. Guard it with Optional, so a visitor's page load is a
// normal 200 and not an error.
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusOK, meJSON{})
		return
	}
	writeJSON(w, http.StatusOK, sess.toJSON())
}

// ChooseRole serves POST /api/me/role {"role":"athlete"|"coach"}: the
// one-time choice at first sign-in. Admin is never a choice; it comes from
// config. Guard it with SignedIn.
func (s *Service) ChooseRole(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	var body struct {
		Role string `json:"role"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || (body.Role != string(RoleAthlete) && body.Role != string(RoleCoach)) {
		writeError(w, http.StatusUnprocessableEntity, "invalid_role")
		return
	}
	var uid pgtype.UUID
	if err := uid.Scan(sess.User.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	role, err := s.q.ChooseRole(r.Context(), store.ChooseRoleParams{
		ID: uid, Role: store.NullUserRole{UserRole: store.UserRole(body.Role), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "role_already_set")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "auth: choose role", "err", err)
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	sess.User.Role = roleOf(role)
	writeJSON(w, http.StatusOK, sess.toJSON())
}

// SignOut serves POST /auth/signout: it deletes the auth session row and
// clears the cookie. Guard it with SignedIn (so it needs the CSRF token).
func (s *Service) SignOut(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	if err := s.q.DeleteAuthSession(r.Context(), sess.idHash); err != nil {
		slog.ErrorContext(r.Context(), "auth: delete session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal")
		return
	}
	clearCookie(w, SessionCookie)
	w.WriteHeader(http.StatusNoContent)
}

// Ban blocks a user from signing in and deletes all their auth sessions, so
// they are signed out everywhere at once.
func (s *Service) Ban(ctx context.Context, userID string) error {
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return err
	}
	if err := s.q.BanUser(ctx, uid); err != nil {
		return err
	}
	return s.q.DeleteUserAuthSessions(ctx, uid)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
