package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.infratographer.com/x/echox"
	"go.infratographer.com/x/events"
	"go.infratographer.com/x/otelx"
	"go.infratographer.com/x/versionx"
	"go.uber.org/zap"

	"go.infratographer.com/permissions-api/internal/config"
	"go.infratographer.com/permissions-api/internal/iapl"
	"go.infratographer.com/permissions-api/internal/pubsub"
	"go.infratographer.com/permissions-api/internal/query"
	"go.infratographer.com/permissions-api/internal/spicedbx"
)

const shutdownTimeout = 10 * time.Second

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "starts a permissions-api queue worker",
	Run: func(cmd *cobra.Command, args []string) {
		worker(cmd.Context(), globalCfg)
	},
}

func init() {
	rootCmd.AddCommand(workerCmd)

	otelx.MustViperFlags(viper.GetViper(), workerCmd.Flags())
	events.MustViperFlags(viper.GetViper(), workerCmd.Flags(), appName)
	echox.MustViperFlags(viper.GetViper(), workerCmd.Flags(), apiDefaultListen)
	config.MustViperFlags(viper.GetViper(), workerCmd.Flags())
}

func worker(ctx context.Context, cfg *config.AppConfig) {
	err := otelx.InitTracer(cfg.Tracing, appName, logger)
	if err != nil {
		logger.Fatalw("unable to initialize tracing system", "error", err)
	}

	spiceClient, err := spicedbx.NewClient(cfg.SpiceDB, cfg.Tracing.Enabled)
	if err != nil {
		logger.Fatalw("unable to initialize spicedb client", "error", err)
	}

	var policy iapl.Policy

	if cfg.SpiceDB.PolicyFile != "" {
		policy, err = iapl.NewPolicyFromFile(cfg.SpiceDB.PolicyFile)
		if err != nil {
			logger.Fatalw("unable to load new policy from schema file", "policy_file", cfg.SpiceDB.PolicyFile, "error", err)
		}
	} else {
		logger.Warn("no spicedb policy file defined, using default policy")

		policy = iapl.DefaultPolicy()
	}

	if err = policy.Validate(); err != nil {
		logger.Fatalw("invalid spicedb policy", "error", err)
	}

	engine := query.NewEngine("infratographer", spiceClient, query.WithPolicy(policy), query.WithLogger(logger))

	events, err := events.NewConnection(cfg.Events.Config, events.WithLogger(logger))
	if err != nil {
		logger.Fatalw("failed to initialize events", "error", err)
	}

	subscriber, err := pubsub.NewSubscriber(ctx, events, engine,
		pubsub.WithLogger(logger),
	)
	if err != nil {
		logger.Fatalw("unable to initialize subscriber", "error", err)
	}

	topics := cfg.Events.Topics

	// if no topics are defined, add all topics from the schema.
	if len(topics) == 0 {
		schema := policy.Schema()

		for _, rt := range schema {
			topics = append(topics, "*."+rt.Name)
		}
	}

	for _, topic := range topics {
		if err := subscriber.Subscribe(topic); err != nil {
			logger.Fatalw("failed to subscribe to changes topic", "topic", topic, "error", err)
		}
	}

	srv, err := echox.NewServer(logger.Desugar(), cfg.Server, versionx.BuildDetails())
	if err != nil {
		logger.Fatal("failed to initialize new server", zap.Error(err))
	}

	srv.AddReadinessCheck("spicedb", spicedbx.Healthcheck(spiceClient))

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("Listening for events")

		if err := subscriber.Listen(); err != nil {
			logger.Fatalw("error listening for events", "error", err)
		}
	}()

	go func() {
		if err := srv.Run(); err != nil {
			logger.Fatal("failed to run server", zap.Error(err))
		}
	}()

	var cancel func()

	select {
	case <-quit:
		logger.Info("signal caught, shutting down")

		ctx, cancel = context.WithTimeout(ctx, shutdownTimeout)
	case <-ctx.Done():
		logger.Info("context done, shutting down")

		ctx, cancel = context.WithTimeout(context.Background(), shutdownTimeout)
	}

	defer cancel()

	if err := events.Shutdown(ctx); err != nil {
		logger.Fatalw("failed to shutdown events gracefully", "error", "err")
	}
}
