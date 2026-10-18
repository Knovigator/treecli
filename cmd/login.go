package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/Knovigator/treecli/api"
	"github.com/adrg/xdg"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var SelectedProfile string
var BackendURLOverride string
var AppHostOverride string

var loginEmail string
var loginPassword string
var readPasswordFromStdin bool
var profileShowJSON bool

var loginEndpointCandidates = []string{"/auth/sign_in", "/auth/signin"}
var builtInProfileOrder = []string{"dev", "staging", "prod"}

const profileRedactedValue = "********"

type authTokens struct {
	AccessToken string
	Client      string
	UID         string
	Expiry      string
}

type profileConfig struct {
	Environment   string `json:"environment,omitempty"`
	Account       string `json:"account,omitempty"`
	Name          string `json:"name"`
	BackendURL    string `json:"backend_url"`
	AppHost       string `json:"app_host"`
	AccessToken   string `json:"access_token,omitempty"`
	Client        string `json:"client,omitempty"`
	UID           string `json:"uid,omitempty"`
	Expiry        string `json:"expiry,omitempty"`
	CurrentUserID string `json:"current_user_id,omitempty"`
	ActiveSpaceID string `json:"active_space_id,omitempty"`
	// Set by `treecli connect` (Client == api.OAuthClient); Expiry is then RFC 3339.
	RefreshToken  string `json:"refresh_token,omitempty"`
	OAuthTokenURL string `json:"oauth_token_url,omitempty"`
	OAuthResource string `json:"oauth_resource,omitempty"`
	// Where the saved credentials were read from, when that differs from the
	// profile's own section (an account inheriting a legacy profile's login).
	credentialKeyPrefix string
}

type loginBootstrap struct {
	Host        string `json:"host"`
	CurrentUser *struct {
		ID      string `json:"id"`
		SpaceID string `json:"space_id"`
	} `json:"currentUser"`
}

// LoginCmd represents the login command
var LoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your account",
	Long:  `Authenticate and log in to your tree account to access protected features.`,
	RunE:  runLogin,
}

var ProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Deprecated: inspect legacy profiles; use account commands",
	Long:  `List, inspect, and switch the saved treecli profiles.`,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available profiles",
	RunE:  runProfileList,
}

var profileUseCmd = &cobra.Command{
	Use:   "use [profile]",
	Short: "Select the active profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileUse,
}

var profileShowCmd = &cobra.Command{
	Use:   "show [profile]",
	Short: "Show the resolved profile configuration",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProfileShow,
}

func init() {
	LoginCmd.Flags().StringVarP(&loginEmail, "email", "e", "", "Email address for login")
	LoginCmd.Flags().StringVarP(&loginPassword, "password", "p", "", "Password for login")
	LoginCmd.Flags().BoolVar(&readPasswordFromStdin, "password-stdin", false, "Read password from stdin")
	_ = LoginCmd.Flags().MarkDeprecated("password", "use the interactive prompt or --password-stdin; command-line passwords can be exposed via shell history and process listings")

	ProfileCmd.AddCommand(profileListCmd)
	ProfileCmd.AddCommand(profileUseCmd)
	ProfileCmd.AddCommand(profileShowCmd)
	profileShowCmd.Flags().BoolVar(&profileShowJSON, "json", false, "Output JSON instead of human-readable text")
}

