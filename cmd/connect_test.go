package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Knovigator/treecli/api"
	"github.com/spf13/viper"
)

// fakeAuthServer is a backend with OAuth discovery, the device grant, refresh
// and a bearer-only /mcp endpoint.
type fakeAuthServer struct {
	*httptest.Server
	mu          sync.Mutex
	polls       int
	refreshes   int
	validBearer string
	grantTypes  []string
	// Lifetime of refreshed tokens; under oauthRefreshSkew they are never fresh.
	refreshExpiresIn int
}

func newFakeAuthServer(t *testing.T) *fakeAuthServer {
	f := &fakeAuthServer{validBearer: "access-1", refreshExpiresIn: 900}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/mcp":
			fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, f.URL+"/mcp", f.URL)
		case "/.well-known/oauth-authorization-server":
			fmt.Fprintf(w, `{"token_endpoint":%q,"device_authorization_endpoint":%q}`, f.URL+"/mcp/oauth/token", f.URL+"/mcp/oauth/device_authorization")
		case "/mcp/oauth/device_authorization":
			if r.FormValue("client_id") != "treecli" || r.FormValue("resource") != f.URL+"/mcp" {
				t.Errorf("unexpected device request %v", r.Form)
			}
			fmt.Fprintf(w, `{"device_code":"dev-code","user_code":"BCDF-GHJK","verification_uri":%q,"verification_uri_complete":%q,"expires_in":60,"interval":1}`,
				f.URL+"/mcp/oauth/device", f.URL+"/mcp/oauth/device?user_code=BCDF-GHJK")
		case "/mcp/oauth/token":
			f.grantTypes = append(f.grantTypes, r.FormValue("grant_type"))
			switch r.FormValue("grant_type") {
			case deviceCodeGrantType:
				f.polls++
				if f.polls == 1 {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprint(w, `{"error":"authorization_pending"}`)
					return
				}
				fmt.Fprint(w, `{"access_token":"access-1","refresh_token":"refresh-1","expires_in":900,"token_type":"Bearer"}`)
			case "refresh_token":
				f.refreshes++
				f.validBearer = fmt.Sprintf("access-r%d", f.refreshes)
				fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-r%d","expires_in":%d}`, f.validBearer, f.refreshes, f.refreshExpiresIn)
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
		case "/mcp":
			if r.Header.Get("Authorization") != "Bearer "+f.validBearer {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"invalid_token"}`)
				return
			}
			var body struct {
				Params struct {
					Name string `json:"name"`
				} `json:"params"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			switch body.Params.Name {
			case "treechat_account":
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[],"structuredContent":{"id":"user-1","username":"tree"}}}`)
			case "treechat_billing":
				fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{"content":[],"structuredContent":{"stripe":{"subscription_status":"active"}}}}`)
			default:
				t.Errorf("unexpected tool %q", body.Params.Name)
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func useTempConfig(t *testing.T, backendURL string) {
	viper.Reset()
	viper.SetConfigFile(filepath.Join(t.TempDir(), "config.toml"))
	viper.SetConfigType("toml")
	t.Cleanup(viper.Reset)
	t.Setenv("TREECLI_PROFILE", "")
	t.Setenv("TREECTL_PROFILE", "")
	previousBackend, previousProfile := BackendURLOverride, SelectedProfile
	BackendURLOverride, SelectedProfile = backendURL, ""
	t.Cleanup(func() { BackendURLOverride, SelectedProfile = previousBackend, previousProfile })
	t.Cleanup(func() { api.SetOAuthRefresher(nil) })
}

func TestConnectRunsTheDeviceGrantAndSavesARefreshableBearer(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, server.URL)
	connectNoBrowser, connectBillingWait = true, 0
	t.Cleanup(func() { connectNoBrowser, connectBillingWait = false, 3*time.Minute })

	var out bytes.Buffer
	ConnectCmd.SetOut(&out)
	t.Cleanup(func() { ConnectCmd.SetOut(nil) })
	if err := runConnect(ConnectCmd, nil); err != nil {
		t.Fatalf("connect: %v", err)
	}

	for _, want := range []string{"BCDF-GHJK", "/mcp/oauth/device?user_code=BCDF-GHJK", "Connected as @tree", "Card billing is on"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	if server.polls != 2 {
		t.Fatalf("expected one pending poll then success, got %d polls", server.polls)
	}

	profile, err := resolveProfile(resolveProfileName())
	if err != nil {
		t.Fatal(err)
	}
	if profile.Client != api.OAuthClient || profile.AccessToken != "access-1" || profile.RefreshToken != "refresh-1" ||
		profile.UID != "user-1" || profile.OAuthTokenURL != server.URL+"/mcp/oauth/token" || profile.OAuthResource != server.URL+"/mcp" {
		t.Fatalf("unexpected saved profile: %#v", profile)
	}
}

