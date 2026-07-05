package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Knovigator/treecli/wallet"
	"github.com/spf13/cobra"
)

var billingWalletLabel string
var billingWalletName string
var billingWalletShowSecret bool

// billingWalletCmd groups the local BSV wallet lane. None of these subcommands
// require `treecli login`: the wallet keyfile is the payment credential.
var billingWalletCmd = &cobra.Command{
	Use:   "wallet",
	Short: "Manage the local BSV wallet that pays for AI usage (no login)",
	Long: `A local treechat-style wallet pays for AI usage directly from its balance.
No account or login is required — the wallet is the payment credential.

The keyfile format matches the treechat web wallet (WIF + address), so a wallet
created here works in the web app and vice versa. Fund the wallet's address and,
once on-chain settlement is wired, charges are paid automatically from its
balance, capped by the balance itself.`,
}

var billingWalletNewCmd = &cobra.Command{
	Use:     "new [label]",
	Short:   "Create a new local BSV wallet",
	Long:    `Generate a new treechat-style BSV wallet and store it locally. The keyfile holds the private key — back it up. No login required.`,
	Example: "  treecli billing wallet new\n  treecli billing wallet new agent-bot\n  treecli billing wallet new --show-secret",
	Args:    cobra.MaximumNArgs(1),
	Run:     runBillingWalletNew,
}

var billingWalletImportCmd = &cobra.Command{
	Use:     "import <file>",
	Short:   "Import an existing treechat/Shuallet wallet JSON",
	Long:    `Import a wallet from a treechat wallet export (e.g. treechat_shuallet.json) or any JSON containing a WIF private key. No login required.`,
	Example: "  treecli billing wallet import ./treechat_shuallet.json\n  treecli billing wallet import ./wallet.json --label main",
	Args:    cobra.ExactArgs(1),
	Run:     runBillingWalletImport,
}

var billingWalletAddressCmd = &cobra.Command{
	Use:     "address [name]",
	Aliases: []string{"deposit"},
	Short:   "Show the wallet's deposit address",
	Example: "  treecli billing wallet address\n  treecli billing wallet address main",
	Args:    cobra.MaximumNArgs(1),
	Run:     runBillingWalletAddress,
}

var billingWalletListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List local wallets",
	Example: "  treecli billing wallet list",
	Run:     runBillingWalletList,
}

var billingWalletPayCmd = &cobra.Command{
	Use:   "pay",
	Short: "Pay outstanding AI usage from the local wallet (preview — not yet wired)",
	Long:  `Sign and broadcast a BSV payment for outstanding AI usage from the local wallet. On-chain signing/settlement is not wired in this build; see HANDOFF.`,
	Run:   runBillingWalletPay,
}

func init() {
	billingWalletNewCmd.Flags().StringVar(&billingWalletLabel, "label", "", "Optional label for the wallet")
	billingWalletNewCmd.Flags().BoolVar(&billingWalletShowSecret, "show-secret", false, "Print the WIF private key to stdout (handle with care)")
	billingWalletImportCmd.Flags().StringVar(&billingWalletLabel, "label", "", "Optional label for the imported wallet")
	billingWalletAddressCmd.Flags().StringVar(&billingWalletName, "wallet", "", "Wallet name/label to act on")
	billingWalletPayCmd.Flags().StringVar(&billingWalletName, "wallet", "", "Wallet name/label to pay from")

	billingWalletCmd.AddCommand(billingWalletNewCmd)
	billingWalletCmd.AddCommand(billingWalletImportCmd)
	billingWalletCmd.AddCommand(billingWalletAddressCmd)
	billingWalletCmd.AddCommand(billingWalletListCmd)
	billingWalletCmd.AddCommand(billingWalletPayCmd)
}

func runBillingWalletNew(cmd *cobra.Command, args []string) {
	label := strings.TrimSpace(billingWalletLabel)
	if label == "" && len(args) > 0 {
		label = strings.TrimSpace(args[0])
	}

	newWallet, err := wallet.Generate(label)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	path, err := wallet.Save(newWallet)
	if err != nil {
		fmt.Println("Error saving wallet:", err)
		return
	}

	fmt.Println("Created a new BSV wallet.")
	if newWallet.Label != "" {
		fmt.Printf("Label:   %s\n", newWallet.Label)
	}
	fmt.Printf("Address: %s\n", newWallet.Address)
	fmt.Printf("Keyfile: %s\n", path)
	if billingWalletShowSecret {
		fmt.Printf("WIF:     %s\n", newWallet.WIF)
	} else {
		fmt.Printf("WIF:     %s  (use --show-secret to reveal)\n", newWallet.Redacted())
	}
	fmt.Println()
	fmt.Println("Back up the keyfile — it holds your funds. Anyone with it can spend.")
	fmt.Println("Fund the address above to start paying. No login needed.")
}

func runBillingWalletImport(cmd *cobra.Command, args []string) {
	sourcePath := strings.TrimSpace(args[0])
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		fmt.Println("Error reading wallet file:", err)
		return
	}

	imported, err := wallet.ParseImport(data, strings.TrimSpace(billingWalletLabel))
	if err != nil {
		fmt.Println("Error importing wallet:", err)
		return
	}

	path, err := wallet.Save(imported)
	if err != nil {
		fmt.Println("Error saving wallet:", err)
		return
	}

	fmt.Println("Imported wallet.")
	if imported.Label != "" {
		fmt.Printf("Label:   %s\n", imported.Label)
	}
	fmt.Printf("Address: %s\n", imported.Address)
	fmt.Printf("Keyfile: %s\n", path)
	fmt.Println()
	fmt.Printf("Remove the source file when done:  rm %s\n", sourcePath)
}

func runBillingWalletAddress(cmd *cobra.Command, args []string) {
	name := strings.TrimSpace(billingWalletName)
	if name == "" && len(args) > 0 {
		name = strings.TrimSpace(args[0])
	}

	resolved, err := wallet.Resolve(name)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if resolved.Label != "" {
		fmt.Printf("%s\t%s\n", resolved.Label, resolved.Address)
	} else {
		fmt.Println(resolved.Address)
	}
	fmt.Println("Send BSV to this address to fund AI usage. No login needed.")
}

func runBillingWalletList(cmd *cobra.Command, args []string) {
	files, err := wallet.List()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	if len(files) == 0 {
		fmt.Println("No local wallets. Create one: treecli billing wallet new")
		return
	}

	for _, file := range files {
		label := file.Label
		if label == "" {
			label = "(unlabeled)"
		}
		fmt.Printf("%s\t%s\n", label, file.Address)
	}
}

func runBillingWalletPay(cmd *cobra.Command, args []string) {
	resolved, err := wallet.Resolve(strings.TrimSpace(billingWalletName))
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Wallet: %s (%s)\n", resolved.Name(), resolved.Address)
	fmt.Println("On-chain BSV settlement is not wired in this build yet.")
	fmt.Println("Once wired, this will sign a payment for outstanding AI usage from")
	fmt.Println("this wallet's balance (auto-pay is the BSV default), capped by the balance.")
	fmt.Println("See HANDOFF.md for the settlement path to implement.")
}
