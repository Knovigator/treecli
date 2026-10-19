package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knovigator/treecli/api"
	treeclicontent "github.com/Knovigator/treecli/content"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The backend registers treecli as a public OAuth client allowed to use the
// device grant (RFC 8628).
const oauthClientID = "treecli"
const deviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// Refresh a little before the server's expiry so a command never starts with a
// token that dies mid-request.
const oauthRefreshSkew = time.Minute

var connectNoBrowser bool
var connectBillingWait time.Duration
var connectInstallSkills bool

var ConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect treecli to your Treechat account in the browser (sign up, sign in, card billing)",
	Long: "Connect treecli to a Treechat account without typing a password into the terminal.\n\n" +
		"treecli shows a short code and opens a Treechat page. There you sign in or create an account " +
		"(email, Google or X), check the code, allow access, and can add a card for pay-as-you-go AI " +
		"generation. treecli waits, then saves a connection that renews itself.",
	Example: "  treecli connect\n  treecli connect --no-browser\n  treecli --env staging --account bot connect",
	Args:    cobra.NoArgs,
	RunE:    runConnect,
}

func init() {
	ConnectCmd.Flags().BoolVar(&connectNoBrowser, "no-browser", false, "Print the link instead of opening a browser")
	ConnectCmd.Flags().DurationVar(&connectBillingWait, "billing-wait", 3*time.Minute, "How long to wait for card setup after access is allowed (0 to skip)")
	ConnectCmd.Flags().BoolVar(&connectInstallSkills, "install-skills", false, "Also install treecli skills for the coding agents found on this machine (Claude Code, Codex, Pi)")
}

type oauthServer struct {
	Resource                    string
	TokenEndpoint               string
	DeviceAuthorizationEndpoint string
}

type deviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

func runConnect(cmd *cobra.Command, args []string) error {
	profile, err := resolveProfile(resolveProfileName())
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	if err := validateCredentialTransport(profile.BackendURL); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	server, err := discoverOAuthServer(profile.BackendURL)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	device, err := startDeviceAuthorization(server)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	link := device.VerificationURIComplete
	if link == "" {
		link = device.VerificationURI
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Your code: %s\n", device.UserCode)
	fmt.Fprintf(out, "Open this link to sign in or sign up, then allow access:\n  %s\n", link)
	if !connectNoBrowser {
		if err := validateCredentialTransport(link); err == nil && openExternalURL(link) == nil {
			fmt.Fprintln(out, "Opened it in your browser.")
		}
	}
	fmt.Fprintln(out, "Waiting for you to allow access...")

	tokens, err := pollDeviceToken(server, device)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	clearOAuthCredentials(&profile)
	applyOAuthTokens(&profile, tokens)
	profile.Client = api.OAuthClient
	profile.OAuthTokenURL = server.TokenEndpoint
	profile.OAuthResource = server.Resource
	// UID must be non-empty for saved-credential checks; the verified id replaces it below.
	profile.UID = api.OAuthClient

	identity, err := fetchIdentity(profile)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	profile.UID = identity.UserID
	profile.CurrentUserID = identity.UserID
	profile.ActiveSpaceID = identity.SpaceID
	if err := saveProfile(profile, true); err != nil {
		return fmt.Errorf("connected but saving the account failed: %w", err)
	}
	fmt.Fprintf(out, "Connected as @%s.\n", identity.Username)

	reportBillingAfterConnect(cmd, profile)
	return offerAgentSkills(cmd)
}

// detectedSkillRoots lists skill folders for coding agents installed here.
func detectedSkillRoots() map[string]string {
	home := userHomeDir()
	if home == "" {
		return nil
	}
	candidates := map[string][2]string{
		"--claude": {filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "skills")},
		"--codex":  {filepath.Join(home, ".codex"), filepath.Join(home, ".codex", "skills")},
		"--pi":     {filepath.Join(home, ".pi", "agent"), filepath.Join(home, ".pi", "agent", "skills")},
	}
	found := map[string]string{}
	for flag, paths := range candidates {
		if info, err := os.Stat(paths[0]); err == nil && info.IsDir() {
			found[flag] = paths[1]
		}
	}
	return found
}

