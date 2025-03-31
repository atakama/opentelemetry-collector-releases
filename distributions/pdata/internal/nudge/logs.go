// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge // import "go.opentelemetry.io/collector/pdata/internal/nudge"

import (
	nudgelogs "go.opentelemetry.io/collector/pdata/internal/data/protogen/logs/v1"
)

// MigrateLogs implements any translation needed due to deprecation in NUDGE logs protocol.
// Any plog.Unmarshaler implementation from NUDGE (proto/json) MUST call this, and the gRPC Server implementation.
func MigrateLogs(rls []*nudgelogs.ResourceLogs) {
	for _, rl := range rls {
		if len(rl.ScopeLogs) == 0 {
			rl.ScopeLogs = rl.DeprecatedScopeLogs
		}
		rl.DeprecatedScopeLogs = nil
	}
}
