module github.com/open-telemetry/opentelemetry-collector-contrib/receiver/wavefrontreceiver

go 1.15

require (
	github.com/census-instrumentation/opencensus-proto v0.3.0
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/carbonreceiver v0.0.0-00010101000000-000000000000
	github.com/open-telemetry/opentelemetry-collector-contrib/receiver/collectdreceiver v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/collector v0.25.1-0.20210430165557-1eb6d1a7b03c
	go.uber.org/zap v1.16.0
	google.golang.org/protobuf v1.26.0
)

replace github.com/open-telemetry/opentelemetry-collector-contrib/receiver/collectdreceiver => ../collectdreceiver

replace github.com/open-telemetry/opentelemetry-collector-contrib/receiver/carbonreceiver => ../carbonreceiver
