package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
)

const signupEndpoint = "/auth"

var signupUsername string
var signupEmail string
var readSignupPasswordFromStdin bool

type signupRequest struct {
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

// SignupCmd represents the signup command.
var SignupCmd = &cobra.Command{
	Use:   "signup",
	Short: "Create an account",
	Long:  `Create a Treechat account interactively and save its authenticated profile.`,
	Args:  cobra.NoArgs,
	RunE:  runSignup,
}

func init() {
	SignupCmd.Flags().StringVarP(&signupUsername, "username", "u", "", "Username for the new account")
	SignupCmd.Flags().StringVarP(&signupEmail, "email", "e", "", "Email address for the new account")
	SignupCmd.Flags().BoolVar(&readSignupPasswordFromStdin, "password-stdin", false, "Read the password from stdin and use it as confirmation")
}

func runSignup(cmd *cobra.Command, args []string) error {
	profileName := resolveProfileName()
	profile, err := resolveProfile(profileName)
	if err != nil {
		return fmt.Errorf("signup failed: %w", err)
	}
	if err := validateCredentialTransport(profile.BackendURL); err != nil {
		return fmt.Errorf("signup failed: %w", err)
	}

	if readSignupPasswordFromStdin &&
		(strings.TrimSpace(signupUsername) == "" || strings.TrimSpace(signupEmail) == "") {
		return fmt.Errorf("signup failed: --password-stdin requires --username and --email")
	}

	inputReader := bufio.NewReader(os.Stdin)
	username, err := resolveSignupUsername(inputReader)
	if err != nil {
		return fmt.Errorf("signup failed: %w", err)
	}

	email, err := resolveSignupEmail(inputReader)
	if err != nil {
		return fmt.Errorf("signup failed: %w", err)
	}

	password, err := resolveSignupPassword()
	if err != nil {
		return fmt.Errorf("signup failed: %w", err)
	}

	tokens, err := performSignup(profile.BackendURL, username, email, password)
	if err != nil {
		return fmt.Errorf("signup failed: %w", err)
	}

	bootstrap, err := fetchBootstrap(profile.BackendURL, tokens)
	if err != nil {
		return fmt.Errorf("signup failed: account created but profile setup failed: %w; run treecli login --profile %s", err, profile.Name)
	}

	profile.AccessToken = tokens.AccessToken
	profile.Client = tokens.Client
	profile.UID = tokens.UID
	profile.Expiry = tokens.Expiry
	profile.AppHost = normalizeAppHost(bootstrap.Host)
	profile.CurrentUserID = bootstrap.CurrentUser.ID
	profile.ActiveSpaceID = bootstrap.CurrentUser.SpaceID

	if err := saveProfile(profile, true); err != nil {
		return fmt.Errorf("signup succeeded but saving profile failed: %w", err)
	}

	fmt.Printf("Signup successful. Profile: %s Backend: %s\n", profile.Name, profile.BackendURL)
	return nil
}

func resolveSignupUsername(reader *bufio.Reader) (string, error) {
	if strings.TrimSpace(signupUsername) != "" {
		return strings.TrimSpace(signupUsername), nil
	}

	fmt.Print("Enter username: ")
	username, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading username: %w", err)
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", fmt.Errorf("username cannot be empty")
	}

	return username, nil
}

func resolveSignupEmail(reader *bufio.Reader) (string, error) {
	if strings.TrimSpace(signupEmail) != "" {
		return strings.TrimSpace(signupEmail), nil
	}

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

func resolveSignupPassword() (string, error) {
	if readSignupPasswordFromStdin {
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

	password, err := readHiddenPassword("Enter password: ")
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	confirmation, err := readHiddenPassword("Confirm password: ")
	if err != nil {
		return "", err
	}
	if confirmation != password {
		return "", fmt.Errorf("passwords do not match")
	}

	return password, nil
}

func performSignup(backendURL, username, email, password string) (authTokens, error) {
	client := resty.New()
	client.SetTimeout(10 * time.Second)

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetBody(signupRequest{
			Name:                 username,
			Email:                email,
			Password:             password,
			PasswordConfirmation: password,
		}).
		Post(backendURL + signupEndpoint)
	if err != nil {
		return authTokens{}, fmt.Errorf("error making signup request: %w", err)
	}

	if resp.StatusCode() < http.StatusOK || resp.StatusCode() >= http.StatusMultipleChoices {
		return authTokens{}, fmt.Errorf("registration request failed: %s", formatResponseError(resp))
	}

	return authTokensFromResponse(resp)
}
