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
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// supportedLevels in this exporter's configuration.
// configtelemetry.LevelNone and other future values are not supported.
var supportedLevels map[configtelemetry.Level]struct{} = map[configtelemetry.Level]struct{}{
	configtelemetry.LevelBasic:    {},
	configtelemetry.LevelNormal:   {},
	configtelemetry.LevelDetailed: {},
}

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
	UnknownClass    string `mapstructure:"unknownClass"`
	UnknownFunction string `mapstructure:"unknownFunction"`
}

type ConsoleX struct {
	Verbosity configtelemetry.Level `mapstructure:"verbosity,omitempty"`
}

// NudgeHTTPClientConfig étend confighttp.ClientConfig avec des champs personnalisés.
type NudgeHTTPClientConfig struct {
	confighttp.ClientConfig `mapstructure:",squash"`
	AppID                   string   `mapstructure:"app_id" comment:"Il est possible de laisser la valeur par defaut si non définie dans la partie processor"`
	PathCollect             string   `mapstructure:"pathCollect" comment:"chemin de la requete http, par defaut /collect/rawdata"`
	Method                  string   `mapstructure:"method" comment:"methode http pour le flush des rawdatas"`
	NodeJS                  NodeJS   `mapstructure:"nodeJS" comment:"Configuration pour NodeJS"`
	Console                 ConsoleX `mapstructure:"console" comment:"Configuration pour ConsoleX"`
	// The level of telemetry to be sent to the Nudge server.
	RecordCollecte           bool     `mapstructure:"recordCollecte" comment:"Enregistrement du rawdata brut sur disque"`
	InsertResource           bool     `mapstructure:"insertResource" comment:"Insertion des métriques resources du poste ou est situé le collecteur OTLP (disque, memoire, cpu)"`
	TimeoutBufferTransaction int64    `mapstructure:"timeoutBufferTransaction" comment:"Timeout du Buffer Transaction en secondes"`
	ApplicationName          string   `mapstructure:"applicationName" comment:"Nom de l'application et du serveur, inseré dans le rawdata, par defaut APP_CCC"`
	PrefixeServiceName       string   `mapstructure:"prefixeServiceName" comment:"Prefixe du nom de service, par defaut APP_CCC"`
	FilterHeader             []string `mapstructure:"filterHeader"`
	SessionId                string   `mapstructure:"sessionId"`
	SendUserConnectionString bool     `mapstructure:"sendUserConnectionString" comment:"Envoi de la connection string de l'utilisateur"`
	RefreshScanDrives        float64  `mapstructure:"refreshScanDrives" comment:"Temps en minutes de rafraichissement des lecteurs physiques & logiques (CF : Worker.js), par defaut 10"`
	RefreshLoadCPU           float64  `mapstructure:"refreshLoadCPU" comment:"Temps refresh en secondes pour le calcul de la charge CPU, par defaut 5s"`
	WaitEndService           bool     `mapstructure:"waitEndService" comment:"Attend la fin de la trace (parentSpanId == empty) avant de faire le flush vers le rawdata, par defaut true`
	DebugPort                int      `mapstructure:"debugPort"`
	DebugPath                string   `mapstructure:"debugPath"`
	// The level of telemetry to be sent to the Nudge server.
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

	// Chemin Page personnelle de Nudge
	DebugPath string `mapstructure:"debug_path"`
	// Port Page personnelle de Nudge
	DebugPort int `mapstructure:"debug_port"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the exporter configuration is valid
func (cfg *Config) Validate() error {
	if cfg.Endpoint == "" && cfg.TracesEndpoint == "" && cfg.MetricsEndpoint == "" && cfg.LogsEndpoint == "" {
		return errors.New("at least one endpoint must be specified")
	}
	return nil
}
