package wallet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateRoundTrip(t *testing.T) {
	generated, err := Generate("unit")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(generated.Address, "1") {
		t.Fatalf("expected mainnet P2PKH address, got %q", generated.Address)
	}
	if generated.WIF == "" || generated.PublicKey == "" {
		t.Fatalf("missing wif/publicKey: %+v", generated)
	}

	// Re-importing the WIF must derive the identical address.
	reimported, err := FromWIF(generated.WIF, "unit")
	if err != nil {
		t.Fatalf("FromWIF: %v", err)
	}
	if reimported.Address != generated.Address {
		t.Fatalf("address mismatch on reimport: %q != %q", reimported.Address, generated.Address)
	}
}

func TestParseImportShuallet(t *testing.T) {
	seed, err := Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	shuallet, _ := json.Marshal(map[string]string{
		"payPk":   seed.WIF,
		"payAddr": seed.Address,
		"ordPk":   "unused",
	})

	imported, err := ParseImport(shuallet, "main")
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	if imported.Address != seed.Address {
		t.Fatalf("imported address %q != %q", imported.Address, seed.Address)
	}
	if imported.Label != "main" {
		t.Fatalf("label not applied: %q", imported.Label)
	}
}

func TestParseImportAddressMismatch(t *testing.T) {
	seed, err := Generate("")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"payPk":   seed.WIF,
		"payAddr": "1SomeOtherAddressThatIsWrong00000",
	})

	if _, err := ParseImport(payload, ""); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestParseImportNoKey(t *testing.T) {
	if _, err := ParseImport([]byte(`{"ordPk":"x"}`), ""); err == nil {
		t.Fatal("expected error when no payment key present")
	}
}