func runLogin(cmd *cobra.Command, args []string) error {
	profileName := resolveProfileName()
	profile, err := resolveProfile(profileName)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	if err := validateCredentialTransport(profile.BackendURL); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	email, err := resolveEmail()
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	password, err := resolvePassword()
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	tokens, err := performLogin(profile.BackendURL, email, password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	bootstrap, err := fetchBootstrap(profile.BackendURL, tokens)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	profile.AccessToken = tokens.AccessToken
	profile.Client = tokens.Client
	profile.UID = tokens.UID
	profile.Expiry = tokens.Expiry
	clearOAuthCredentials(&profile)
	profile.AppHost = normalizeAppHost(bootstrap.Host)
	profile.CurrentUserID = bootstrap.CurrentUser.ID
	profile.ActiveSpaceID = bootstrap.CurrentUser.SpaceID

	err = saveProfile(profile, true)
	if err != nil {
		return fmt.Errorf("saving profile: %w", err)
	}

	if profile.Environment != "" {
		fmt.Printf("Login successful. Environment: %s Account: %s Backend: %s\n", profile.Environment, profile.Account, profile.BackendURL)
	} else {
		fmt.Printf("Login successful. Profile: %s Backend: %s\n", profile.Name, profile.BackendURL)
	}
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	activeProfile := resolveProfileName()
	profileNames := allProfileNames()

	for _, profileName := range profileNames {
		profile, err := resolveProfile(profileName)
		if err != nil {
			fmt.Printf("  %s (invalid: %v)\n", profileName, err)
			continue
		}

		marker := " "
		if profileName == activeProfile {
			marker = "*"
		}

		loginState := "signed-out"
		if profile.AccessToken != "" && profile.Client != "" && profile.UID != "" {
			loginState = "signed-in"
		}

		fmt.Printf("%s %s\t%s\t%s\n", marker, profile.Name, profile.BackendURL, loginState)
	}

	return nil
}

func runProfileUse(cmd *cobra.Command, args []string) error {
	profileName := normalizeProfileName(args[0])
	if _, err := resolveProfile(profileName); err != nil {
		return err
	}

	err := setActiveProfile(profileName)
	if err != nil {
		return err
	}

	fmt.Printf("Active profile set to %s\n", profileName)
	return nil
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	profileName := resolveProfileName()
	if len(args) == 1 {
		profileName = normalizeProfileName(args[0])
	}

	profile, err := resolveProfile(profileName)
	if err != nil {
		return err
	}

	redactedProfile := redactProfile(profile)

	if profileShowJSON {
		prettyJSON, err := json.MarshalIndent(redactedProfile, "", "  ")
		if err != nil {
			return fmt.Errorf("formatting profile: %w", err)
		}

		fmt.Println(string(prettyJSON))
		return nil
	}

	loginState := "signed-out"
	if redactedProfile.AccessToken != "" && redactedProfile.Client != "" && redactedProfile.UID != "" {
		loginState = "signed-in"
	}

	fmt.Printf("Profile: %s\n", redactedProfile.Name)
	fmt.Printf("Backend: %s\n", redactedProfile.BackendURL)
	if redactedProfile.AppHost != "" {
		fmt.Printf("App host: %s\n", redactedProfile.AppHost)
	}
	fmt.Printf("Login: %s\n", loginState)
	if redactedProfile.UID != "" {
		fmt.Printf("UID: %s\n", redactedProfile.UID)
	}
	if redactedProfile.CurrentUserID != "" {
		fmt.Printf("Current user id: %s\n", redactedProfile.CurrentUserID)
	}
	if redactedProfile.ActiveSpaceID != "" {
		fmt.Printf("Active space id: %s\n", redactedProfile.ActiveSpaceID)
	}
	if redactedProfile.AccessToken != "" {
		fmt.Printf("Access token: %s\n", redactedProfile.AccessToken)
	}
	if redactedProfile.Client != "" {
		fmt.Printf("Client: %s\n", redactedProfile.Client)
	}
	if redactedProfile.Expiry != "" {
		fmt.Printf("Expiry: %s\n", redactedProfile.Expiry)
	}

	return nil
}

func builtInProfiles() map[string]profileConfig {
	return map[string]profileConfig{
		"dev": {
			Name:       "dev",
			BackendURL: "http://localhost:5001",
			AppHost:    "http://localhost:5173",
		},
		"staging": {
			Name:       "staging",
			BackendURL: "https://knov-staging-jajw.onrender.com",
			AppHost:    "https://staging-frontend-vi5w.onrender.com",
		},
		"prod": {
			Name:       "prod",
			BackendURL: "https://knov-prod.onrender.com",
			AppHost:    "https://prod-frontend-kitu.onrender.com",
		},
	}
}

func resolveProfileName() string {
	if strings.TrimSpace(SelectedProfile) != "" {
		return normalizeProfileName(SelectedProfile)
	}

	if profileName := firstEnv("TREECLI_PROFILE", "TREECTL_PROFILE"); profileName != "" {
		return normalizeProfileName(profileName)
	}

	if profileName := strings.TrimSpace(viper.GetString("active_profile")); profileName != "" {
		return normalizeProfileName(profileName)
	}

	return "prod"
}

func normalizeProfileName(profileName string) string {
	return strings.ToLower(strings.TrimSpace(profileName))
}

func firstEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func resolveProfile(profileName string) (profileConfig, error) {
	if accountSelection {
		return resolveAccount()
	}
	return resolveLegacyProfile(profileName)
}

func resolveLegacyProfile(profileName string) (profileConfig, error) {
	resolvedProfileName := normalizeProfileName(profileName)
	profile := builtInProfiles()[resolvedProfileName]
	profile.Name = resolvedProfileName

	storedProfile := loadStoredProfile(resolvedProfileName)
	profile = mergeProfile(profile, storedProfile)

	backendURLOverride := strings.TrimSpace(BackendURLOverride)
	if backendURLOverride == "" {
		backendURLOverride = firstEnv("TREECLI_BACKEND_URL", "TREECTL_BACKEND_URL")
	}
	if backendURLOverride != "" {
		profile.BackendURL = normalizeBaseURL(backendURLOverride)
	}

	appHostOverride := strings.TrimSpace(AppHostOverride)
	if appHostOverride == "" {
		appHostOverride = firstEnv("TREECLI_APP_HOST", "TREECTL_APP_HOST")
	}
	if appHostOverride != "" {
		profile.AppHost = normalizeAppHost(appHostOverride)
	}

	if profile.BackendURL == "" {
		return profileConfig{}, fmt.Errorf("profile %q does not define a backend_url", resolvedProfileName)
	}

	profile.BackendURL = normalizeBaseURL(profile.BackendURL)
	profile.AppHost = normalizeAppHost(profile.AppHost)

	return profile, nil
}

func loadStoredProfile(profileName string) profileConfig {
	profileKeyPrefix := fmt.Sprintf("profiles.%s.", normalizeProfileName(profileName))

	return profileConfig{
		Name:          normalizeProfileName(profileName),
		BackendURL:    strings.TrimSpace(viper.GetString(profileKeyPrefix + "backend_url")),
		AppHost:       strings.TrimSpace(viper.GetString(profileKeyPrefix + "app_host")),
		AccessToken:   strings.TrimSpace(viper.GetString(profileKeyPrefix + "access_token")),
		Client:        strings.TrimSpace(viper.GetString(profileKeyPrefix + "client")),
		UID:           strings.TrimSpace(viper.GetString(profileKeyPrefix + "uid")),
		Expiry:        strings.TrimSpace(viper.GetString(profileKeyPrefix + "expiry")),
		CurrentUserID: strings.TrimSpace(viper.GetString(profileKeyPrefix + "current_user_id")),
		ActiveSpaceID: strings.TrimSpace(viper.GetString(profileKeyPrefix + "active_space_id")),
		RefreshToken:  strings.TrimSpace(viper.GetString(profileKeyPrefix + "refresh_token")),
		OAuthTokenURL: strings.TrimSpace(viper.GetString(profileKeyPrefix + "oauth_token_url")),
		OAuthResource: strings.TrimSpace(viper.GetString(profileKeyPrefix + "oauth_resource")),
	}
}

func mergeProfile(base profileConfig, override profileConfig) profileConfig {
	if override.Name != "" {
		base.Name = override.Name
	}
	if override.BackendURL != "" {
		base.BackendURL = override.BackendURL
	}
	if override.AppHost != "" {
		base.AppHost = override.AppHost
	}
	if override.AccessToken != "" {
		base.AccessToken = override.AccessToken
	}
	if override.Client != "" {
		base.Client = override.Client
	}
	if override.UID != "" {
		base.UID = override.UID
	}
	if override.Expiry != "" {
		base.Expiry = override.Expiry
	}
	if override.CurrentUserID != "" {
		base.CurrentUserID = override.CurrentUserID
	}
	if override.ActiveSpaceID != "" {
		base.ActiveSpaceID = override.ActiveSpaceID
	}
	if override.RefreshToken != "" {
		base.RefreshToken = override.RefreshToken
	}
	if override.OAuthTokenURL != "" {
		base.OAuthTokenURL = override.OAuthTokenURL
	}
	if override.OAuthResource != "" {
		base.OAuthResource = override.OAuthResource
	}
	if override.credentialKeyPrefix != "" {
		base.credentialKeyPrefix = override.credentialKeyPrefix
	}

	return base
}

func allProfileNames() []string {
	profileNames := append([]string{}, builtInProfileOrder...)
	profileMap := viper.GetStringMap("profiles")
	for profileName := range profileMap {
		normalizedProfileName := normalizeProfileName(profileName)
		if normalizedProfileName == "" || slices.Contains(profileNames, normalizedProfileName) {
			continue
		}

		profileNames = append(profileNames, normalizedProfileName)
	}

	slices.Sort(profileNames)
	return profileNames
}

func requireAuthenticatedProfile() (profileConfig, error) {
	profile, err := resolveProfile(resolveProfileName())
	if err != nil {
		return profileConfig{}, err
	}

	if profile.AccessToken == "" || profile.Client == "" || profile.UID == "" {
		if profile.Environment != "" {
			return profileConfig{}, fmt.Errorf("missing credentials for account %q in environment %q; run treecli --env %s --account %s connect (or login)", profile.Account, profile.Environment, profile.Environment, profile.Account)
		}
		return profileConfig{}, fmt.Errorf("missing credentials for profile %q; run treecli connect --profile %s (or login)", profile.Name, profile.Name)
	}
	if err := validateCredentialTransport(profile.BackendURL); err != nil {
		return profileConfig{}, err
	}
	if profile.Client == api.OAuthClient {
		if err := prepareOAuthProfile(&profile); err != nil {
			return profileConfig{}, err
		}
	}

	return profile, nil
}

func setActiveProfile(profileName string) error {
	profile, err := resolveProfile(profileName)
	if err != nil {
		return err
	}

	return selectProfile(profile)
}

func resolveEmail() (string, error) {
	if strings.TrimSpace(loginEmail) != "" {
		return strings.TrimSpace(loginEmail), nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter email: ")
	email, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading email: %w", err)
	}

	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("email cannot be empty")
	}

	return email, nil
}