func TestOAuthProfileRefreshesBeforeExpiryAndWhenTheServerRejectsIt(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, server.URL)
	expired := profileConfig{
		Name: "prod", BackendURL: server.URL, AccessToken: "access-old", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-1",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	if err := saveProfile(expired, true); err != nil {
		t.Fatal(err)
	}

	profile, err := requireAuthenticatedProfile()
	if err != nil {
		t.Fatalf("refresh before use: %v", err)
	}
	if profile.AccessToken != "access-r1" || profile.RefreshToken != "refresh-r1" {
		t.Fatalf("expected refreshed credentials, got %#v", profile)
	}

	// The server revokes the bearer mid-session: the client refreshes once and retries.
	server.mu.Lock()
	server.validBearer = "rotated-elsewhere"
	server.mu.Unlock()
	api.SetOAuthRefresher(func(stale string) (string, error) {
		server.mu.Lock()
		defer server.mu.Unlock()
		server.validBearer = "access-after-401"
		return "access-after-401", nil
	})
	identity, err := fetchIdentity(profile)
	if err != nil || identity.UserID != "user-1" {
		t.Fatalf("expected retry after refresh, got %#v, %v", identity, err)
	}

	saved, _ := resolveProfile(resolveProfileName())
	if saved.AccessToken != "access-r1" || saved.RefreshToken != "refresh-r1" {
		t.Fatalf("refreshed credentials were not saved: %#v", saved)
	}
}

func TestPasswordLoginClearsConnectCredentials(t *testing.T) {
	profile := profileConfig{RefreshToken: "r", OAuthTokenURL: "https://x/token", OAuthResource: "https://x/mcp"}
	clearOAuthCredentials(&profile)
	if profile.RefreshToken != "" || profile.OAuthTokenURL != "" || profile.OAuthResource != "" {
		t.Fatalf("connect credentials survived: %#v", profile)
	}
}

func TestRefreshSavesOnlyTokensNotPerCommandOverrides(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, "")
	stored := profileConfig{
		Name: "prod", BackendURL: "https://saved.example.test", AccessToken: "access-old", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-1",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	if err := saveProfile(stored, true); err != nil {
		t.Fatal(err)
	}
	BackendURLOverride = server.URL

	if _, err := requireAuthenticatedProfile(); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := viper.GetString("profiles.prod.backend_url"); got != "https://saved.example.test" {
		t.Fatalf("refresh saved the --backend-url override: %q", got)
	}
	if got := viper.GetString("profiles.prod.refresh_token"); got != "refresh-r1" {
		t.Fatalf("refreshed token not saved: %q", got)
	}
}

func TestRefreshAdoptsATokenAnotherProcessAlreadyRenewed(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, server.URL)
	expired := profileConfig{
		Name: "prod", BackendURL: server.URL, AccessToken: "access-old", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-old",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	if err := saveProfile(expired, true); err != nil {
		t.Fatal(err)
	}
	// Another treecli process renews and saves while this one still holds the old token.
	other := viper.New()
	other.SetConfigFile(viper.ConfigFileUsed())
	other.SetConfigType("toml")
	if err := other.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	other.Set("profiles.prod.access_token", "access-from-other")
	other.Set("profiles.prod.refresh_token", "refresh-from-other")
	other.Set("profiles.prod.expiry", time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
	if err := other.WriteConfig(); err != nil {
		t.Fatal(err)
	}

	profile := expired
	if err := renewOAuthProfile(&profile, "access-old"); err != nil {
		t.Fatal(err)
	}
	if profile.AccessToken != "access-from-other" || server.refreshes != 0 {
		t.Fatalf("expected to reuse the saved token without refreshing, got %q after %d refreshes", profile.AccessToken, server.refreshes)
	}
}

