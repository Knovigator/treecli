package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var backendURL string

// LoginCmd represents the login command
var LoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to your account",
	Long:  `Authenticate and log in to your tree account to access protected features.`,
	Run:   runLogin,
}

func init() {
	LoginCmd.Flags().StringVarP(&backendURL, "backend-url", "b", "", "Set the Knov backend base URL")
}

func runLogin(cmd *cobra.Command, args []string) {
	// get backend url from flag, env var, or use default
	if backendURL == "" {
		backendURL = os.Getenv("KNOV_HOST")
		if backendURL == "" {
			backendURL = "https://knov-prod.onrender.com"
		}
	}

	// prompt for email
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	// prompt for password (hidden input)
	fmt.Print("Enter password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Println("\nError reading password:", err)
		return
	}
	password := string(passwordBytes)
	fmt.Println() // print a newline after password input

	// perform login
	accessToken, client, uid, err := performLogin(email, password)
	if err != nil {
		fmt.Println("Login failed:", err)
		return
	}

	// save tokens to config file
	err = saveTokens(accessToken, client, uid)
	if err != nil {
		fmt.Println("Error saving tokens:", err)
		return
	}

	fmt.Println("Login successful!")
}

func performLogin(email, password string) (string, string, string, error) {
	client := resty.New()
	// fmt.Fprintf(os.Stderr, "Making request to: %s\n", backendURL+"/auth/sign_in")
	// fmt.Fprintf(os.Stderr, "with params: email=%s, password=***\n", email)

	resp, err := client.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetBody(fmt.Sprintf("email=%s&password=%s", url.QueryEscape(email), url.QueryEscape(password))).
		Post(backendURL + "/auth/sign_in")

	if err != nil {
		return "", "", "", fmt.Errorf("error making request: %w", err)
	}

	if resp.StatusCode() != 200 {
		responseBody := resp.Body()
		if err != nil {
			fmt.Println("Error reading response body:", err)

			fmt.Printf("login failed: %s\n", string(responseBody))
			return "", "", "", fmt.Errorf("login failed with status code: %d", resp.StatusCode())
		}
	}

	// fmt.Printf("Response Body: %s\n", string(resp.Body()))
	// fmt.Println("Response Headers:")
	for key, value := range resp.Header() {
		fmt.Printf("%s: %s\n", key, value)
	}

	accessToken := resp.Header().Get("access-token")
	if accessToken == "" {
		return "", "", "", fmt.Errorf("access token not found in response headers")
	}

	clientStr := resp.Header().Get("client")
	if clientStr == "" {
		return "", "", "", fmt.Errorf("client not found in response headers")
	}

	uid := resp.Header().Get("uid")
	if uid == "" {
		return "", "", "", fmt.Errorf("uid not found in response headers")
	}

	return accessToken, clientStr, uid, nil
}

func saveTokens(accessToken, client, uid string) error {
	configPath := viper.ConfigFileUsed() // get config path from viper

	// ensure the directory exists
	err := os.MkdirAll(filepath.Dir(configPath), 0755)
	if err != nil {
		return err
	}

	viper.Set("access_token", accessToken)
	viper.Set("client", client)
	viper.Set("uid", uid)
	viper.Set("backend_url", backendURL)

	return viper.WriteConfigAs(configPath)
}
