// Package blob keeps the key frame JPEGs in Azure Blob Storage (ADR 0006).
//
// The server never handles the JPEG bytes on the way in. It hands the
// browser a short-lived SAS URL (shared access signature: a URL with a
// signed query string) that allows exactly one thing: create one named blob,
// for 10 minutes. The browser PUTs the JPEG straight to Blob Storage. Later
// the server checks the blob (size, first bytes) and gives out read URLs
// that allow only reading that one blob, for 15 minutes.
//
// On Azure the SAS URLs are user delegation SAS: signed with a short-lived
// key that the app gets with its managed identity, so no account key exists
// anywhere. Locally and in CI the emulator Azurite is used, with its
// well-known development key; that key is refused for any other host.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
)

// How long the signed URLs work.
const (
	UploadTTL = 10 * time.Minute
	ReadTTL   = 15 * time.Minute
)

// AzuriteKey is the public, documented development key of the Azurite
// emulator (account devstoreaccount1). It is not a secret, and LoadConfig
// accepts it only for a loopback storage URL with a loopback app URL.
const AzuriteKey = "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="

// Config says where the key frames live and how the app signs in to Storage.
type Config struct {
	AccountURL       string // https://<account>.blob.core.windows.net, or Azurite's http://127.0.0.1:10000/devstoreaccount1
	Container        string // default key-frames
	IdentityClientID string // the user-assigned managed identity (Azure)
	AccountKey       string // Azurite only
}

// ErrNotConfigured means BLOB_ACCOUNT_URL is not set: key frames are off,
// and analyses are saved without them.
var ErrNotConfigured = errors.New("BLOB_ACCOUNT_URL is not set")

// LoadConfig reads BLOB_ACCOUNT_URL, BLOB_CONTAINER, BLOB_IDENTITY_CLIENT_ID
// and BLOB_ACCOUNT_KEY. publicURL is the app's own URL (PUBLIC_URL).
func LoadConfig(getenv func(string) string, publicURL string) (Config, error) {
	c := Config{
		AccountURL:       strings.TrimRight(getenv("BLOB_ACCOUNT_URL"), "/"),
		Container:        getenv("BLOB_CONTAINER"),
		IdentityClientID: getenv("BLOB_IDENTITY_CLIENT_ID"),
		AccountKey:       getenv("BLOB_ACCOUNT_KEY"),
	}
	if c.AccountURL == "" {
		return c, ErrNotConfigured
	}
	if c.Container == "" {
		c.Container = "key-frames"
	}
	u, err := url.Parse(c.AccountURL)
	if err != nil || u.Host == "" {
		return c, fmt.Errorf("BLOB_ACCOUNT_URL %q is not a URL", c.AccountURL)
	}
	local := isLoopback(u.Hostname())
	if !local && u.Scheme != "https" {
		return c, errors.New("BLOB_ACCOUNT_URL must use https")
	}
	if c.AccountKey != "" {
		p, err := url.Parse(publicURL)
		if !local || err != nil || !isLoopback(p.Hostname()) {
			return c, errors.New("BLOB_ACCOUNT_KEY is only for Azurite on localhost; on Azure the app uses its managed identity")
		}
		return c, nil
	}
	if c.IdentityClientID == "" {
		return c, errors.New("BLOB_IDENTITY_CLIENT_ID (the app's managed identity) must be set with BLOB_ACCOUNT_URL")
	}
	return c, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Store signs URLs for, and inspects, the key frame blobs of one container.
type Store struct {
	svc       *service.Client
	container string
	origin    string // scheme://host[:port] of the account, for the page's CSP
	local     bool   // Azurite over plain http

	sharedKey *service.SharedKeyCredential // Azurite only

	mu      sync.Mutex // guards the cached user delegation key
	udc     *service.UserDelegationCredential
	udcEnds time.Time
}

// New builds a Store. It does not call Azure yet.
func New(cfg Config) (*Store, error) {
	u, err := url.Parse(cfg.AccountURL)
	if err != nil {
		return nil, err
	}
	s := &Store{container: cfg.Container, origin: u.Scheme + "://" + u.Host, local: isLoopback(u.Hostname())}
	serviceURL := cfg.AccountURL + "/"
	if cfg.AccountKey != "" {
		// Azurite puts the account name in the path: /devstoreaccount1.
		account := strings.Trim(u.Path, "/")
		if s.sharedKey, err = service.NewSharedKeyCredential(account, cfg.AccountKey); err != nil {
			return nil, err
		}
		s.svc, err = service.NewClientWithSharedKeyCredential(serviceURL, s.sharedKey, nil)
		return s, err
	}
	cred, err := azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{
		ID: azidentity.ClientID(cfg.IdentityClientID),
	})
	if err != nil {
		return nil, err
	}
	s.svc, err = service.NewClient(serviceURL, cred, nil)
	return s, err
}

