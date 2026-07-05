// Package wallet manages local treechat-style BSV wallets for the CLI.
//
// A wallet is a Treechat/Shuallet-style keyfile with separate payment and
// ordinals WIF keys (`payPk` and `ordPk`). The payment key derives the address
// used by the CLI's BSV billing lane.
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

// File is the on-disk Shuallet-compatible keyfile. `payPk` and `ordPk` are the
// canonical Treechat wallet fields; address fields are derived convenience
// metadata for CLI display.
type File struct {
	Label      string `json:"label,omitempty"`
	PayWIF     string `json:"payPk"`
	OrdWIF     string `json:"ordPk"`
	PublicKey  string `json:"publicKey,omitempty"`
	Address    string `json:"address,omitempty"`
	OrdAddress string `json:"ordinalsAddress,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// Generate creates a brand-new random wallet.
func Generate(label string) (*File, error) {
	paymentKey, err := ec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generating payment key: %w", err)
	}

	ordinalKey, err := ec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generating ordinals key: %w", err)
	}

	return fromPrivateKeys(paymentKey, ordinalKey, label)
}

// FromWIF builds a Shuallet-compatible wallet from an existing payment WIF.
// Legacy single-key imports reuse the payment key as the ordinals key so the
// resulting keyfile still has Treechat's required `payPk`/`ordPk` shape.
func FromWIF(wif, label string) (*File, error) {
	privateKey, err := ec.PrivateKeyFromWif(strings.TrimSpace(wif))
	if err != nil {
		return nil, fmt.Errorf("invalid WIF: %w", err)
	}

	return fromPrivateKeys(privateKey, privateKey, label)
}

func fromWIFs(paymentWIF, ordinalWIF, label string) (*File, error) {
	paymentKey, err := ec.PrivateKeyFromWif(strings.TrimSpace(paymentWIF))
	if err != nil {
		return nil, fmt.Errorf("invalid payment WIF: %w", err)
	}

	ordinalWIF = strings.TrimSpace(ordinalWIF)
	if ordinalWIF == "" {
		ordinalWIF = paymentWIF
	}
	ordinalKey, err := ec.PrivateKeyFromWif(ordinalWIF)
	if err != nil {
		return nil, fmt.Errorf("invalid ordinals WIF: %w", err)
	}

	return fromPrivateKeys(paymentKey, ordinalKey, label)
}

func fromPrivateKeys(paymentKey, ordinalKey *ec.PrivateKey, label string) (*File, error) {
	paymentPublicKey := paymentKey.PubKey()
	paymentAddress, err := script.NewAddressFromPublicKey(paymentPublicKey, mainnet)
	if err != nil {
		return nil, fmt.Errorf("deriving payment address: %w", err)
	}

	ordinalAddress, err := script.NewAddressFromPublicKey(ordinalKey.PubKey(), mainnet)
	if err != nil {
		return nil, fmt.Errorf("deriving ordinals address: %w", err)
	}

	return &File{
		Label:      strings.TrimSpace(label),
		PayWIF:     paymentKey.Wif(),
		OrdWIF:     ordinalKey.Wif(),
		PublicKey:  paymentPublicKey.ToDERHex(),
		Address:    paymentAddress.AddressString,
		OrdAddress: ordinalAddress.AddressString,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// paymentKeyFieldCandidates are the JSON keys, in priority order, that hold a
// payment private key across Treechat/Shuallet exports and legacy CLI keyfiles.
var paymentKeyFieldCandidates = []string{"payPk", "paypk", "payWif", "paymentKey", "privateKey", "private_key", "wif"}

var ordinalKeyFieldCandidates = []string{"ordPk", "ordpk", "ordWif", "ordinalKey", "ownerKey", "ownerWif"}

var paymentAddressFieldCandidates = []string{"address", "payAddr", "payaddr", "paymentAddress", "payment_address"}
var ordinalAddressFieldCandidates = []string{"ordAddress", "ordaddress", "ordAddr", "ordaddr", "ordinalAddress", "ordinalsAddress"}

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

	resolvedLabel := strings.TrimSpace(label)
	if resolvedLabel == "" {
		resolvedLabel = firstString(lower, []string{"label", "name"})
	}

	paymentWIF := firstString(lower, paymentKeyFieldCandidates)
	if paymentWIF == "" {
		return nil, fmt.Errorf("no payment key found in wallet JSON (looked for: %s)", strings.Join(paymentKeyFieldCandidates, ", "))
	}

	ordinalWIF := firstString(lower, ordinalKeyFieldCandidates)
	file, err := fromWIFs(paymentWIF, ordinalWIF, resolvedLabel)
	if err != nil {
		return nil, err
	}

	if createdAt := firstString(lower, []string{"created_at", "createdAt"}); createdAt != "" {
		file.CreatedAt = createdAt
	}

	declared := firstString(lower, paymentAddressFieldCandidates)
	if declared != "" && !strings.EqualFold(declared, file.Address) {
		return nil, fmt.Errorf("declared payment address %q does not match the key (derives %q)", declared, file.Address)
	}

	declaredOrdinal := firstString(lower, ordinalAddressFieldCandidates)
	if declaredOrdinal != "" && !strings.EqualFold(declaredOrdinal, file.OrdAddress) {
		return nil, fmt.Errorf("declared ordinals address %q does not match the key (derives %q)", declaredOrdinal, file.OrdAddress)
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

	normalized, err := fromWIFs(file.PayWIF, file.OrdWIF, file.Label)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(file.CreatedAt) != "" {
		normalized.CreatedAt = file.CreatedAt
	}

	name := sanitizeName(normalized.Label)
	if name == "" {
		name = sanitizeName(normalized.Address)
	}
	if name == "" {
		return "", fmt.Errorf("wallet has neither label nor address")
	}

	path := filepath.Join(dir, name+".json")
	encoded, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", err
	}

	writer, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("a wallet named %q already exists at %s", name, path)
		}
		return "", err
	}
	if _, err := writer.Write(encoded); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
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

	file, err := ParseImport(data, "")
	if err != nil {
		return nil, fmt.Errorf("wallet %q is corrupt: %w", name, err)
	}

	return file, nil
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
	wif := file.PayWIF
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