func readHiddenPassword(prompt string) (string, error) {
	if !term.IsTerminal(int(syscall.Stdin)) {
		return "", fmt.Errorf("stdin is not a terminal; use --password-stdin for non-interactive input")
	}

	fmt.Print(prompt)
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	return strings.TrimSpace(string(passwordBytes)), nil
}

func resolvePassword() (string, error) {
	if readPasswordFromStdin {
		passwordBytes, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("error reading password from stdin: %w", err)
		}

		password := strings.TrimSpace(string(passwordBytes))
		if password == "" {
			return "", fmt.Errorf("password cannot be empty")
		}

		return password, nil
	}

	if loginPassword != "" {
		return loginPassword, nil
	}

	password, err := readHiddenPassword("Enter password: ")
	if err != nil {
		return "", err
	}

	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	return password, nil
}

func performLogin(backendURL, email, password string) (authTokens, error) {
	api.MarkNetwork()
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	formBody := fmt.Sprintf("email=%s&password=%s", url.QueryEscape(email), url.QueryEscape(password))
	var lastError error

	for _, endpoint := range loginEndpointCandidates {
		resp, err := client.R().
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			SetBody(formBody).
			Post(backendURL + endpoint)

		if err != nil {
			lastError = fmt.Errorf("error making request to %s: %w", endpoint, err)
			continue
		}

		if resp.StatusCode() == http.StatusNotFound {
			lastError = fmt.Errorf("%s returned 404", endpoint)
			continue
		}

		if resp.StatusCode() != http.StatusOK {
			return authTokens{}, fmt.Errorf("login failed: %s", formatResponseError(resp))
		}

		return authTokensFromResponse(resp)
	}

	if lastError != nil {
		return authTokens{}, lastError
	}

	return authTokens{}, fmt.Errorf("login failed: no auth endpoint succeeded")
}

