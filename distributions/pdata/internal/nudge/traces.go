// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge // import "go.opentelemetry.io/collector/pdata/internal/otlp"

import (
	nudgetrace "go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1"
)

// MigrateTraces implements any translation needed due to deprecation in NUDGE traces protocol.
// Any ptrace.Unmarshaler implementation from NUDGE (proto/json) MUST call this, and the gRPC Server implementation.
func MigrateTraces(rss []*nudgetrace.ResourceSpans) {
	for _, rs := range rss {
		if len(rs.ScopeSpans) == 0 {
			rs.ScopeSpans = rs.DeprecatedScopeSpans
		}
		rs.DeprecatedScopeSpans = nil
	}
}
