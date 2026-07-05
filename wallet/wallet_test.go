package wallet

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/adrg/xdg"
)

func TestGenerateCreatesShualletKeyfileShape(t *testing.T) {
	generated, err := Generate("unit")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(generated.Address, "1") {
		t.Fatalf("expected mainnet payment address, got %q", generated.Address)
	}
	if !strings.HasPrefix(generated.OrdAddress, "1") {
		t.Fatalf("expected mainnet ordinals address, got %q", generated.OrdAddress)
	}
	if generated.PayWIF == "" || generated.OrdWIF == "" || generated.PublicKey == "" {
		t.Fatalf("missing shuallet keys/publicKey: %+v", generated)
	}
	if generated.PayWIF == generated.OrdWIF {
		t.Fatal("expected generated wallet to use separate payment and ordinals keys")
	}

	encoded, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal generated wallet: %v", err)
	}
	if _, ok := raw["payPk"]; !ok {
		t.Fatal("generated wallet JSON missing payPk")
	}
	if _, ok := raw["ordPk"]; !ok {
		t.Fatal("generated wallet JSON missing ordPk")
	}
	if _, ok := raw["wif"]; ok {
		t.Fatal("generated wallet JSON should not use legacy wif field")
	}

	reimported, err := FromWIF(generated.PayWIF, "unit")
	if err != nil {
		t.Fatalf("FromWIF: %v", err)
	}
	if reimported.Address != generated.Address {
		t.Fatalf("address mismatch on reimport: %q != %q", reimported.Address, generated.Address)
	}
	if reimported.OrdWIF != generated.PayWIF {
		t.Fatal("legacy WIF import should fill ordPk from the payment key")
	}
}

func TestParseImportShuallet(t *testing.T) {
	seed, err := Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	shuallet, _ := json.Marshal(map[string]string{
		"payPk":           seed.PayWIF,
		"ordPk":           seed.OrdWIF,
		"address":         seed.Address,
		"ordinalsAddress": seed.OrdAddress,
	})

	imported, err := ParseImport(shuallet, "main")
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	if imported.PayWIF != seed.PayWIF {
		t.Fatal("payment key was not preserved")
	}
	if imported.OrdWIF != seed.OrdWIF {
		t.Fatal("ordinals key was not preserved")
	}
	if imported.Address != seed.Address {
		t.Fatalf("imported address %q != %q", imported.Address, seed.Address)
	}
	if imported.OrdAddress != seed.OrdAddress {
		t.Fatalf("imported ordinals address %q != %q", imported.OrdAddress, seed.OrdAddress)
	}
	if imported.Label != "main" {
		t.Fatalf("label not applied: %q", imported.Label)
	}
}

func TestParseImportLegacyWIF(t *testing.T) {
	seed, err := Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"wif":     seed.PayWIF,
		"address": seed.Address,
		"label":   "legacy",
	})

	imported, err := ParseImport(payload, "")
	if err != nil {
		t.Fatalf("ParseImport legacy WIF: %v", err)
	}
	if imported.PayWIF != seed.PayWIF || imported.OrdWIF != seed.PayWIF {
		t.Fatal("legacy WIF import should populate payPk and ordPk from wif")
	}
	if imported.Label != "legacy" {
		t.Fatalf("expected label from import, got %q", imported.Label)
	}
}

func TestParseImportPaymentAddressMismatch(t *testing.T) {
	seed, err := Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"payPk":   seed.PayWIF,
		"ordPk":   seed.OrdWIF,
		"address": "1SomeOtherAddressThatIsWrong00000",
	})

	if _, err := ParseImport(payload, ""); err == nil {
		t.Fatal("expected payment address mismatch error, got nil")
	}
}

func TestParseImportOrdinalsAddressMismatch(t *testing.T) {
	seed, err := Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"payPk":           seed.PayWIF,
		"ordPk":           seed.OrdWIF,
		"ordinalsAddress": "1SomeOtherAddressThatIsWrong00000",
	})

	if _, err := ParseImport(payload, ""); err == nil {
		t.Fatal("expected ordinals address mismatch error, got nil")
	}
}

func TestParseImportNoPaymentKey(t *testing.T) {
	if _, err := ParseImport([]byte(`{"ordPk":"x"}`), ""); err == nil {
		t.Fatal("expected error when no payment key present")
	}
}

func TestSaveLoadAndListUseShualletKeyfile(t *testing.T) {
	withTempConfigHome(t)

	generated, err := Generate("main")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path, err := Save(generated)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat saved wallet: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected wallet file mode 0600, got %04o", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read saved wallet: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal saved wallet: %v", err)
	}
	if _, ok := raw["payPk"]; !ok {
		t.Fatal("saved wallet missing payPk")
	}
	if _, ok := raw["ordPk"]; !ok {
		t.Fatal("saved wallet missing ordPk")
	}
	if _, ok := raw["wif"]; ok {
		t.Fatal("saved wallet should not include legacy wif")
	}

	loaded, err := Load("main")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PayWIF != generated.PayWIF || loaded.OrdWIF != generated.OrdWIF {
		t.Fatal("loaded wallet keys did not match generated keys")
	}
	if loaded.Address != generated.Address || loaded.OrdAddress != generated.OrdAddress {
		t.Fatal("loaded wallet addresses did not match generated addresses")
	}

	listed, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Name() != "main" {
		t.Fatalf("expected one listed wallet named main, got %+v", listed)
	}

	resolved, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.PayWIF != generated.PayWIF {
		t.Fatal("resolved wallet did not match generated wallet")
	}
}

func TestSaveRejectsDuplicateWalletName(t *testing.T) {
	withTempConfigHome(t)

	first, err := Generate("main")
	if err != nil {
		t.Fatalf("Generate first: %v", err)
	}
	if _, err := Save(first); err != nil {
		t.Fatalf("Save first: %v", err)
	}

	second, err := Generate("main")
	if err != nil {
		t.Fatalf("Generate second: %v", err)
	}
	if _, err := Save(second); err == nil {
		t.Fatal("expected duplicate wallet save to fail")
	}
}

func withTempConfigHome(t *testing.T) {
	t.Helper()

	oldValue, hadOldValue := os.LookupEnv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", t.TempDir()); err != nil {
		t.Fatalf("Setenv XDG_CONFIG_HOME: %v", err)
	}
	xdg.Reload()

	t.Cleanup(func() {
		if hadOldValue {
			_ = os.Setenv("XDG_CONFIG_HOME", oldValue)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		xdg.Reload()
	})
}
