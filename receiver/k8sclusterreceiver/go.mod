module github.com/open-telemetry/opentelemetry-collector-contrib/receiver/k8sclusterreceiver

go 1.15

require (
	github.com/census-instrumentation/opencensus-proto v0.3.0
	github.com/iancoleman/strcase v0.1.3
	github.com/onsi/ginkgo v1.14.1 // indirect
	github.com/onsi/gomega v1.10.2 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/common v0.0.0-00010101000000-000000000000
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sconfig v0.0.0-00010101000000-000000000000
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/experimentalmetricmetadata v0.0.0-00010101000000-000000000000
	github.com/pelletier/go-toml v1.8.0 // indirect
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/collector v0.25.1-0.20210430165557-1eb6d1a7b03c
	go.uber.org/atomic v1.7.0
	go.uber.org/zap v1.16.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/ini.v1 v1.57.0 // indirect
	k8s.io/api v0.20.5
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v0.20.5
)

replace github.com/open-telemetry/opentelemetry-collector-contrib/internal/common => ../../internal/common

replace github.com/open-telemetry/opentelemetry-collector-contrib/internal/k8sconfig => ../../internal/k8sconfig

replace github.com/open-telemetry/opentelemetry-collector-contrib/pkg/experimentalmetricmetadata => ../../pkg/experimentalmetricmetadata