func TestRefreshingOneAccountKeepsAnotherAccountsNewerTokens(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, server.URL)
	base := profileConfig{
		BackendURL: server.URL, Client: api.OAuthClient, UID: "user-1",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	mine := base
	mine.Name, mine.AccessToken, mine.RefreshToken = "prod", "access-old", "refresh-1"
	mine.Expiry = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	theirs := base
	theirs.Name, theirs.AccessToken, theirs.RefreshToken = "alt", "alt-old", "alt-refresh-old"
	theirs.Expiry = mine.Expiry
	for _, profile := range []profileConfig{theirs, mine} {
		if err := saveProfile(profile, true); err != nil {
			t.Fatal(err)
		}
	}

	// Another process renews the "alt" account after this one loaded its config.
	other := viper.New()
	other.SetConfigFile(viper.ConfigFileUsed())
	other.SetConfigType("toml")
	if err := other.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	other.Set("profiles.alt.refresh_token", "alt-refresh-new")
	if err := other.WriteConfig(); err != nil {
		t.Fatal(err)
	}

	if err := renewOAuthProfile(&mine, "access-old"); err != nil {
		t.Fatal(err)
	}

	onDisk := viper.New()
	onDisk.SetConfigFile(viper.ConfigFileUsed())
	onDisk.SetConfigType("toml")
	if err := onDisk.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if got := onDisk.GetString("profiles.alt.refresh_token"); got != "alt-refresh-new" {
		t.Fatalf("refreshing prod overwrote alt's newer token with %q", got)
	}
	if got := onDisk.GetString("profiles.prod.refresh_token"); got != "refresh-r1" {
		t.Fatalf("prod's refreshed token was not saved: %q", got)
	}
}

func TestShowRedactsTheRefreshToken(t *testing.T) {
	redacted := redactProfile(profileConfig{AccessToken: "access-token-value", RefreshToken: "refresh-token-value"})
	data, _ := json.Marshal(redacted)
	if bytes.Contains(data, []byte("refresh-token-value")) {
		t.Fatalf("refresh token leaked: %s", data)
	}
}

func TestRefreshOfAnAccountInheritingALegacyLoginSavesToTheLegacyProfile(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, "")
	oldEnv, oldAccount, oldSelection := SelectedEnvironment, SelectedAccount, accountSelection
	t.Cleanup(func() { SelectedEnvironment, SelectedAccount, accountSelection = oldEnv, oldAccount, oldSelection })
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL"} {
		t.Setenv(name, "")
	}
	// A login saved with the legacy `--profile qa` interface.
	legacy := profileConfig{
		Name: "qa", BackendURL: server.URL, AccessToken: "access-old", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-1",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	if err := saveProfile(legacy, false); err != nil {
		t.Fatal(err)
	}
	viper.Set("environments.qa.backend_url", server.URL)

	// The account interface inherits it and refreshes twice in a row.
	accountSelection, SelectedEnvironment, SelectedAccount = true, "qa", "qa"
	first, err := requireAuthenticatedProfile()
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if first.AccessToken != "access-r1" {
		t.Fatalf("expected a refreshed token, got %q", first.AccessToken)
	}
	if got := viper.GetString("profiles.qa.refresh_token"); got != "refresh-r1" {
		t.Fatalf("renewed token not saved where it is read from: %q", got)
	}
	viper.Set("profiles.qa.expiry", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339))
	second, err := requireAuthenticatedProfile()
	if err != nil || second.AccessToken != "access-r2" {
		t.Fatalf("second refresh should use the rotated token: %q, %v", second.AccessToken, err)
	}
}