func authTokensFromResponse(resp *resty.Response) (authTokens, error) {
	tokens := authTokens{
		AccessToken: strings.TrimSpace(resp.Header().Get("access-token")),
		Client:      strings.TrimSpace(resp.Header().Get("client")),
		UID:         strings.TrimSpace(resp.Header().Get("uid")),
		Expiry:      strings.TrimSpace(resp.Header().Get("expiry")),
	}

	if tokens.AccessToken == "" {
		return authTokens{}, fmt.Errorf("access token not found in response headers")
	}
	if tokens.Client == "" {
		return authTokens{}, fmt.Errorf("client not found in response headers")
	}
	if tokens.UID == "" {
		return authTokens{}, fmt.Errorf("uid not found in response headers")
	}

	return tokens, nil
}

func fetchBootstrap(backendURL string, tokens authTokens) (*loginBootstrap, error) {
	api.MarkNetwork()
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	resp, err := client.R().
		SetHeader("access-token", tokens.AccessToken).
		SetHeader("client", tokens.Client).
		SetHeader("uid", tokens.UID).
		Get(backendURL + "/api/v1/gon")
	if err != nil {
		return nil, fmt.Errorf("error fetching bootstrap: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("bootstrap fetch failed: %s", formatResponseError(resp))
	}

	var bootstrap loginBootstrap
	if err := json.Unmarshal(resp.Body(), &bootstrap); err != nil {
		return nil, fmt.Errorf("error decoding bootstrap payload: %w", err)
	}

	if bootstrap.CurrentUser == nil || bootstrap.CurrentUser.ID == "" {
		return nil, fmt.Errorf("bootstrap payload did not include an authenticated currentUser")
	}

	return &bootstrap, nil
}

// saveProfile saves new credentials (login, signup, connect) for a profile.
func saveProfile(profile profileConfig, setActive bool) error {
	return saveProfileLocked(profile, setActive, false)
}

// selectProfile makes an already-saved profile active. It re-reads the saved
// credentials under the lock first, so a refresh token another treecli just
// rotated is never written back with the stale value this process resolved.
func selectProfile(profile profileConfig) error {
	return saveProfileLocked(profile, true, true)
}

func saveProfileLocked(profile profileConfig, setActive, reloadCredentials bool) error {
	configPath, err := configFilePath()
	if err != nil {
		return err
	}
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	return withConfigLock(func() error {
		if reloadCredentials {
			if err := reloadSavedCredentials(&profile, configPath); err != nil {
				return err
			}
		}
		return saveConfigFields(profileFields(profile, setActive))
	})
}

func reloadSavedCredentials(profile *profileConfig, configPath string) error {
	saved := viper.New()
	saved.SetConfigFile(configPath)
	saved.SetConfigType("toml")
	prefix := credentialKeyPrefix(*profile)
	if saved.ReadInConfig() != nil || !saved.IsSet(prefix+"client") {
		return nil
	}
	if strings.TrimSpace(saved.GetString(prefix+"client")) == "" {
		// An inherited login another `account use` already moved: read it from
		// this account's own section instead of the emptied source.
		own := storedProfileKeyPrefix(*profile)
		if own == prefix || strings.TrimSpace(saved.GetString(own+"client")) == "" {
			return fmt.Errorf("this account has no saved login anymore; log in or connect again")
		}
		prefix, profile.credentialKeyPrefix = own, ""
	}
	get := func(key string) string { return strings.TrimSpace(saved.GetString(prefix + key)) }
	// Credentials saved for another server must never be paired with this one.
	if backend := get("backend_url"); backend != "" && normalizeBaseURL(backend) != normalizeBaseURL(profile.BackendURL) {
		return fmt.Errorf("this account's login changed in another treecli; run the command again")
	}
	profile.AccessToken, profile.Client, profile.UID, profile.Expiry = get("access_token"), get("client"), get("uid"), get("expiry")
	profile.RefreshToken, profile.OAuthTokenURL, profile.OAuthResource = get("refresh_token"), get("oauth_token_url"), get("oauth_resource")
	return nil
}

func profileFields(profile profileConfig, setActive bool) map[string]interface{} {
	fields := map[string]interface{}{}
	profileKeyPrefix := storedProfileKeyPrefix(profile)
	if profile.Environment != "" {
		fields["environments."+profile.Environment+".backend_url"] = profile.BackendURL
		fields["environments."+profile.Environment+".app_host"] = profile.AppHost
		if setActive {
			fields["environments."+profile.Environment+".active_account"] = profile.Account
		}
	}
	fields[profileKeyPrefix+"backend_url"] = normalizeBaseURL(profile.BackendURL)
	fields[profileKeyPrefix+"app_host"] = normalizeAppHost(profile.AppHost)
	fields[profileKeyPrefix+"access_token"] = profile.AccessToken
	fields[profileKeyPrefix+"client"] = profile.Client
	fields[profileKeyPrefix+"uid"] = profile.UID
	fields[profileKeyPrefix+"expiry"] = profile.Expiry
	fields[profileKeyPrefix+"current_user_id"] = profile.CurrentUserID
	fields[profileKeyPrefix+"active_space_id"] = profile.ActiveSpaceID
	fields[profileKeyPrefix+"refresh_token"] = profile.RefreshToken
	fields[profileKeyPrefix+"oauth_token_url"] = profile.OAuthTokenURL
	fields[profileKeyPrefix+"oauth_resource"] = profile.OAuthResource

	if setActive && profile.Environment == "" {
		fields["active_profile"] = normalizeProfileName(profile.Name)
	}

	// An OAuth login inherited from another section moves here rather than being
	// copied: refresh tokens rotate, so two copies would each break the other.
	if source := profile.credentialKeyPrefix; profile.Client == api.OAuthClient && source != "" && source != profileKeyPrefix {
		for _, key := range []string{"access_token", "refresh_token", "client", "expiry"} {
			fields[source+key] = ""
		}
	}
	return fields
}

// saveConfigFields sets keys in the config file as it is on disk now, not in
// this process's startup copy (which may hold tokens another treecli process
// has since rotated), mirrors them into this process, and replaces the file
// atomically with owner-only permissions. Callers hold withConfigLock.
func saveConfigFields(fields map[string]interface{}) error {
	configPath, err := configFilePath()
	if err != nil {
		return err
	}
	onDisk := viper.New()
	onDisk.SetConfigFile(configPath)
	onDisk.SetConfigType("toml")
	// The temp file holds credentials from the moment it exists.
	onDisk.SetConfigPermissions(0600)
	if err := onDisk.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !os.IsNotExist(err) && !errors.As(err, &notFound) {
			return err
		}
	}
	for key, value := range fields {
		onDisk.Set(key, value)
		viper.Set(key, value)
	}

	// Write through a symlinked config (e.g. dotfiles) instead of replacing the link.
	if resolved, err := filepath.EvalSymlinks(configPath); err == nil {
		configPath = resolved
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(configPath), fmt.Sprintf(".config-%d.tmp.toml", os.Getpid()))
	os.Remove(temp) // an existing file would keep its old permissions
	if err := onDisk.WriteConfigAs(temp); err != nil {
		os.Remove(temp)
		return err
	}
	if err := os.Chmod(temp, 0600); err != nil {
		os.Remove(temp)
		return err
	}
	return os.Rename(temp, configPath)
}