// offerAgentSkills teaches the coding agents found here how to use treecli:
// installs the packaged skills with --install-skills, or prints the command.
func offerAgentSkills(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	roots := detectedSkillRoots()
	if len(roots) == 0 {
		return nil
	}
	flags := make([]string, 0, len(roots))
	for flag := range roots {
		flags = append(flags, flag)
	}
	sort.Strings(flags)
	if !connectInstallSkills {
		fmt.Fprintf(out, "Teach your coding agent treecli: treecli skills install all %s\n", strings.Join(flags, " "))
		return nil
	}
	keys, err := resolvePackagedSkillKeys("all")
	if err != nil {
		return err
	}
	for _, flag := range flags {
		for _, key := range keys {
			skill, err := treeclicontent.GetPackagedSkill(key)
			if err != nil || skill == nil {
				return fmt.Errorf("packaged skill %q unavailable", key)
			}
			if _, err := treeclicontent.InstallPackagedSkill(*skill, roots[flag]); err != nil {
				return err
			}
		}
		fmt.Fprintf(out, "Installed treecli skills to %s\n", roots[flag])
	}
	return nil
}

// discoverOAuthServer follows the backend's protected-resource metadata to its
// authorization server, as MCP clients do.
func discoverOAuthServer(backendURL string) (oauthServer, error) {
	var resourceMeta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := getJSON(strings.TrimRight(backendURL, "/")+"/.well-known/oauth-protected-resource/mcp", &resourceMeta); err != nil {
		return oauthServer{}, fmt.Errorf("this server does not offer browser connect (%w); use treecli login", err)
	}
	if resourceMeta.Resource == "" || len(resourceMeta.AuthorizationServers) == 0 {
		return oauthServer{}, fmt.Errorf("server metadata is incomplete; use treecli login")
	}
	issuer := strings.TrimRight(resourceMeta.AuthorizationServers[0], "/")
	if err := validateCredentialTransport(issuer); err != nil {
		return oauthServer{}, err
	}

	var serverMeta struct {
		TokenEndpoint               string `json:"token_endpoint"`
		DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	}
	if err := getJSON(issuer+"/.well-known/oauth-authorization-server", &serverMeta); err != nil {
		return oauthServer{}, err
	}
	if serverMeta.DeviceAuthorizationEndpoint == "" {
		return oauthServer{}, fmt.Errorf("this server does not support browser connect yet; use treecli login")
	}
	for _, endpoint := range []string{serverMeta.TokenEndpoint, serverMeta.DeviceAuthorizationEndpoint} {
		if err := validateCredentialTransport(endpoint); err != nil {
			return oauthServer{}, err
		}
	}
	return oauthServer{
		Resource:                    resourceMeta.Resource,
		TokenEndpoint:               serverMeta.TokenEndpoint,
		DeviceAuthorizationEndpoint: serverMeta.DeviceAuthorizationEndpoint,
	}, nil
}

func startDeviceAuthorization(server oauthServer) (deviceAuthorization, error) {
	var device deviceAuthorization
	response, err := oauthHTTP().R().
		SetFormData(map[string]string{"client_id": oauthClientID, "resource": server.Resource}).
		SetResult(&device).
		Post(server.DeviceAuthorizationEndpoint)
	if err != nil {
		return device, fmt.Errorf("starting connect: %w", err)
	}
	if response.StatusCode() != http.StatusOK || device.DeviceCode == "" || device.UserCode == "" {
		return device, fmt.Errorf("starting connect failed (status %d): %s", response.StatusCode(), api.SafeResponseBody(response.Body()))
	}
	return device, nil
}

// pollDeviceToken waits for the person to approve in the browser (RFC 8628 3.4-3.5).
func pollDeviceToken(server oauthServer, device deviceAuthorization) (oauthTokenResponse, error) {
	interval := devicePollInterval(device)
	deadline := time.Now().Add(time.Duration(max(device.ExpiresIn, 60)) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		tokens, status, err := requestToken(server.TokenEndpoint, map[string]string{
			"grant_type":  deviceCodeGrantType,
			"device_code": device.DeviceCode,
			"client_id":   oauthClientID,
			"resource":    server.Resource,
		})
		if err != nil {
			return tokens, err
		}
		switch {
		case status == http.StatusOK && tokens.AccessToken != "":
			return tokens, nil
		case tokens.Error == "authorization_pending":
		case tokens.Error == "slow_down":
			interval += 5 * time.Second
		case tokens.Error == "access_denied":
			return tokens, fmt.Errorf("access was denied in the browser")
		case tokens.Error == "expired_token":
			return tokens, fmt.Errorf("the code expired; run treecli connect again")
		default:
			return tokens, fmt.Errorf("unexpected response from the server (status %d, error %q)", status, tokens.Error)
		}
	}
	return oauthTokenResponse{}, fmt.Errorf("the code expired; run treecli connect again")
}

