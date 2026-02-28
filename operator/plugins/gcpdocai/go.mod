module github.com/danielnyari/flokoa/operator/plugins/gcpdocai

go 1.24.10

require (
	cloud.google.com/go/documentai v1.35.0
	cloud.google.com/go/longrunning v0.6.4
	github.com/argoproj/argo-workflows/v3 v3.7.9
	github.com/danielnyari/flokoa v0.0.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0
	go.opentelemetry.io/otel v1.36.0
	go.opentelemetry.io/otel/trace v1.36.0
	google.golang.org/api v0.228.0
	google.golang.org/protobuf v1.36.11
	k8s.io/apimachinery v0.33.1
	k8s.io/client-go v0.33.1
	sigs.k8s.io/controller-runtime v0.21.0
)

replace github.com/danielnyari/flokoa => ../..