func TestALongCommandRenewsAgainOnceTheReusedTokenExpires(t *testing.T) {
	server := newFakeAuthServer(t)
	server.refreshExpiresIn = 30 // shorter than the refresh skew: never fresh
	useTempConfig(t, server.URL)
	profile := profileConfig{
		Name: "prod", BackendURL: server.URL, AccessToken: "access-0", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-0",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	if err := saveProfile(profile, true); err != nil {
		t.Fatal(err)
	}
	if err := prepareOAuthProfile(&profile); err != nil {
		t.Fatal(err)
	}

	// Each call starts from the command's original token, as SharedOperation does.
	for round := 1; round <= 2; round++ {
		server.mu.Lock()
		server.validBearer = fmt.Sprintf("rejects-everything-%d", round)
		server.mu.Unlock()
		if _, err := fetchIdentity(profile); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	if server.refreshes != 2 {
		t.Fatalf("expected a second renewal after the first replacement expired, got %d", server.refreshes)
	}
}

func TestSavingAConnectionKeepsAnotherAccountsNewerTokens(t *testing.T) {
	useTempConfig(t, "https://example.test")
	if err := saveProfile(profileConfig{Name: "alt", BackendURL: "https://example.test", RefreshToken: "alt-old"}, false); err != nil {
		t.Fatal(err)
	}
	other := viper.New()
	other.SetConfigFile(viper.ConfigFileUsed())
	other.SetConfigType("toml")
	if err := other.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	other.Set("profiles.alt.refresh_token", "alt-new")
	if err := other.WriteConfig(); err != nil {
		t.Fatal(err)
	}

	if err := saveProfile(profileConfig{Name: "prod", BackendURL: "https://example.test", AccessToken: "a", Client: api.OAuthClient, UID: "u"}, true); err != nil {
		t.Fatal(err)
	}
	onDisk := viper.New()
	onDisk.SetConfigFile(viper.ConfigFileUsed())
	onDisk.SetConfigType("toml")
	if err := onDisk.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if got := onDisk.GetString("profiles.alt.refresh_token"); got != "alt-new" {
		t.Fatalf("saving prod restored alt's stale token: %q", got)
	}
}

func TestRenewalStopsWhenTheSavedLoginChanged(t *testing.T) {
	server := newFakeAuthServer(t)
	useTempConfig(t, server.URL)
	expired := profileConfig{
		Name: "prod", BackendURL: server.URL, AccessToken: "access-old", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-old",
		OAuthTokenURL: server.URL + "/mcp/oauth/token", OAuthResource: server.URL + "/mcp",
	}
	if err := saveProfile(expired, true); err != nil {
		t.Fatal(err)
	}
	// Another invocation reconnected this section to a different server.
	other := viper.New()
	other.SetConfigFile(viper.ConfigFileUsed())
	other.SetConfigType("toml")
	if err := other.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	other.Set("profiles.prod.access_token", "other-server-token")
	other.Set("profiles.prod.refresh_token", "other-server-refresh")
	other.Set("profiles.prod.expiry", time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339))
	other.Set("profiles.prod.oauth_token_url", "https://other.example.test/mcp/oauth/token")
	if err := other.WriteConfig(); err != nil {
		t.Fatal(err)
	}

	profile := expired
	if err := renewOAuthProfile(&profile, "access-old"); err == nil {
		t.Fatal("expected renewal to stop when the saved login changed")
	}
	if profile.AccessToken == "other-server-token" || server.refreshes != 0 {
		t.Fatalf("used or refreshed across connections: %q after %d refreshes", profile.AccessToken, server.refreshes)
	}
	reread := viper.New()
	reread.SetConfigFile(viper.ConfigFileUsed())
	reread.SetConfigType("toml")
	if err := reread.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if got := reread.GetString("profiles.prod.refresh_token"); got != "other-server-refresh" {
		t.Fatalf("overwrote the new login: %q", got)
	}
}

func TestAccountUseMovesAnInheritedOAuthLoginInsteadOfCopyingIt(t *testing.T) {
	useTempConfig(t, "")
	oldEnv, oldAccount, oldSelection := SelectedEnvironment, SelectedAccount, accountSelection
	t.Cleanup(func() { SelectedEnvironment, SelectedAccount, accountSelection = oldEnv, oldAccount, oldSelection })
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL"} {
		t.Setenv(name, "")
	}
	legacy := profileConfig{
		Name: "qa", BackendURL: "https://qa.example.test", AccessToken: "access-1", Client: api.OAuthClient, UID: "user-1",
		Expiry: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339), RefreshToken: "refresh-1",
		OAuthTokenURL: "https://qa.example.test/mcp/oauth/token", OAuthResource: "https://qa.example.test/mcp",
	}
	if err := saveProfile(legacy, false); err != nil {
		t.Fatal(err)
	}
	viper.Set("environments.qa.backend_url", "https://qa.example.test")

	accountSelection, SelectedEnvironment, SelectedAccount = true, "qa", "qa"
	inherited, err := resolveAccount()
	if err != nil || inherited.RefreshToken != "refresh-1" {
		t.Fatalf("inherit: %#v %v", inherited, err)
	}
	// Another process rotates the inherited login before `account use` saves.
	other := viper.New()
	other.SetConfigFile(viper.ConfigFileUsed())
	other.SetConfigType("toml")
	if err := other.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	other.Set("profiles.qa.refresh_token", "refresh-2")
	if err := other.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	if err := selectProfile(inherited); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString("accounts.qa.qa.refresh_token"); got != "refresh-2" {
		t.Fatalf("login not moved to the account with its newest token: %q", got)
	}
	if got := viper.GetString("profiles.qa.refresh_token"); got != "" {
		t.Fatalf("rotating refresh token left in two places: %q", got)
	}
	if info, err := os.Stat(viper.ConfigFileUsed()); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config must stay private: %v %v", info.Mode().Perm(), err)
	}
}