// RFC 8628 section 3.2: poll every 5 seconds unless the server says otherwise.
func devicePollInterval(device deviceAuthorization) time.Duration {
	if device.Interval > 0 {
		return time.Duration(device.Interval) * time.Second
	}
	return 5 * time.Second
}

func requestToken(tokenURL string, form map[string]string) (oauthTokenResponse, int, error) {
	var tokens oauthTokenResponse
	response, err := oauthHTTP().R().SetFormData(form).Post(tokenURL)
	if err != nil {
		return tokens, 0, fmt.Errorf("contacting the server: %w", err)
	}
	if err := json.Unmarshal(response.Body(), &tokens); err != nil {
		return tokens, response.StatusCode(), fmt.Errorf("unexpected response from the server (status %d)", response.StatusCode())
	}
	return tokens, response.StatusCode(), nil
}

func applyOAuthTokens(profile *profileConfig, tokens oauthTokenResponse) {
	profile.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		profile.RefreshToken = tokens.RefreshToken
	}
	profile.Expiry = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
}

// clearOAuthCredentials drops connect-only fields when a password login or
// signup replaces the account's credentials.
func clearOAuthCredentials(profile *profileConfig) {
	profile.RefreshToken = ""
	profile.OAuthTokenURL = ""
	profile.OAuthResource = ""
}

// prepareOAuthProfile renews an expiring bearer before a command runs and lets
// the API client renew it again if it expires mid-command. The API client
// serializes calls to the hook.
func prepareOAuthProfile(profile *profileConfig) error {
	current := *profile
	api.SetOAuthRefresher(func(stale string) (string, error) {
		// Another call in this process may already have renewed it; reuse that
		// only while it is still fresh (long commands can outlive it too).
		if stale != current.AccessToken && oauthTokenFresh(current) {
			return current.AccessToken, nil
		}
		if err := renewOAuthProfile(&current, stale); err != nil {
			return "", err
		}
		return current.AccessToken, nil
	})

	if oauthTokenFresh(*profile) {
		return nil
	}
	if err := renewOAuthProfile(&current, profile.AccessToken); err != nil {
		return err
	}
	*profile = current
	return nil
}

func oauthTokenFresh(profile profileConfig) bool {
	expiry, err := time.Parse(time.RFC3339, profile.Expiry)
	return err == nil && time.Until(expiry) > oauthRefreshSkew
}

// renewOAuthProfile first adopts tokens another treecli process may have saved
// (refresh tokens rotate, so only the newest one works), and refreshes only if
// those are stale too.
func renewOAuthProfile(profile *profileConfig, stale string) error {
	return withConfigLock(func() error {
		if err := reloadSavedOAuthTokens(profile); err != nil {
			return err
		}
		if profile.AccessToken != stale && oauthTokenFresh(*profile) {
			return nil
		}
		return refreshOAuthProfile(profile)
	})
}

// reloadSavedOAuthTokens reads the config file afresh (a separate reader, since
// values this process already set would mask the file in the global one).
// It fails if the saved section now holds a different login (reconnected to
// another server or account, or replaced by a password login): renewing the
// old one would overwrite the new login and could mix their credentials.
func reloadSavedOAuthTokens(profile *profileConfig) error {
	configPath, err := configFilePath()
	if err != nil {
		return nil
	}
	saved := viper.New()
	saved.SetConfigFile(configPath)
	saved.SetConfigType("toml")
	if saved.ReadInConfig() != nil {
		return nil
	}
	prefix := credentialKeyPrefix(*profile)
	if !saved.IsSet(prefix + "client") {
		return nil
	}
	if strings.TrimSpace(saved.GetString(prefix+"client")) != api.OAuthClient ||
		strings.TrimSpace(saved.GetString(prefix+"oauth_token_url")) != profile.OAuthTokenURL ||
		strings.TrimSpace(saved.GetString(prefix+"oauth_resource")) != profile.OAuthResource ||
		strings.TrimSpace(saved.GetString(prefix+"uid")) != profile.UID {
		return fmt.Errorf("this account's Treechat login changed in another treecli; run the command again")
	}
	if refresh := strings.TrimSpace(saved.GetString(prefix + "refresh_token")); refresh != "" {
		profile.AccessToken = strings.TrimSpace(saved.GetString(prefix + "access_token"))
		profile.RefreshToken = refresh
		profile.Expiry = strings.TrimSpace(saved.GetString(prefix + "expiry"))
	}
	return nil
}

