// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package pmetricnudge // import "go.opentelemetry.io/collector/pdata/pmetric/pmetricnudge"

import (
	"testing"
)

func FuzzRequestUnmarshalJSON(f *testing.F) {
	f.Fuzz(func(_ *testing.T, data []byte) {
		er := NewExportRequest()
		_ = er.UnmarshalJSON(data)
	})
}

func FuzzResponseUnmarshalJSON(f *testing.F) {
	f.Fuzz(func(_ *testing.T, data []byte) {
		er := NewExportResponse()
		_ = er.UnmarshalJSON(data)
	})
}

func FuzzRequestUnmarshalProto(f *testing.F) {
	f.Fuzz(func(_ *testing.T, data []byte) {
		er := NewExportRequest()
		_ = er.UnmarshalJSON(data)
	})
}

func FuzzResponseUnmarshalProto(f *testing.F) {
	f.Fuzz(func(_ *testing.T, data []byte) {
		er := NewExportResponse()
		_ = er.UnmarshalJSON(data)
	})
}
