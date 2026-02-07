package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"patchmon-agent/internal/config"
	"patchmon-agent/internal/constants"
	"patchmon-agent/internal/utils"
	"patchmon-agent/internal/version"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	cfgManager *config.Manager
	logger     *logrus.Logger
	configFile string
	logLevel   string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "patchmon-agent",
	Short:   "PatchMon Agent for package monitoring",
	Version: version.Version,
	Long: `PatchMon Agent v` + version.Version + `

A monitoring agent that sends package information to PatchMon.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		initialiseAgent()
		updateLogLevel(cmd)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Set default values
	configFile = config.DefaultConfigFile
	logLevel = config.DefaultLogLevel

	// Add global flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", configFile, "config file path")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", logLevel, "log level (debug, info, warn, error)")

	// Add all subcommands
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(pingCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(checkVersionCmd)
	rootCmd.AddCommand(updateAgentCmd)
	rootCmd.AddCommand(diagnosticsCmd)
	// Note: Uninstall functionality removed - use patchmon_remove.sh script instead
	// rootCmd.AddCommand(uninstallCmd)
}

// initialiseAgent initialises the configuration manager and logger
func initialiseAgent() {
	// Initialise logger
	logger = logrus.New()
	// Get timezone for log timestamps
	// Note: logrus TextFormatter doesn't directly support timezone
	// The timestamp will use the system timezone, but we can configure
	// the TZ environment variable to control this
	tz_loc := utils.GetTimezoneLocation()
	logger.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: false,
		FullTimestamp:    true,
		TimestampFormat:  "2006-01-02T15:04:05",
	})
	// Store timezone location for future use if needed
	_ = tz_loc

	// Initialise configuration manager
	cfgManager = config.New()
	cfgManager.SetConfigFile(configFile)

	// Load config early to determine log file path
	_ = cfgManager.LoadConfig()
	logFile := cfgManager.GetConfig().LogFile
	if logFile == "" {
		logFile = config.DefaultLogFile
	}
	// SECURITY: Use 0750 for log directory (no world access)
	_ = os.MkdirAll(filepath.Dir(logFile), 0750)
	logger.SetOutput(&lumberjack.Logger{Filename: logFile, MaxSize: 10, MaxBackups: 5, MaxAge: 14, Compress: true})
}

// updateLogLevel sets the logger level based on the flag value
func updateLogLevel(cmd *cobra.Command) {
	// Load configuration first
	if err := cfgManager.LoadConfig(); err != nil {
		logger.WithError(err).Warn("Failed to load config")
	}

	// Check if the log-level flag was explicitly set
	flagLogLevel := logLevel
	if cmd.Flag("log-level").Changed {
		// Flag was explicitly set, use it
		level, err := logrus.ParseLevel(flagLogLevel)
		if err != nil {
			level = logrus.InfoLevel
		}
		logger.SetLevel(level)
		cfgManager.GetConfig().LogLevel = flagLogLevel
	} else {
		// Flag was not set, use config file value if available
		configLogLevel := cfgManager.GetConfig().LogLevel
		if configLogLevel != "" {
			level, err := logrus.ParseLevel(configLogLevel)
			if err != nil {
				level = logrus.InfoLevel
			}
			logger.SetLevel(level)
		} else {
			// No config value either, use default
			logger.SetLevel(logrus.InfoLevel)
			cfgManager.GetConfig().LogLevel = constants.LogLevelInfo
		}
	}
}

// checkRoot ensures the command is run as root
func checkRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("this command requires root privileges, please run with sudo or as root user")
	}
	return nil
}