func refreshOAuthProfile(profile *profileConfig) error {
	if profile.RefreshToken == "" || profile.OAuthTokenURL == "" {
		return fmt.Errorf("Treechat connection expired; run treecli connect again")
	}
	if err := validateCredentialTransport(profile.OAuthTokenURL); err != nil {
		return err
	}
	tokens, status, err := requestToken(profile.OAuthTokenURL, map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": profile.RefreshToken,
		"client_id":     oauthClientID,
		"resource":      profile.OAuthResource,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK || tokens.AccessToken == "" {
		return fmt.Errorf("Treechat connection expired or was revoked; run treecli connect again")
	}
	applyOAuthTokens(profile, tokens)
	if err := saveOAuthTokens(*profile); err != nil {
		return fmt.Errorf("renewed the connection but saving it failed: %w", err)
	}
	return nil
}

// saveOAuthTokens writes only the renewed token fields, so per-command overrides
// such as --backend-url never become saved settings.
// Callers hold withConfigLock.
func saveOAuthTokens(profile profileConfig) error {
	prefix := credentialKeyPrefix(profile)
	return saveConfigFields(map[string]interface{}{
		prefix + "access_token":  profile.AccessToken,
		prefix + "refresh_token": profile.RefreshToken,
		prefix + "expiry":        profile.Expiry,
	})
}

const configLockStale = 30 * time.Second

// withConfigLock serializes token renewal across treecli processes with a lock
// file next to the config (portable, unlike flock). A lock older than
// configLockStale is treated as abandoned by a crashed process.
func withConfigLock(fn func() error) error {
	configPath, err := configFilePath()
	if err != nil {
		return fn()
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	lockPath := configPath + ".lock"
	deadline := time.Now().Add(20 * time.Second)
	for {
		lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			lock.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		if info, statErr := os.Stat(lockPath); statErr == nil && time.Since(info.ModTime()) > configLockStale {
			os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("another treecli is renewing the Treechat connection; try again")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// reportBillingAfterConnect waits briefly for card setup started in the browser,
// so an agent can generate right after connect returns.
func reportBillingAfterConnect(cmd *cobra.Command, profile profileConfig) {
	out := cmd.OutOrStdout()
	active := func(refresh bool) (bool, error) {
		status, err := api.GetStripeAiBillingStatus(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, refresh)
		return err == nil && (status.Stripe.SubscriptionStatus == "active" || status.Stripe.SubscriptionStatus == "trialing"), err
	}
	on, err := active(true)
	if err != nil {
		// Don't wait on a check that can't succeed; the connection itself is saved.
		fmt.Fprintf(out, "Couldn't check card billing (%v). Run `treecli billing status` to see it.\n", err)
		return
	}
	if on {
		fmt.Fprintln(out, "Card billing is on: AI generations are charged pay as you go.")
		return
	}
	if connectBillingWait > 0 {
		fmt.Fprintf(out, "If you're adding a card in the browser, finish there. Waiting up to %s (Ctrl-C to stop)...\n", connectBillingWait)
		deadline := time.Now().Add(connectBillingWait)
		for time.Now().Before(deadline) {
			time.Sleep(5 * time.Second)
			if on, _ := active(false); on {
				fmt.Fprintln(out, "Card billing is on: AI generations are charged pay as you go.")
				return
			}
		}
	}
	fmt.Fprintln(out, "Card billing isn't set up. Add it any time with `treecli billing checkout`.")
}

func oauthHTTP() *resty.Client {
	client := resty.New().SetTimeout(15 * time.Second)
	// Never follow redirects with credentials or device codes in the request.
	client.SetRedirectPolicy(resty.NoRedirectPolicy())
	return client.SetHeader("Accept", "application/json")
}

func getJSON(rawURL string, out interface{}) error {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return err
	}
	response, err := oauthHTTP().R().Get(rawURL)
	if err != nil {
		return err
	}
	if response.StatusCode() != http.StatusOK {
		return fmt.Errorf("status %d", response.StatusCode())
	}
	return json.Unmarshal(response.Body(), out)
}