// Origin is the storage origin the browser talks to (for CSP connect-src and img-src).
func (s *Store) Origin() string { return s.origin }

// UploadURL returns a URL that may only create the named blob (no read, no
// overwrite, no delete), and when it stops working.
func (s *Store) UploadURL(ctx context.Context, name string) (string, time.Time, error) {
	expires := time.Now().Add(UploadTTL)
	u, err := s.sign(ctx, name, sas.BlobPermissions{Create: true}, expires, "")
	return u, expires, err
}

// ReadURL returns a URL that may only read the named blob. The response
// always says Content-Type image/jpeg, whatever was uploaded.
func (s *Store) ReadURL(ctx context.Context, name string) (string, error) {
	return s.sign(ctx, name, sas.BlobPermissions{Read: true}, time.Now().Add(ReadTTL), "image/jpeg")
}

func (s *Store) sign(ctx context.Context, name string, perm sas.BlobPermissions, expires time.Time, contentType string) (string, error) {
	v := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		ExpiryTime:    expires.UTC(),
		Permissions:   perm.String(),
		ContainerName: s.container,
		BlobName:      name,
		ContentType:   contentType,
	}
	if s.local {
		v.Protocol = sas.ProtocolHTTPSandHTTP // Azurite runs on plain http
	}
	var q sas.QueryParameters
	var err error
	if s.sharedKey != nil {
		q, err = v.SignWithSharedKey(s.sharedKey)
	} else {
		var udc *service.UserDelegationCredential
		if udc, err = s.delegationKey(ctx, expires); err == nil {
			q, err = v.SignWithUserDelegation(udc)
		}
	}
	if err != nil {
		return "", fmt.Errorf("blob: sign %s: %w", name, err)
	}
	return s.blobClient(name).URL() + "?" + q.Encode(), nil
}

// delegationKey returns a cached user delegation key that is valid until
// at least the given time, and asks Azure for a new one (valid 2 hours)
// when needed. A SAS signed with it cannot outlive the key.
func (s *Store) delegationKey(ctx context.Context, until time.Time) (*service.UserDelegationCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.udc != nil && s.udcEnds.After(until.Add(time.Minute)) {
		return s.udc, nil
	}
	now := time.Now().UTC()
	start, end := now.Add(-5*time.Minute), now.Add(2*time.Hour) // a little back, for clock skew
	st, en := start.Format(sas.TimeFormat), end.Format(sas.TimeFormat)
	udc, err := s.svc.GetUserDelegationCredential(ctx, service.KeyInfo{Start: &st, Expiry: &en}, nil)
	if err != nil {
		return nil, fmt.Errorf("get user delegation key: %w", err)
	}
	s.udc, s.udcEnds = udc, end
	return udc, nil
}

func (s *Store) blobClient(name string) *blob.Client {
	return s.svc.NewContainerClient(s.container).NewBlobClient(name)
}

// ErrNotFound means the blob does not exist (for example, the upload never happened).
var ErrNotFound = errors.New("blob not found")

// Info is what Inspect learns about a blob.
type Info struct {
	Size int64
	Head []byte // the first bytes (up to 8), to check the file type
}

// Inspect reads a blob's size and its first bytes.
func (s *Store) Inspect(ctx context.Context, name string) (Info, error) {
	c := s.blobClient(name)
	props, err := c.GetProperties(ctx, nil)
	if err != nil {
		return Info{}, notFound(err)
	}
	info := Info{}
	if props.ContentLength != nil {
		info.Size = *props.ContentLength
	}
	if info.Size == 0 {
		return info, nil
	}
	res, err := c.DownloadStream(ctx, &blob.DownloadStreamOptions{Range: blob.HTTPRange{Offset: 0, Count: min(8, info.Size)}})
	if err != nil {
		return Info{}, notFound(err)
	}
	defer func() { _ = res.Body.Close() }()
	info.Head, err = io.ReadAll(io.LimitReader(res.Body, 8))
	return info, err
}

// Delete removes a blob. A missing blob is not an error.
func (s *Store) Delete(ctx context.Context, name string) error {
	_, err := s.blobClient(name).Delete(ctx, nil)
	if err = notFound(err); errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func notFound(err error) error {
	if bloberror.HasCode(err, bloberror.BlobNotFound, bloberror.ContainerNotFound) {
		return ErrNotFound
	}
	var re *azcore.ResponseError
	if errors.As(err, &re) && re.StatusCode == 404 {
		return ErrNotFound
	}
	return err
}
