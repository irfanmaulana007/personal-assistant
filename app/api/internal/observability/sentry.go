// Package observability wires the server into Sentry for error and performance
// monitoring. Initialization is best-effort: when no DSN is configured (the
// default for local development) the SDK stays disabled and every helper here
// degrades to a harmless no-op, so the application runs unchanged.
package observability

import (
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"
)

// InitSentry initializes the global Sentry client from the given settings and
// returns a flush function to call on shutdown (safe to call even when Sentry
// is disabled). It never returns an error for a missing DSN — that simply
// disables reporting — so callers can defer the flush unconditionally.
//
//	dsn              the project DSN; empty disables Sentry entirely.
//	environment      the deployment name ("local" / "production", …) — becomes
//	                 the Sentry environment so issues can be filtered by it.
//	release          an optional release identifier (e.g. the app version) used
//	                 to attribute issues to a version; empty leaves it unset.
//	tracesSampleRate fraction of transactions sampled for performance tracing
//	                 (0 disables tracing; errors are always captured).
func InitSentry(dsn, environment, release string, tracesSampleRate float64, debug bool, log *slog.Logger) func() {
	if dsn == "" {
		log.Info("sentry disabled (no DSN configured)")
		return func() {}
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      environment,
		Release:          release,
		EnableTracing:    tracesSampleRate > 0,
		TracesSampleRate: tracesSampleRate,
		Debug:            debug,
		// Attach a stack trace to plain errors captured via CaptureException so
		// server-side issues are actionable without a panic.
		AttachStacktrace: true,
	})
	if err != nil {
		// A bad DSN or option shouldn't take the server down — log and continue
		// with reporting disabled.
		log.Error("sentry init failed; continuing without it", "error", err)
		return func() {}
	}

	log.Info("sentry enabled", "environment", environment, "release", release, "traces_sample_rate", tracesSampleRate)
	return func() { sentry.Flush(2 * time.Second) }
}

// Recover reports a panic to Sentry and re-raises it. Use it as a deferred guard
// in background goroutines (schedulers, the WhatsApp message handler) whose
// panics would otherwise crash the process silently:
//
//	defer observability.Recover()
//
// It is a no-op when Sentry is disabled beyond flushing, so it is always safe to
// defer.
func Recover() {
	if err := recover(); err != nil {
		sentry.CurrentHub().Recover(err)
		sentry.Flush(2 * time.Second)
		panic(err)
	}
}

// CaptureError reports a handled (non-panic) error to Sentry, tagging it with
// the given key/value pairs so issues can be grouped and searched (e.g. the
// platform or capability that produced it). It is a no-op when Sentry is
// disabled. nil errors are ignored.
func CaptureError(err error, tags map[string]string) {
	if err == nil {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureException(err)
	})
}