// storedProfileKeyPrefix is where a profile's credentials live in the config.
func storedProfileKeyPrefix(profile profileConfig) string {
	if profile.Environment != "" {
		return "accounts." + profile.Environment + "." + profile.Account + "."
	}
	return fmt.Sprintf("profiles.%s.", normalizeProfileName(profile.Name))
}

// credentialKeyPrefix is where the credentials a profile was resolved with are
// saved, so renewed tokens go back to the same place they are read from.
func credentialKeyPrefix(profile profileConfig) string {
	if profile.credentialKeyPrefix != "" {
		return profile.credentialKeyPrefix
	}
	return storedProfileKeyPrefix(profile)
}

func configFilePath() (string, error) {
	configPath := viper.ConfigFileUsed()
	if configPath != "" {
		return configPath, nil
	}

	return xdg.ConfigFile("treecli/config.toml")
}

func normalizeBaseURL(rawURL string) string {
	return strings.TrimRight(strings.TrimSpace(rawURL), "/")
}

func normalizeAppHost(rawHost string) string {
	trimmedHost := strings.TrimSpace(rawHost)
	if trimmedHost == "" {
		return ""
	}

	if strings.HasPrefix(trimmedHost, "http://") || strings.HasPrefix(trimmedHost, "https://") {
		return strings.TrimRight(trimmedHost, "/")
	}

	scheme := "https"
	if strings.Contains(trimmedHost, "localhost") || strings.Contains(trimmedHost, "127.0.0.1") {
		scheme = "http"
	}

	return fmt.Sprintf("%s://%s", scheme, strings.TrimRight(trimmedHost, "/"))
}

