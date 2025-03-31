// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package nudgehttpexporter // import "go.opentelemetry.io/collector/exporter/nudgehttpexporter"

import (
	"encoding"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// EncodingType defines the type for content encoding
type EncodingType string

const (
	EncodingProto EncodingType = "proto"
	EncodingJSON  EncodingType = "json"
)

var _ encoding.TextUnmarshaler = (*EncodingType)(nil)

// UnmarshalText unmarshalls text to an EncodingType.
func (e *EncodingType) UnmarshalText(text []byte) error {
	if e == nil {
		return errors.New("cannot unmarshal to a nil *EncodingType")
	}

	str := string(text)
	switch str {
	case string(EncodingProto):
		*e = EncodingProto
	case string(EncodingJSON):
		*e = EncodingJSON
	default:
		return fmt.Errorf("invalid encoding type: %s", str)
	}

	return nil
}

type NodeJS struct {
	ApplicationName    string `mapstructure:"applicationName"`
	PrefixeServiceName string `mapstructure:"prefixeServiceName"`
	UnknownClass       string `mapstructure:"unknownClass"`
	UnknownFunction    string `mapstructure:"unknownFunction"`
}

// NudgeHTTPClientConfig étend confighttp.ClientConfig avec des champs personnalisés.
type NudgeHTTPClientConfig struct {
	confighttp.ClientConfig `mapstructure:",squash"`
	AppID                   string `mapstructure:"app_id"`
	PathCollect             string `mapstructure:"pathCollect"`
	Method                  string `mapstructure:"method"`
	Log                     bool   `mapstructure:"log"`
	NodeJS                  NodeJS `mapstructure:"nodeJS"`
}

func NewDefaultNudgeHTTPClientConfig() NudgeHTTPClientConfig {
	// The default values are taken from the values of 'DefaultTransport' of 'http' package.
	//defaultTransport := http.DefaultTransport.(*http.Transport)

	x := NudgeHTTPClientConfig{
		ClientConfig: confighttp.NewDefaultClientConfig(),
	}

	return x
}

// Config defines configuration for NUDGE/HTTP exporter.
type Config struct {
	NudgeHTTPClientConfig      `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct.
	exporterhelper.QueueConfig `mapstructure:"sending_queue"`
	RetryConfig                configretry.BackOffConfig `mapstructure:"retry_on_failure"`

	// The URL to send traces to. If omitted the Endpoint + "/v1/traces" will be used.
	TracesEndpoint string `mapstructure:"traces_endpoint"`

	// The URL to send metrics to. If omitted the Endpoint + "/v1/metrics" will be used.
	MetricsEndpoint string `mapstructure:"metrics_endpoint"`

	// The URL to send logs to. If omitted the Endpoint + "/v1/logs" will be used.
	LogsEndpoint string `mapstructure:"logs_endpoint"`

	// The encoding to export telemetry (default: "proto")
	Encoding EncodingType `mapstructure:"encoding"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	if cfg.Endpoint == "" && cfg.TracesEndpoint == "" && cfg.MetricsEndpoint == "" && cfg.LogsEndpoint == "" {
		return errors.New("at least one endpoint must be specified")
	}
	return nil
}
