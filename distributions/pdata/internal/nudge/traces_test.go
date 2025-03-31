// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudge

import (
	"testing"

	"github.com/stretchr/testify/assert"

	nudgetrace "go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1"
)

func TestDeprecatedScopeSpans(t *testing.T) {
	ss := new(nudgetrace.ScopeSpans)
	rss := []*nudgetrace.ResourceSpans{
		{
			ScopeSpans:           []*nudgetrace.ScopeSpans{ss},
			DeprecatedScopeSpans: []*nudgetrace.ScopeSpans{ss},
		},
		{
			ScopeSpans:           []*nudgetrace.ScopeSpans{},
			DeprecatedScopeSpans: []*nudgetrace.ScopeSpans{ss},
		},
	}

	MigrateTraces(rss)
	assert.Same(t, ss, rss[0].ScopeSpans[0])
	assert.Same(t, ss, rss[1].ScopeSpans[0])
	assert.Nil(t, rss[0].DeprecatedScopeSpans)
	assert.Nil(t, rss[0].DeprecatedScopeSpans)
}
