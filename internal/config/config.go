// Package config defines the application configuration
package config

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.infratographer.com/x/crdbx"
	"go.infratographer.com/x/echox"
	"go.infratographer.com/x/events"
	"go.infratographer.com/x/loggingx"
	"go.infratographer.com/x/otelx"
	"go.infratographer.com/x/viperx"

	"go.infratographer.com/permissions-api/internal/spicedbx"
)

var defaultRuntimeSocketPath = "/tmp/runtime.sock"

// EventsConfig stores the configuration for a load-balancer-api events config
type EventsConfig struct {
	events.Config  `mapstructure:",squash"`
	Topics         []string
	ZedTokenBucket string
}

// RuntimeConfig stores the configuration for the iam-runtime
type RuntimeConfig struct {
	Socket string
}

// AppConfig is the struct used for configuring the app
type AppConfig struct {
	CRDB    crdbx.Config
	Logging loggingx.Config
	Runtime RuntimeConfig
	Server  echox.Config
	SpiceDB spicedbx.Config
	Tracing otelx.Config
	Events  EventsConfig
}

// MustViperFlags sets the cobra flags and viper config for events.
func MustViperFlags(v *viper.Viper, flags *pflag.FlagSet) {
	flags.StringSlice("events-topics", []string{}, "event topics to subscribe to")
	viperx.MustBindFlag(v, "events.topics", flags.Lookup("events-topics"))

	flags.String("events-zedtokenbucket", "", "NATS KV bucket to use for caching ZedTokens")
	viperx.MustBindFlag(v, "events.zedtokenbucket", flags.Lookup("events-zedtokenbucket"))

	flags.String("runtime-socket", "", "change the iam-runtime socket path (default: "+defaultRuntimeSocketPath+")")
	viperx.MustBindFlag(v, "runtime.socket", flags.Lookup("runtime-socket"))
}
