// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge

import (
	"testing"

	"github.com/stretchr/testify/assert"

	nudgemetrics "go.opentelemetry.io/collector/pdata/internal/data/protogen/metrics/v1"
)

func TestDeprecatedScopeMetrics(t *testing.T) {
	sm := new(nudgemetrics.ScopeMetrics)
	rms := []*nudgemetrics.ResourceMetrics{
		{
			ScopeMetrics:           []*nudgemetrics.ScopeMetrics{sm},
			DeprecatedScopeMetrics: []*nudgemetrics.ScopeMetrics{sm},
		},
		{
			ScopeMetrics:           []*nudgemetrics.ScopeMetrics{},
			DeprecatedScopeMetrics: []*nudgemetrics.ScopeMetrics{sm},
		},
	}

	MigrateMetrics(rms)
	assert.Same(t, sm, rms[0].ScopeMetrics[0])
	assert.Same(t, sm, rms[1].ScopeMetrics[0])
	assert.Nil(t, rms[0].DeprecatedScopeMetrics)
	assert.Nil(t, rms[0].DeprecatedScopeMetrics)
}
