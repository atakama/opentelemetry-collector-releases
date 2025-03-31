module go.opentelemetry.io/collector/exporter/nudgehttpexporter

go 1.23.2

require (
	github.com/fatih/structs v1.1.0
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/stretchr/testify v1.10.0
	go.opentelemetry.io/collector v0.121.0
	go.opentelemetry.io/collector/component v1.27.0
	go.opentelemetry.io/collector/component/componenttest v0.121.0
	go.opentelemetry.io/collector/config/configcompression v1.27.0
	go.opentelemetry.io/collector/config/confighttp v0.121.0
	go.opentelemetry.io/collector/config/configopaque v1.27.0
	go.opentelemetry.io/collector/config/configretry v1.27.0
	go.opentelemetry.io/collector/config/configtls v1.27.0
	go.opentelemetry.io/collector/confmap v1.27.0
	go.opentelemetry.io/collector/confmap/xconfmap v0.121.0
	go.opentelemetry.io/collector/consumer v1.27.0
	go.opentelemetry.io/collector/consumer/consumererror v0.121.0
	go.opentelemetry.io/collector/exporter v0.121.0
	go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper v0.121.0
	go.opentelemetry.io/collector/exporter/exportertest v0.121.0
	go.opentelemetry.io/collector/exporter/xexporter v0.121.0
	go.opentelemetry.io/collector/pdata v1.27.0
	go.opentelemetry.io/collector/pdata/pprofile v0.121.0
	go.uber.org/goleak v1.3.0
	go.uber.org/zap v1.27.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250115164207-1a7da9e5054f
	google.golang.org/grpc v1.71.0
	google.golang.org/protobuf v1.36.5
	mySystem v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/collector/pdata/plog/plognudge v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/collector/pdata/ptrace/ptracenudge v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/collector/pdata/pprofile/pprofilenudge v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/collector/pdata/pmetric/pmetricnudge v0.0.0-00010101000000-000000000000
)

require (
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/knadh/koanf/maps v0.1.1 // indirect
	github.com/knadh/koanf/providers/confmap v0.1.0 // indirect
	github.com/knadh/koanf/v2 v2.1.2 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/rs/cors v1.11.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/collector/client v1.27.0 // indirect
	go.opentelemetry.io/collector/config/configauth v0.121.0 // indirect
	go.opentelemetry.io/collector/consumer/consumererror/xconsumererror v0.121.0 // indirect
	go.opentelemetry.io/collector/consumer/consumertest v0.121.0 // indirect
	go.opentelemetry.io/collector/consumer/xconsumer v0.121.0 // indirect
	go.opentelemetry.io/collector/extension v1.27.0 // indirect
	go.opentelemetry.io/collector/extension/extensionauth v0.121.0 // indirect
	go.opentelemetry.io/collector/extension/xextension v0.121.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.27.0 // indirect
	go.opentelemetry.io/collector/pipeline v0.121.0 // indirect
	go.opentelemetry.io/collector/pipeline/xpipeline v0.121.0 // indirect
	go.opentelemetry.io/collector/receiver v0.121.0 // indirect
	go.opentelemetry.io/collector/receiver/receivertest v0.121.0 // indirect
	go.opentelemetry.io/collector/receiver/xreceiver v0.121.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)


retract (
	v0.76.0 // Depends on retracted pdata v1.0.0-rc10 module, use v0.76.1
	v0.69.0 // Release failed, use v0.69.1
)

replace mySystem => C:/Users/frederic1/Documents/DEVELOPPEMENT_GOLANG_BIBLIOTHEQUE/mySystem

replace go.opentelemetry.io/collector/pdata/plog/plognudge => C:\opentelemetry-collector-releases\distributions\pdata\plog\plognudge

replace go.opentelemetry.io/collector/pdata/ptrace/ptracenudge => C:\opentelemetry-collector-releases\distributions\pdata\ptrace\ptracenudge

replace go.opentelemetry.io/collector/pdata/pprofile/pprofilenudge => C:\opentelemetry-collector-releases\distributions\pdata\pprofile\pprofilenudge

replace go.opentelemetry.io/collector/pdata/pmetric/pmetricnudge => C:\opentelemetry-collector-releases\distributions\pdata\pmetric\pmetricnudge