func TestDevicePollingDefaultsToFiveSeconds(t *testing.T) {
	if got := devicePollInterval(deviceAuthorization{}); got != 5*time.Second {
		t.Fatalf("missing interval should default to 5s, got %s", got)
	}
	if got := devicePollInterval(deviceAuthorization{Interval: 2}); got != 2*time.Second {
		t.Fatalf("server interval ignored: %s", got)
	}
}

func TestAccountUseRefusesALoginSavedForAnotherServer(t *testing.T) {
	useTempConfig(t, "")
	profile := profileConfig{Name: "prod", BackendURL: "https://a.example.test", AccessToken: "a", Client: api.OAuthClient, UID: "u"}
	if err := saveProfile(profile, false); err != nil {
		t.Fatal(err)
	}
	other := viper.New()
	other.SetConfigFile(viper.ConfigFileUsed())
	other.SetConfigType("toml")
	if err := other.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	other.Set("profiles.prod.backend_url", "https://b.example.test")
	other.Set("profiles.prod.access_token", "b-token")
	if err := other.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	if err := selectProfile(profile); err == nil {
		t.Fatal("expected selection to refuse credentials saved for another server")
	}
}

func TestOverlappingAccountUseKeepsTheMovedLogin(t *testing.T) {
	useTempConfig(t, "")
	oldEnv, oldAccount, oldSelection := SelectedEnvironment, SelectedAccount, accountSelection
	t.Cleanup(func() { SelectedEnvironment, SelectedAccount, accountSelection = oldEnv, oldAccount, oldSelection })
	for _, name := range []string{"TREECLI_ENV", "TREECLI_ACCOUNT", "TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL"} {
		t.Setenv(name, "")
	}
	legacy := profileConfig{
		Name: "qa", BackendURL: "https://qa.example.test", AccessToken: "access-1", Client: api.OAuthClient, UID: "user-1",
		RefreshToken: "refresh-1", OAuthTokenURL: "https://qa.example.test/mcp/oauth/token", OAuthResource: "https://qa.example.test/mcp",
	}
	if err := saveProfile(legacy, false); err != nil {
		t.Fatal(err)
	}
	viper.Set("environments.qa.backend_url", "https://qa.example.test")
	accountSelection, SelectedEnvironment, SelectedAccount = true, "qa", "qa"

	// Two invocations resolve the inherited login before either saves.
	first, _ := resolveAccount()
	second, _ := resolveAccount()
	if err := selectProfile(first); err != nil {
		t.Fatal(err)
	}
	if err := selectProfile(second); err != nil {
		t.Fatal(err)
	}
	onDisk := viper.New()
	onDisk.SetConfigFile(viper.ConfigFileUsed())
	onDisk.SetConfigType("toml")
	if err := onDisk.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	if got := onDisk.GetString("accounts.qa.qa.refresh_token"); got != "refresh-1" {
		t.Fatalf("second account use wiped the moved login: %q", got)
	}
}

func TestConnectDoesNotWaitOnABillingCheckThatFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	previous := connectBillingWait
	connectBillingWait = 3 * time.Minute
	t.Cleanup(func() { connectBillingWait = previous })

	var out bytes.Buffer
	ConnectCmd.SetOut(&out)
	t.Cleanup(func() { ConnectCmd.SetOut(nil) })
	started := time.Now()
	reportBillingAfterConnect(ConnectCmd, profileConfig{BackendURL: server.URL, AccessToken: "a", Client: api.OAuthClient, UID: "u"})
	if time.Since(started) > 5*time.Second || !bytes.Contains(out.Bytes(), []byte("Couldn't check USD billing")) {
		t.Fatalf("expected an immediate explanation, got %q after %s", out.String(), time.Since(started))
	}
}
