// Package wallet manages local treechat-style BSV wallets for the CLI.
//
// A wallet is a single-key P2PKH keypair whose private key is stored as WIF,
// exactly the format the treechat web app produces (generateRandomKey =>
// {wif, publicKey, address}) and downloads (treechat_shuallet.json). Because
// WIF and P2PKH addresses are standard, a wallet created here is interoperable
// with the web wallet: the same WIF yields the same address in both.
//
// This package deliberately does NOT sign or broadcast transactions. Keygen,
// import, and address derivation are local and safe; spending is left to the
// billing "pay" flow (see cmd/billing_wallet.go), which is where on-chain
// signing/settlement gets wired.
package wallet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adrg/xdg"
	ec "github.com/bsv-blockchain/go-sdk/primitives/ec"
	"github.com/bsv-blockchain/go-sdk/script"
)

// mainnet selects BSV mainnet address encoding.
const mainnet = true

// File is the on-disk keyfile. `wif` and `address` are load-bearing; the rest
// is convenience metadata. It is a superset of the treechat/Shuallet export,
// so a treechat wallet JSON can be imported and a File can be read by tools
// that only look for `wif`/`address`.
type File struct {
	Label     string `json:"label,omitempty"`
	WIF       string `json:"wif"`
	PublicKey string `json:"publicKey,omitempty"`
	Address   string `json:"address"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Generate creates a brand-new random wallet.
func Generate(label string) (*File, error) {
	privateKey, err := ec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	return fromPrivateKey(privateKey, label)
}

// FromWIF builds a wallet from an existing WIF private key.
func FromWIF(wif, label string) (*File, error) {
	privateKey, err := ec.PrivateKeyFromWif(strings.TrimSpace(wif))
	if err != nil {
		return nil, fmt.Errorf("invalid WIF: %w", err)
	}

	return fromPrivateKey(privateKey, label)
}

func fromPrivateKey(privateKey *ec.PrivateKey, label string) (*File, error) {
	publicKey := privateKey.PubKey()
	address, err := script.NewAddressFromPublicKey(publicKey, mainnet)
	if err != nil {
		return nil, fmt.Errorf("deriving address: %w", err)
	}

	return &File{
		Label:     strings.TrimSpace(label),
		WIF:       privateKey.Wif(),
		PublicKey: publicKey.ToDERHex(),
		Address:   address.AddressString,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// wifFieldCandidates are the JSON keys, in priority order, that hold a payment
// private key across treechat / Shuallet exports and our own keyfiles.
// NOTE: confirm this list against BsvWallet#as_json before relying on import
// for real treechat wallets (see HANDOFF).
var wifFieldCandidates = []string{"wif", "payPk", "paypk", "privateKey", "private_key", "paymentKey", "payWif"}

var addressFieldCandidates = []string{"address", "payAddr", "payaddr", "paymentAddress"}

// ParseImport tolerantly reads a treechat-style wallet export and extracts the
// payment key. The address is re-derived from the WIF and cross-checked against
// any address the file declares.
func ParseImport(data []byte, label string) (*File, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("not a JSON wallet: %w", err)
	}

	lower := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		lower[strings.ToLower(key)] = value
	}

	wif := firstString(lower, wifFieldCandidates)
	if wif == "" {
		return nil, fmt.Errorf("no payment key found in wallet JSON (looked for: %s)", strings.Join(wifFieldCandidates, ", "))
	}

	file, err := FromWIF(wif, label)
	if err != nil {
		return nil, err
	}

	declared := firstString(lower, addressFieldCandidates)
	if declared != "" && !strings.EqualFold(declared, file.Address) {
		return nil, fmt.Errorf("declared address %q does not match the key (derives %q)", declared, file.Address)
	}

	return file, nil
}

func firstString(lower map[string]json.RawMessage, candidates []string) string {
	for _, candidate := range candidates {
		value, ok := lower[strings.ToLower(candidate)]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

// Dir is where wallet keyfiles live (alongside the treecli config).
func Dir() (string, error) {
	configPath, err := xdg.ConfigFile("treecli/wallets/.keep")
	if err != nil {
		return "", err
	}
	return filepath.Dir(configPath), nil
}

// Save writes the wallet keyfile with owner-only permissions and returns its
// path. The basename is the label if present, otherwise the address.
func Save(file *File) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	name := sanitizeName(file.Label)
	if name == "" {
		name = sanitizeName(file.Address)
	}
	if name == "" {
		return "", fmt.Errorf("wallet has neither label nor address")
	}

	path := filepath.Join(dir, name+".json")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("a wallet named %q already exists at %s", name, path)
	}

	encoded, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return "", err
	}

	return path, nil
}

// Load reads a wallet by label/name (with or without the .json suffix).
func Load(name string) (*File, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sanitizeName(strings.TrimSuffix(name, ".json"))+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("wallet %q is corrupt: %w", name, err)
	}

	return &file, nil
}

// List returns the saved wallets sorted by name.
func List() ([]*File, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]*File, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		file, err := Load(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		files = append(files, file)
	}

	sort.Slice(files, func(left, right int) bool {
		return files[left].Name() < files[right].Name()
	})

	return files, nil
}

// Resolve returns the wallet to act on: the named one, or the only one when no
// name is given and exactly one exists.
func Resolve(name string) (*File, error) {
	if strings.TrimSpace(name) != "" {
		return Load(name)
	}

	files, err := List()
	if err != nil {
		return nil, err
	}
	switch len(files) {
	case 0:
		return nil, fmt.Errorf("no local wallet; run `treecli billing wallet new` or `... import <file>`")
	case 1:
		return files[0], nil
	default:
		return nil, fmt.Errorf("multiple wallets; pass --wallet <name> (see `treecli billing wallet list`)")
	}
}

// Name is the wallet's display name: its label, or its address if unlabeled.
func (file *File) Name() string {
	if strings.TrimSpace(file.Label) != "" {
		return file.Label
	}
	return file.Address
}

// Redacted returns the WIF with the middle masked, for display.
func (file *File) Redacted() string {
	wif := file.WIF
	if len(wif) <= 8 {
		return "********"
	}
	return wif[:4] + "..." + wif[len(wif)-4:]
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	var builder strings.Builder
	for _, char := range name {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9', char == '-', char == '_', char == '.':
			builder.WriteRune(char)
		default:
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "._")
}
