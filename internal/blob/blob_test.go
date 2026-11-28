package blob_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/v1d1tt/talent-bridge/internal/blob"
)

// newStore connects to Azurite (the local Azure Storage emulator). Tests
// skip when TEST_BLOB_URL is not set; `make azurite` starts it.
func newStore(t *testing.T) *blob.Store {
	t.Helper()
	u := os.Getenv("TEST_BLOB_URL")
	if u == "" {
		t.Skip("TEST_BLOB_URL is not set")
	}
	cfg := blob.Config{AccountURL: u, Container: "test-" + strings.ToLower(rand.Text()[:10]), AccountKey: blob.AzuriteKey}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := blob.SetupDev(ctx, cfg, []string{"http://localhost:8080"}); err != nil {
		t.Fatal(err)
	}
	s, err := blob.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte("x"), 100)...)

func put(t *testing.T, u string, body []byte) int {
	t.Helper()
	return putType(t, u, body, "image/jpeg")
}

func putType(t *testing.T, u string, body []byte, contentType string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, u, bytes.NewReader(body))
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	req.Header.Set("Content-Type", contentType)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res.StatusCode
}

func get(t *testing.T, u string) (int, string, []byte) {
	t.Helper()
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Content-Type"), b
}

func TestUploadAndReadURLs(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	name := "analyses/a1/0.jpg"

	up, expires, err := s.UploadURL(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(expires); d <= 0 || d > blob.UploadTTL {
		t.Errorf("upload URL expires in %v, want within %v", d, blob.UploadTTL)
	}
	// The browser could claim any type; the read URL must still say image/jpeg.
	if code := putType(t, up, jpeg, "text/html"); code != http.StatusCreated {
		t.Fatalf("PUT with the upload URL = %d, want 201", code)
	}
	if code := put(t, up, []byte("<html>replaced</html>")); code < 400 {
		t.Errorf("second PUT with the same upload URL = %d, want a refusal (create only, no overwrite)", code)
	}
	if code, _, _ := get(t, up); code != http.StatusForbidden {
		t.Errorf("GET with the upload URL = %d, want 403 (it may only create)", code)
	}

	read, err := s.ReadURL(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	code, ctype, body := get(t, read)
	if code != http.StatusOK || !bytes.Equal(body, jpeg) {
		t.Fatalf("GET with the read URL = %d (%d bytes), want 200 and the JPEG", code, len(body))
	}
	if ctype != "image/jpeg" {
		t.Errorf("read URL Content-Type = %q, want image/jpeg (forced by the SAS, whatever was uploaded)", ctype)
	}
	if code := put(t, read, jpeg); code != http.StatusForbidden {
		t.Errorf("PUT with the read URL = %d, want 403", code)
	}

	// A SAS is signed for one blob name: pointing it at another blob fails.
	other := strings.Replace(up, "/0.jpg?", "/1.jpg?", 1)
	if code := put(t, other, jpeg); code != http.StatusForbidden {
		t.Errorf("PUT to another blob with a moved upload URL = %d, want 403", code)
	}
	// Changing the signed expiry breaks the signature.
	u, _ := url.Parse(read)
	q := u.Query()
	q.Set("se", time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339))
	u.RawQuery = q.Encode()
	if code, _, _ := get(t, u.String()); code != http.StatusForbidden {
		t.Errorf("GET with an edited expiry = %d, want 403", code)
	}
}

func TestInspectAndDelete(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	name := "analyses/a2/5.jpg"
	if _, err := s.Inspect(ctx, name); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("Inspect(missing) = %v, want ErrNotFound", err)
	}
	up, _, err := s.UploadURL(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if code := put(t, up, jpeg); code != http.StatusCreated {
		t.Fatalf("PUT = %d", code)
	}
	info, err := s.Inspect(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(jpeg)) || !bytes.Equal(info.Head, jpeg[:len(info.Head)]) || len(info.Head) < 3 {
		t.Errorf("Inspect = size %d head %x, want size %d and the first bytes", info.Size, info.Head, len(jpeg))
	}
	if err := s.Delete(ctx, name); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inspect(ctx, name); !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Inspect after Delete = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, name); err != nil {
		t.Errorf("Delete of a missing blob = %v, want nil", err)
	}
}

func TestLoadConfig(t *testing.T) {
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	const azure = "https://tbframes1.blob.core.windows.net"
	tests := []struct {
		name    string
		env     map[string]string
		public  string
		wantErr string // "" = no error; "off" = ErrNotConfigured
	}{
		{"nothing set: key frames off", map[string]string{}, "https://app.example", "off"},
		{"production with managed identity", map[string]string{"BLOB_ACCOUNT_URL": azure, "BLOB_IDENTITY_CLIENT_ID": "abc"}, "https://app.example", ""},
		{"production without an identity", map[string]string{"BLOB_ACCOUNT_URL": azure}, "https://app.example", "BLOB_IDENTITY_CLIENT_ID"},
		{"production: an account key is refused", map[string]string{"BLOB_ACCOUNT_URL": azure, "BLOB_ACCOUNT_KEY": blob.AzuriteKey}, "https://app.example", "only for Azurite"},
		{"production: plain http is refused", map[string]string{"BLOB_ACCOUNT_URL": "http://tbframes1.blob.core.windows.net", "BLOB_IDENTITY_CLIENT_ID": "abc"}, "https://app.example", "https"},
		{"local Azurite with the key", map[string]string{"BLOB_ACCOUNT_URL": "http://127.0.0.1:10000/devstoreaccount1", "BLOB_ACCOUNT_KEY": blob.AzuriteKey}, "http://localhost:8080", ""},
		{"Azurite key while the app is public", map[string]string{"BLOB_ACCOUNT_URL": "http://127.0.0.1:10000/devstoreaccount1", "BLOB_ACCOUNT_KEY": blob.AzuriteKey}, "https://app.example", "only for Azurite"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := blob.LoadConfig(env(tc.env), tc.public)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("err = %v, want nil", err)
			case tc.wantErr == "off" && !errors.Is(err, blob.ErrNotConfigured):
				t.Errorf("err = %v, want ErrNotConfigured", err)
			case tc.wantErr != "" && tc.wantErr != "off" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("err = %v, want one that mentions %q", err, tc.wantErr)
			}
		})
	}
}
