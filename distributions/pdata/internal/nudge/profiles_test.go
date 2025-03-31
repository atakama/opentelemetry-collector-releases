// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge

import (
	"testing"

	nudgeprofiles "go.opentelemetry.io/collector/pdata/internal/data/protogen/profiles/v1development"
)

func TestMigrateProfiles(_ *testing.T) {
	rps := []*nudgeprofiles.ResourceProfiles{
		{},
	}

	MigrateProfiles(rps)
}
