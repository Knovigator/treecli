package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knovigator/treecli/recorder"
	"github.com/spf13/viper"
)

// Config keys for the recorder, stored under [mcp] in treecli's config.toml.
const (
	mcpConfigMemdbBin       = "mcp.memdb_bin"
	mcpConfigMemdbDB        = "mcp.memdb_db"
	mcpConfigDataDir        = "mcp.data_dir"
	mcpConfigTreechatTeam   = "mcp.treechat.team_id"
	mcpConfigTreechatMode   = "mcp.treechat.mode"
	mcpConfigTreechatEnv    = "mcp.treechat.env"
	mcpConfigTreechatAcct   = "mcp.treechat.account"
	mcpConfigRecallOnStart  = "mcp.recall_on_start"
	mcpConfigRecallOnPrompt = "mcp.recall_on_prompt"
)

// mcpSettings is the resolved recorder configuration.
type mcpSettings struct {
	DataDir         string
	MemdbBin        string
	MemdbDB         string
	MemdbBinErr     error
	TreechatTeamID  string
	TreechatMode    string
	TreechatEnv     string
	TreechatAccount string
	RecallOnStart   bool
	RecallOnPrompt  bool
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func viperBool(key string, fallback bool) bool {
	if !viper.IsSet(key) {
		return fallback
	}
	return viper.GetBool(key)
}

// loadMcpSettings merges config.toml [mcp] keys with environment overrides.
func loadMcpSettings() (mcpSettings, error) {
	settings := mcpSettings{
		RecallOnStart:  viperBool(mcpConfigRecallOnStart, true),
		RecallOnPrompt: viperBool(mcpConfigRecallOnPrompt, true),
	}
	if value := os.Getenv("TREECLI_MCP_RECALL_ON_START"); value != "" {
		settings.RecallOnStart = value != "0" && !strings.EqualFold(value, "false")
	}
	if value := os.Getenv("TREECLI_MCP_RECALL_ON_PROMPT"); value != "" {
		settings.RecallOnPrompt = value != "0" && !strings.EqualFold(value, "false")
	}
	dataDir := firstNonEmpty(os.Getenv("TREECLI_MCP_DATA_DIR"), viper.GetString(mcpConfigDataDir))
	store, err := recorder.NewStore(dataDir)
	if err != nil {
		return settings, fmt.Errorf("resolving recorder data dir: %w", err)
	}
	settings.DataDir = store.DataDir
	settings.MemdbDB = firstNonEmpty(os.Getenv("TREECLI_MEMDB_DB"), os.Getenv("MEMDB_PATH"), viper.GetString(mcpConfigMemdbDB), store.DefaultDBPath())
	settings.MemdbBin, settings.MemdbBinErr = recorder.ResolveMemdbBinary(viper.GetString(mcpConfigMemdbBin))
	settings.TreechatTeamID = firstNonEmpty(os.Getenv("TREECLI_MCP_TREECHAT_TEAM"), viper.GetString(mcpConfigTreechatTeam))
	mode, err := recorder.ParseTreechatMode(firstNonEmpty(os.Getenv("TREECLI_MCP_TREECHAT_MODE"), viper.GetString(mcpConfigTreechatMode)))
	if err != nil {
		return settings, err
	}
	settings.TreechatMode = mode
	settings.TreechatEnv = viper.GetString(mcpConfigTreechatEnv)
	settings.TreechatAccount = viper.GetString(mcpConfigTreechatAcct)
	return settings, nil
}

// buildRecorder assembles the recorder from settings. requireMemdb makes a
// missing memdb binary an error instead of a memdb-less recorder.
func buildRecorder(settings mcpSettings, requireMemdb bool) (*recorder.Recorder, error) {
	store, err := recorder.NewStore(settings.DataDir)
	if err != nil {
		return nil, err
	}
	rec := &recorder.Recorder{Store: store}
	if settings.MemdbBin != "" {
		rec.Memdb = &recorder.Memdb{Binary: settings.MemdbBin, DBPath: settings.MemdbDB}
	} else if requireMemdb {
		return nil, settings.MemdbBinErr
	}
	if settings.TreechatMode != recorder.TreechatModeOff {
		credentials, err := treechatCredentials(settings)
		if err != nil {
			store.AppendLog("treechat lane disabled: %v", err)
		} else {
			rec.Poster = &recorder.TreechatPoster{
				Credentials: credentials,
				Store:       store,
				TeamID:      settings.TreechatTeamID,
				Mode:        settings.TreechatMode,
			}
		}
	}
	return rec, nil
}

// treechatCredentials resolves the account the recorder posts as. It honours
// the [mcp.treechat] env/account pins first, then the usual selection.
func treechatCredentials(settings mcpSettings) (recorder.Credentials, error) {
	var profile profileConfig
	var err error
	if settings.TreechatEnv != "" || settings.TreechatAccount != "" {
		environment := settings.TreechatEnv
		if environment == "" {
			environment = "prod"
		}
		account := settings.TreechatAccount
		if account == "" {
			account = "default"
		}
		profile, err = resolveNamedAccount(environment, account)
	} else {
		profile, err = resolveAccount()
	}
	if err != nil {
		return recorder.Credentials{}, err
	}
	if profile.AccessToken == "" {
		return recorder.Credentials{}, fmt.Errorf("no saved login for %s/%s (run `treecli login`)", firstNonEmpty(profile.Environment, "prod"), firstNonEmpty(profile.Account, "default"))
	}
	return recorder.Credentials{
		BackendURL:    profile.BackendURL,
		AppHost:       profile.AppHost,
		AccessToken:   profile.AccessToken,
		Client:        profile.Client,
		UID:           profile.UID,
		SpaceID:       profile.ActiveSpaceID,
		CurrentUserID: profile.CurrentUserID,
	}, nil
}

// saveMcpConfigValues persists [mcp] keys to config.toml.
func saveMcpConfigValues(values map[string]interface{}) error {
	configPath, err := configFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return err
	}
	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")
	for key, value := range values {
		viper.Set(key, value)
	}
	if _, statErr := os.Stat(configPath); statErr == nil {
		if err := viper.WriteConfig(); err != nil {
			return err
		}
		return os.Chmod(configPath, 0o600)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if err := viper.SafeWriteConfigAs(configPath); err != nil {
		return err
	}
	return os.Chmod(configPath, 0o600)
}
