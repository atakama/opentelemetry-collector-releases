module go.opentelemetry.io/collector/exporter/nudgehttpexporter

go 1.24.2

require (
	extensionlinux v0.0.0-00010101000000-000000000000
	github.com/fatih/structs v1.1.0
	github.com/google/uuid v1.6.0
	github.com/jackpal/gateway v1.1.1
	github.com/mssola/user_agent v0.6.0
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/collector v0.124.0
	go.opentelemetry.io/collector/component v1.30.0
	go.opentelemetry.io/collector/component/componenttest v0.124.0
	go.opentelemetry.io/collector/config/configcompression v1.30.0
	go.opentelemetry.io/collector/config/confighttp v0.124.0
	go.opentelemetry.io/collector/config/configopaque v1.30.0
	go.opentelemetry.io/collector/config/configretry v1.30.0
	go.opentelemetry.io/collector/config/configtelemetry v0.124.0
	go.opentelemetry.io/collector/config/configtls v1.30.0
	go.opentelemetry.io/collector/confmap v1.30.0
	go.opentelemetry.io/collector/confmap/xconfmap v0.124.0
	go.opentelemetry.io/collector/consumer v1.30.0
	go.opentelemetry.io/collector/consumer/consumererror v0.124.0
	go.opentelemetry.io/collector/exporter v0.124.0
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.124.0
	go.opentelemetry.io/collector/exporter/exportertest v0.124.0
	go.opentelemetry.io/collector/exporter/xexporter v0.124.0
	go.opentelemetry.io/collector/pdata v0.1.0
	go.opentelemetry.io/collector/pdata/pprofile v0.1.0
	go.opentelemetry.io/otel v1.35.0
	go.uber.org/goleak v1.3.0
	go.uber.org/zap v1.27.0
	gonum.org/v1/gonum v0.16.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250422160041-2d3770c4ea7f
	google.golang.org/grpc v1.72.0
	google.golang.org/protobuf v1.36.6
	mySystem v0.0.0-00010101000000-000000000000
)

require (
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.2.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/client v1.30.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.124.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.124.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.124.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.124.0 // indirect
	go.opentelemetry.io/collector/extension v1.30.0 // indirect
	go.opentelemetry.io/collector/extension/extensionauth v1.30.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.124.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.30.0 // indirect
	go.opentelemetry.io/collector/internal/telemetry v0.124.0 // indirect
	go.opentelemetry.io/collector/pipeline v0.124.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.124.0 // indirect
	go.opentelemetry.io/collector/receiver v1.30.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.124.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.124.0 // indirect
	go.opentelemetry.io/contrib/bridges/otelzap v0.10.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel/log v0.11.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sys v0.32.0 // indirect; indirect-
	golang.org/x/text v0.24.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	extensionlinux => C:\Users\frederic1\Documents\DEVELOPPEMENT_GOLANG_BIBLIOTHEQUE\ExtensionLinux
	mySystem => C:\Users\frederic1\Documents\DEVELOPPEMENT_GOLANG_BIBLIOTHEQUE\mySystem
	go.opentelemetry.io/collector/pdata v0.1.0 => ../../pdata
	go.opentelemetry.io/collector/pdata/pprofile v0.1.0 => ../../pdata/pprofile
)

retract (
	v0.76.0 // Depends on retracted pdata v1.0.0-rc10 module, use v0.76.1
	v0.69.0 // Release failed, use v0.69.1
)
