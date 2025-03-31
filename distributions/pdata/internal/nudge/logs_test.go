// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge

import (
	"testing"

	"github.com/stretchr/testify/assert"

	nudgelogs "go.opentelemetry.io/collector/pdata/internal/data/protogen/logs/v1"
)

func TestDeprecatedScopeLogs(t *testing.T) {
	sl := new(nudgelogs.ScopeLogs)
	rls := []*nudgelogs.ResourceLogs{
		{
			ScopeLogs:           []*nudgelogs.ScopeLogs{sl},
			DeprecatedScopeLogs: []*nudgelogs.ScopeLogs{sl},
		},
		{
			ScopeLogs:           []*nudgelogs.ScopeLogs{},
			DeprecatedScopeLogs: []*nudgelogs.ScopeLogs{sl},
		},
	}

	MigrateLogs(rls)
	assert.Same(t, sl, rls[0].ScopeLogs[0])
	assert.Same(t, sl, rls[1].ScopeLogs[0])
	assert.Nil(t, rls[0].DeprecatedScopeLogs)
	assert.Nil(t, rls[0].DeprecatedScopeLogs)
}
