package cmd

import (
	"fmt"
	"os"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// RootCmd represents the base command when called without any subcommands
	RootCmd = &cobra.Command{
		Use:   "auklet",
		Version: fmt.Sprintf("%s-%s (built by %s on %s)", version, gitTag, buildUser, buildDate),
		Short: "A service autoscaler for Docker Swarm",
		Long:  `
Autoscale services in Docker Swarm based on service
labels by querying Prometheus.`,
	}

	cfgFile string
)

func init() {
	cobra.OnInitialize(initConfig)
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.auklet)")
}

func initConfig() {
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

		// Search config in home directory with name ".auklet" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".auklet")
	}

	if err := viper.ReadInConfig(); err != nil {
		// Ignore it, config file is optional..
	}
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
