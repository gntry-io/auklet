package cmd

import (
	"fmt"
	"os"

	"github.com/gntry-io/auklet/pkg/auklet"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// RootCmd represents the base command when called without any subcommands
	RootCmd = &cobra.Command{
		Use:     "auklet",
		Version: fmt.Sprintf("%s-%s (built by %s on %s)", version, gitTag, buildUser, buildDate),
		Short:   "A service autoscaler for Docker Swarm",
		Long:    `Autoscale services in Docker Swarm based on service labels by querying Prometheus.`,
		Run: func(cmd *cobra.Command, args []string) {

			if viper.GetBool("debug") {
				log.SetLevel(log.DebugLevel)
			}

			if viper.GetBool("no-color") {
				logFormat = log.TextFormatter{
					ForceColors: false,
					DisableColors: true,
					FullTimestamp: true,
					TimestampFormat: "2006-01-02T15:04:05.999999999",
				}
			}

			if viper.GetBool("json") {
				log.SetFormatter(&log.JSONFormatter{})
			}


			auklet, err := auklet.New(viper.GetString("prometheus-url"), viper.GetInt("listen"))
			if err != nil {
				log.Error(err)
				log.Error("Auklet aborted flight")
				os.Exit(1)
			}

			if err = auklet.Fly(); err != nil {
				log.Error(err)
				log.Error("Auklet aborted flight")
				os.Exit(1)
			}
		},
	}

	logFormat = log.TextFormatter{
		ForceColors: true,
		DisableColors: false,
		FullTimestamp: true,
		TimestampFormat: "2006-01-02T15:04:05.999999999",
	}

	cfgFile  string
	promURL  string
	debug    bool
	json	 bool
	nocolor  bool
	httpPort int
)

func init() {
	// Default logging settings
	log.SetOutput(os.Stdout)
	log.SetFormatter(&logFormat)
	log.SetLevel(log.InfoLevel)

	// Flags & Config
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (e.g. $HOME/.auklet)")
	RootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "enable debug logging")
	RootCmd.PersistentFlags().BoolVar(&nocolor, "no-color", false, "disable colors in logging")
	RootCmd.PersistentFlags().BoolVarP(&json, "json", "j", false, "Log output in JSON format")
	RootCmd.PersistentFlags().StringVarP(&promURL, "prometheus-url", "p", "", "Prometheus API URL")
	RootCmd.PersistentFlags().IntVarP(&httpPort, "listen", "l", 8080, "Port of HTTP listener")
	_ = RootCmd.MarkFlagRequired("prometheus-url")
	_ = viper.BindPFlag("debug", RootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("prometheus-url", RootCmd.PersistentFlags().Lookup("prometheus-url"))
	_ = viper.BindPFlag("json", RootCmd.PersistentFlags().Lookup("json"))
	_ = viper.BindPFlag("no-color", RootCmd.PersistentFlags().Lookup("no-color"))
	_ = viper.BindPFlag("listen", RootCmd.PersistentFlags().Lookup("listen"))
}

func initConfig() {
	// Automatically bind flags to AUKLET_<flagname> environment variables
	viper.SetEnvPrefix("auklet")
	viper.AutomaticEnv()

	// Try to load configuration file when specified
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config
		viper.AddConfigPath("/etc/auklet/")
		viper.AddConfigPath(fmt.Sprintf("%s/.auklet/", home))
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		// Ignore, just use flags/env
	}
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		log.Error(err)
		log.Error("Auklet aborted flight")
		os.Exit(-1)
	}
}
