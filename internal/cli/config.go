package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	cfg "github.com/allegro-php/allegro/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all config values with sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := cfg.ReadConfig(cfg.DefaultConfigPath())
		data, _ := json.MarshalIndent(c, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := cfg.ReadConfig(cfg.DefaultConfigPath())
		val, err := getConfigKey(c, args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), val)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfg.DefaultConfigPath()
		c, _ := cfg.ReadConfig(path)
		if err := setConfigKey(&c, args[0], args[1]); err != nil {
			return err
		}
		return cfg.WriteConfig(path, c)
	},
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "Remove a config override",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfg.DefaultConfigPath()
		c, _ := cfg.ReadConfig(path)
		if err := unsetConfigKey(&c, args[0]); err != nil {
			return err
		}
		return cfg.WriteConfig(path, c)
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print config file path",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), cfg.DefaultConfigPath())
	},
}

func init() {
	configCmd.AddCommand(configListCmd, configGetCmd, configSetCmd, configUnsetCmd, configPathCmd)
	rootCmd.AddCommand(configCmd)
}

var validKeys = map[string]string{
	"store_path": "string", "workers": "int", "link_strategy": "string",
	"no_progress": "bool", "no_color": "bool", "composer_path": "string",
	"no_dev": "bool", "no_scripts": "bool", "prune_stale_days": "int",
}

func getConfigKey(c cfg.Config, key string) (string, error) {
	switch key {
	case "store_path":     return c.StorePath, nil
	case "workers":        return strconv.Itoa(c.Workers), nil
	case "link_strategy":  return c.LinkStrategy, nil
	case "no_progress":    return strconv.FormatBool(c.NoProgress), nil
	case "no_color":       return strconv.FormatBool(c.NoColor), nil
	case "composer_path":  return c.ComposerPath, nil
	case "no_dev":         return strconv.FormatBool(c.NoDev), nil
	case "no_scripts":     return strconv.FormatBool(c.NoScripts), nil
	case "prune_stale_days": return strconv.Itoa(c.PruneStaleDay), nil
	default:
		return "", fmt.Errorf("unknown config key: %s", key)
	}
}

func setConfigKey(c *cfg.Config, key, value string) error {
	if _, ok := validKeys[key]; !ok {
		return fmt.Errorf("unknown config key: %s", key)
	}
	switch key {
	case "store_path":     c.StorePath = value
	case "workers":
		v, err := strconv.Atoi(value)
		if err != nil || v < 1 || v > 32 {
			return fmt.Errorf("workers must be 1-32, got %s", value)
		}
		c.Workers = v
	case "link_strategy":
		valid := map[string]bool{"auto": true, "reflink": true, "hardlink": true, "copy": true}
		if !valid[strings.ToLower(value)] {
			return fmt.Errorf("link_strategy must be auto/reflink/hardlink/copy, got %s", value)
		}
		c.LinkStrategy = strings.ToLower(value)
	case "no_progress":    c.NoProgress = parseBool(value)
	case "no_color":       c.NoColor = parseBool(value)
	case "composer_path":  c.ComposerPath = value
	case "no_dev":         c.NoDev = parseBool(value)
	case "no_scripts":     c.NoScripts = parseBool(value)
	case "prune_stale_days":
		v, err := strconv.Atoi(value)
		if err != nil || v < 1 || v > 365 {
			return fmt.Errorf("prune_stale_days must be 1-365, got %s", value)
		}
		c.PruneStaleDay = v
	}
	return nil
}

func unsetConfigKey(c *cfg.Config, key string) error {
	if _, ok := validKeys[key]; !ok {
		return fmt.Errorf("unknown config key: %s", key)
	}
	switch key {
	case "store_path":       c.StorePath = ""
	case "workers":          c.Workers = 0
	case "link_strategy":    c.LinkStrategy = ""
	case "no_progress":      c.NoProgress = false
	case "no_color":         c.NoColor = false
	case "composer_path":    c.ComposerPath = ""
	case "no_dev":           c.NoDev = false
	case "no_scripts":       c.NoScripts = false
	case "prune_stale_days": c.PruneStaleDay = 0
	}
	return nil
}

func parseBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "1" || s == "yes"
}
