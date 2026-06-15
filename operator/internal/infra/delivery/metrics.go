package delivery

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// The operator's first custom metrics, registered on the controller-runtime
// registry so they ride the existing secured metrics endpoint.
var (
	deliveryModeGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "flokoa_capability_delivery_mode",
		Help: "Effective capability delivery mode for this operator process: 1 for the active mode label, absent otherwise.",
	}, []string{"mode"})

	imageVolumeSupportedGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "flokoa_capability_imagevolume_supported",
		Help: "Whether the startup probe found ImageVolume delivery supported (1) or not (0). Only exported when a probe ran (auto mode).",
	})
)

func init() {
	metrics.Registry.MustRegister(deliveryModeGauge)
}

// recordMetrics exports the resolution. The supported gauge is registered
// lazily: a registered-but-unset gauge exports 0, which would read as a
// probed "unsupported" verdict when no probe ran at all.
func recordMetrics(result Result) {
	deliveryModeGauge.Reset()
	deliveryModeGauge.WithLabelValues(string(result.EffectiveMode)).Set(1)

	if result.ImageVolumeSupported == nil {
		return
	}
	if err := metrics.Registry.Register(imageVolumeSupportedGauge); err != nil {
		are := prometheus.AlreadyRegisteredError{}
		if !errors.As(err, &are) {
			panic(err) // a fresh gauge can only collide with itself
		}
	}
	if *result.ImageVolumeSupported {
		imageVolumeSupportedGauge.Set(1)
	} else {
		imageVolumeSupportedGauge.Set(0)
	}
}
