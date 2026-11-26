// Package fakegoogle is a small OpenID Connect provider that acts like Google
// sign-in, for tests and for local development without a Google account.
//
// It does the parts the server must get right: discovery, an authorization
// page, a token endpoint that checks the client secret, the redirect URI and
// the PKCE verifier (S256), and RS256-signed ID tokens with a JWKS endpoint.
// It is never part of the production image: only cmd/fake-google and tests
// import it, and the server accepts a non-Google issuer only on localhost
// (see auth.LoadConfig).
package fakegoogle

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Identity is the Google account that "signs in" on the fake page.
type Identity struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
}

// Provider is the fake. Create it with New and serve Handler().
type Provider struct {
	issuer       string
	clientID     string
	clientSecret string
	key          *rsa.PrivateKey
	signer       jose.Signer

	mu    sync.Mutex
	codes map[string]grant // authorization code -> what it was issued for
}

type grant struct {
	identity      Identity
	redirectURI   string
	codeChallenge string
	nonce         string
	expires       time.Time
}

const keyID = "fake-google-1"

// New returns a provider whose issuer is issuerURL (the URL it is served at,
// for example "http://localhost:9999"), with one registered client.
func New(issuerURL, clientID, clientSecret string) (*Provider, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID))
	if err != nil {
		return nil, err
	}
	return &Provider{
		issuer:       strings.TrimSuffix(issuerURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		key:          key,
		signer:       signer,
		codes:        map[string]grant{},
	}, nil
}

// Handler serves the provider's endpoints.
func (p *Provider) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("GET /o/oauth2/v2/auth", p.authorizePage)
	mux.HandleFunc("POST /o/oauth2/v2/auth", p.authorize)
	mux.HandleFunc("POST /token", p.token)
	mux.HandleFunc("GET /oauth2/v3/certs", p.jwks)
	return mux
}

func (p *Provider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                p.issuer,
		"authorization_endpoint":                p.issuer + "/o/oauth2/v2/auth",
		"token_endpoint":                        p.issuer + "/token",
		"jwks_uri":                              p.issuer + "/oauth2/v3/certs",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"scopes_supported":                      []string{"openid", "email", "profile"},
	})
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><title>Fake Google sign-in</title>
<style>body{font:16px system-ui;max-width:28rem;margin:3rem auto;padding:0 1rem}label{display:block;margin:.75rem 0}input[type=text],input[type=email]{width:100%;padding:.4rem}</style>
<h1>Fake Google sign-in</h1>
<p>Local development only. This is not Google. Type any account.</p>
<form method="post">
{{range $k, $v := .Query}}<input type="hidden" name="{{$k}}" value="{{index $v 0}}">{{end}}
<label>Subject (Google user id) <input type="text" name="sub" value="fake-player-1" required></label>
<label>Email <input type="email" name="email" value="player@example.com" required></label>
<label>Name <input type="text" name="name" value="Test Player"></label>
<label><input type="checkbox" name="email_verified" value="true" checked> Email verified</label>
<button type="submit">Continue</button>
</form>`))

// authorizePage shows a form instead of Google's account chooser.
func (p *Provider) authorizePage(w http.ResponseWriter, r *http.Request) {
	if msg := p.checkAuthRequest(r.URL.Query()); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, map[string]url.Values{"Query": r.URL.Query()})
}

// authorize takes the form (the original query plus the identity) and sends
// the browser back to the client with a one-time code, like Google does.
func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	q := r.PostForm
	if msg := p.checkAuthRequest(q); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	id := Identity{Sub: q.Get("sub"), Email: q.Get("email"), EmailVerified: q.Get("email_verified") == "true", Name: q.Get("name")}
	if id.Sub == "" || id.Email == "" {
		http.Error(w, "sub and email are required", http.StatusBadRequest)
		return
	}
	code := randomString()
	p.mu.Lock()
	p.codes[code] = grant{
		identity:      id,
		redirectURI:   q.Get("redirect_uri"),
		codeChallenge: q.Get("code_challenge"),
		nonce:         q.Get("nonce"),
		expires:       time.Now().Add(time.Minute),
	}
	p.mu.Unlock()

	back, _ := url.Parse(q.Get("redirect_uri"))
	bq := back.Query()
	bq.Set("code", code)
	bq.Set("state", q.Get("state"))
	back.RawQuery = bq.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// checkAuthRequest rejects what Google would reject.
func (p *Provider) checkAuthRequest(q url.Values) string {
	switch {
	case q.Get("client_id") != p.clientID:
		return "unknown client_id"
	case q.Get("response_type") != "code":
		return "response_type must be code"
	case q.Get("redirect_uri") == "":
		return "redirect_uri is required"
	case !strings.Contains(" "+q.Get("scope")+" ", " openid "):
		return "scope must include openid"
	case q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "":
		return "PKCE S256 is required"
	case q.Get("state") == "":
		return "state is required"
	}
	return ""
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, "invalid_request")
		return
	}
	id, secret, ok := r.BasicAuth()
	if ok { // client_secret_basic: the values are form-encoded first (RFC 6749 2.3.1)
		id, _ = url.QueryUnescape(id)
		secret, _ = url.QueryUnescape(secret)
	} else {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if id != p.clientID || subtle.ConstantTimeCompare([]byte(secret), []byte(p.clientSecret)) != 1 {
		tokenError(w, "invalid_client")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		tokenError(w, "unsupported_grant_type")
		return
	}

	code := r.PostForm.Get("code")
	p.mu.Lock()
	g, found := p.codes[code]
	delete(p.codes, code) // a code works once
	p.mu.Unlock()
	if !found || time.Now().After(g.expires) || g.redirectURI != r.PostForm.Get("redirect_uri") {
		tokenError(w, "invalid_grant")
		return
	}
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != g.codeChallenge {
		tokenError(w, "invalid_grant") // PKCE: the verifier does not match the challenge
		return
	}

	now := time.Now()
	raw, err := jwt.Signed(p.signer).Claims(jwt.Claims{
		Issuer:   p.issuer,
		Subject:  g.identity.Sub,
		Audience: jwt.Audience{p.clientID},
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
	}).Claims(map[string]any{
		"nonce":          g.nonce,
		"email":          g.identity.Email,
		"email_verified": g.identity.EmailVerified,
		"name":           g.identity.Name,
		"azp":            p.clientID,
	}).Serialize()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": randomString(),
		"token_type":   "Bearer",
		"expires_in":   3600,
		"scope":        "openid email profile",
		"id_token":     raw,
	})
}

func (p *Provider) jwks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &p.key.PublicKey, KeyID: keyID, Algorithm: string(jose.RS256), Use: "sig",
	}}})
}

func tokenError(w http.ResponseWriter, code string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomString() string { return rand.Text() }