func validateCredentialTransport(backendURL string) error {
	parsedURL, err := url.Parse(strings.TrimSpace(backendURL))
	if err != nil {
		return fmt.Errorf("invalid backend_url %q: %w", backendURL, err)
	}
	if parsedURL.Scheme == "https" {
		return nil
	}
	if parsedURL.Scheme == "http" && isLoopbackHost(parsedURL.Hostname()) {
		return nil
	}
	if parsedURL.Scheme == "http" && strings.EqualFold(firstEnv("TREECLI_ALLOW_INSECURE_HTTP", "TREECTL_ALLOW_INSECURE_HTTP"), "1") {
		return nil
	}
	if parsedURL.Scheme == "http" {
		return fmt.Errorf("refusing to send credentials to insecure backend_url %q; use https, localhost, or set TREECLI_ALLOW_INSECURE_HTTP=1", backendURL)
	}

	return fmt.Errorf("refusing to send credentials to unsupported backend_url scheme %q", parsedURL.Scheme)
}

func isLoopbackHost(host string) bool {
	normalizedHost := strings.ToLower(strings.TrimSpace(host))
	if normalizedHost == "localhost" {
		return true
	}

	ip := net.ParseIP(normalizedHost)
	return ip != nil && ip.IsLoopback()
}

func formatResponseError(resp *resty.Response) string {
	body := api.SafeResponseBody(resp.Body())
	if body == "" {
		return fmt.Sprintf("status %d", resp.StatusCode())
	}

	return fmt.Sprintf("status %d: %s", resp.StatusCode(), body)
}

func redactProfile(profile profileConfig) profileConfig {
	redactedProfile := profile
	if redactedProfile.AccessToken != "" {
		redactedProfile.AccessToken = redactValue(redactedProfile.AccessToken)
	}
	if redactedProfile.Client != "" {
		redactedProfile.Client = redactValue(redactedProfile.Client)
	}
	if strings.Contains(redactedProfile.UID, "@") {
		redactedProfile.UID = profileRedactedValue
	}
	if redactedProfile.RefreshToken != "" {
		redactedProfile.RefreshToken = profileRedactedValue
	}

	return redactedProfile
}

func redactValue(value string) string {
	if len(value) <= 8 {
		return profileRedactedValue
	}

	return value[:4] + "..." + value[len(value)-4:]
}
