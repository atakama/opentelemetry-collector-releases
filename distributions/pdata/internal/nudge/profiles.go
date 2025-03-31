// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge // import "go.opentelemetry.io/collector/pdata/internal/nudge"

import (
	nudgeprofiles "go.opentelemetry.io/collector/pdata/internal/data/protogen/profiles/v1development"
)

// MigrateProfiles implements any translation needed due to deprecation in NUDGE profiles protocol.
// Any pprofile.Unmarshaler implementation from NUDGE (proto/json) MUST call this, and the gRPC Server implementation.
func MigrateProfiles(_ []*nudgeprofiles.ResourceProfiles) {}
