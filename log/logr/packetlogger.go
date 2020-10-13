package logr

import (
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// WithLogLevel sets the log level
func WithLogLevel(level string) LoggerOption {
	return func(args *PacketLogr) { args.logLevel = level }
}

// WithOutputPaths adds output paths
func WithOutputPaths(paths []string) LoggerOption {
	return func(args *PacketLogr) { args.outputPaths = paths }
}

// WithServiceName adds a service name a logged field
func WithServiceName(name string) LoggerOption {
	return func(args *PacketLogr) { args.serviceName = name }
}

// WithKeysAndValues adds extra key/value fields
func WithKeysAndValues(kvs []interface{}) LoggerOption {
	return func(args *PacketLogr) { args.keysAndValues = append(args.keysAndValues, kvs...) }
}

// WithEnableErrLogsToStderr sends .Error logs to stderr
func WithEnableErrLogsToStderr(enable bool) LoggerOption {
	return func(args *PacketLogr) { args.enableErrLogsToStderr = enable }
}

// WithEnableRollbar sends error logs to Rollbar service
func WithEnableRollbar(enable bool) LoggerOption {
	return func(args *PacketLogr) { args.enableRollbar = enable }
}

// WithRollbarConfig customizes the Rollbar details
func WithRollbarConfig(config rollbarConfig) LoggerOption {
	return func(args *PacketLogr) { args.rollbarConfig = config }
}

// PacketLogr is a wrapper around zap.SugaredLogger
type PacketLogr struct {
	logr.Logger
	logLevel              string
	outputPaths           []string
	serviceName           string
	keysAndValues         []interface{}
	enableErrLogsToStderr bool
	enableRollbar         bool
	rollbarConfig         rollbarConfig
}

// LoggerOption for setting optional values
type LoggerOption func(*PacketLogr)

// NewPacketLogr is the opionated packet logger setup
func NewPacketLogr(opts ...LoggerOption) (logr.Logger, *zap.Logger, error) {
	// defaults
	const (
		defaultLogLevel    = "info"
		defaultServiceName = "not/set"
	)
	var (
		defaultOutputPaths   = []string{"stdout"}
		defaultKeysAndValues = []interface{}{}
		zapConfig            = zap.NewProductionConfig()
		zLevel               = zap.InfoLevel
		defaultZapOpts       = []zap.Option{}
		rollbarOptions       zap.Option
		defaultRollbarConfig = rollbarConfig{
			token:   "123",
			env:     "production",
			version: "1",
		}
	)

	pl := &PacketLogr{
		Logger:        nil,
		logLevel:      defaultLogLevel,
		outputPaths:   defaultOutputPaths,
		serviceName:   defaultServiceName,
		keysAndValues: defaultKeysAndValues,
		enableRollbar: false,
		rollbarConfig: defaultRollbarConfig,
	}

	for _, opt := range opts {
		opt(pl)
	}

	switch pl.logLevel {
	case "debug":
		zLevel = zap.DebugLevel
	}
	zapConfig.Level = zap.NewAtomicLevelAt(zLevel)
	zapConfig.OutputPaths = sliceDedupe(pl.outputPaths)

	if pl.enableErrLogsToStderr {
		defaultZapOpts = append(defaultZapOpts, errLogsToStderr(zapConfig))
	}

	zapLogger, err := zapConfig.Build(defaultZapOpts...)
	if err != nil {
		return pl, zapLogger, errors.Wrap(err, "failed to build logger config")
	}
	if pl.enableRollbar {
		rollbarOptions = pl.rollbarConfig.setupRollbar(pl.serviceName, zapLogger)
		zapLogger = zapLogger.WithOptions(rollbarOptions)
	}
	pl.Logger = zapr.NewLogger(zapLogger)
	keysAndValues := append(pl.keysAndValues, "service", pl.serviceName)
	pl.Logger = pl.WithValues(keysAndValues...)
	return pl, zapLogger, err
}

func sliceDedupe(elements []string) []string {
	encountered := map[string]bool{}
	result := []string{}

	for v := range elements {
		if encountered[elements[v]] {
		} else {
			encountered[elements[v]] = true
			result = append(result, elements[v])
		}
	}
	return result
}

func errLogsToStderr(c zap.Config) zap.Option {
	errorLogs := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})
	nonErrorLogs := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return !errorLogs(lvl)
	})
	console := zapcore.Lock(os.Stdout)
	consoleErrors := zapcore.Lock(os.Stderr)
	encoder := zapcore.NewJSONEncoder(c.EncoderConfig)

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, console, nonErrorLogs),
		zapcore.NewCore(encoder, consoleErrors, errorLogs),
	)
	splitLogger := zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return core

	})
	return splitLogger
}