// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/integration/util.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
//go:build requires_docker

package integration

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/grafana/e2e"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"

	"github.com/grafana/mimir/pkg/util/test"
)

var (
	// Expose some utilities from the framework so that we don't have to prefix them
	// with the package name in tests.
	mergeFlags = e2e.MergeFlags

	generateFloatSeries  = e2e.GenerateSeries
	generateNFloatSeries = e2e.GenerateNSeries

	// These are local, because e2e is used by non metric products that do not have native histograms
	generateHistogramSeries         = GenerateHistogramSeries
	generateNHistogramSeries        = GenerateNHistogramSeries
	generateTestHistogram           = test.GenerateTestHistogram
	generateTestFloatHistogram      = test.GenerateTestFloatHistogram
	generateTestGaugeHistogram      = test.GenerateTestGaugeHistogram
	generateTestGaugeFloatHistogram = test.GenerateTestGaugeFloatHistogram
	generateTestSampleHistogram     = test.GenerateTestSampleHistogram

	// These are the earliest and latest possible timestamps supported by the Prometheus API -
	// the Prometheus API does not support omitting a time range from query requests,
	// so we use these when we want to query over all time.
	// These values are defined in github.com/prometheus/prometheus/web/api/v1/api.go but
	// sadly not exported.
	prometheusMinTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()
	prometheusMaxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999).UTC()
)

// generateSeriesFunc defines what kind of series (and expected vectors/matrices) to generate - float samples or native histograms
type generateSeriesFunc func(name string, ts time.Time, additionalLabels ...prompb.Label) (series []prompb.TimeSeries, vector model.Vector, matrix model.Matrix)

// Generates different typed series based on an index in i.
// Use with a large enough number of series, e.g. i>100
func generateAlternatingSeries(i int) generateSeriesFunc {
	switch i % 5 {
	case 0:
		return generateFloatSeries
	case 1:
		return generateHistogramSeries
	case 2:
		return GenerateFloatHistogramSeries
	case 3:
		return GenerateGaugeHistogramSeries
	case 4:
		return GenerateGaugeFloatHistogramSeries
	default:
		return nil
	}
}

// generateNSeriesFunc defines what kind of n * series (and expected vectors) to generate - float samples or native histograms
type generateNSeriesFunc func(nSeries, nExemplars int, name func() string, ts time.Time, additionalLabels func() []prompb.Label) (series []prompb.TimeSeries, vector model.Vector)

func getMimirProjectDir() string {
	if dir := os.Getenv("MIMIR_CHECKOUT_DIR"); dir != "" {
		return dir
	}

	// use the git path if available
	dir, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return string(bytes.TrimSpace(dir))
	}

	return os.Getenv("GOPATH") + "/src/github.com/grafana/mimir"
}

func writeFileToSharedDir(s *e2e.Scenario, dst string, content []byte) error {
	dst = filepath.Join(s.SharedDir(), dst)

	// Ensure the entire path of directories exist.
	if err := os.MkdirAll(filepath.Dir(dst), os.ModePerm); err != nil {
		return err
	}

	return os.WriteFile(
		dst,
		content,
		os.ModePerm)
}

func copyFileToSharedDir(s *e2e.Scenario, src, dst string) error {
	content, err := os.ReadFile(filepath.Join(getMimirProjectDir(), src))
	if err != nil {
		return errors.Wrapf(err, "unable to read local file %s", src)
	}

	return writeFileToSharedDir(s, dst, content)
}

func getServerTLSFlags() map[string]string {
	return map[string]string{
		"-server.grpc-tls-cert-path":   filepath.Join(e2e.ContainerSharedDir, serverCertFile),
		"-server.grpc-tls-key-path":    filepath.Join(e2e.ContainerSharedDir, serverKeyFile),
		"-server.grpc-tls-client-auth": "RequireAndVerifyClientCert",
		"-server.grpc-tls-ca-path":     filepath.Join(e2e.ContainerSharedDir, caCertFile),
	}
}

func getServerHTTPTLSFlags() map[string]string {
	return map[string]string{
		"-server.http-tls-cert-path":   filepath.Join(e2e.ContainerSharedDir, serverCertFile),
		"-server.http-tls-key-path":    filepath.Join(e2e.ContainerSharedDir, serverKeyFile),
		"-server.http-tls-client-auth": "RequireAndVerifyClientCert",
		"-server.http-tls-ca-path":     filepath.Join(e2e.ContainerSharedDir, caCertFile),
	}
}

func getClientTLSFlagsWithPrefix(prefix string) map[string]string {
	return getTLSFlagsWithPrefix(prefix, "ingester.client", false)
}

func getTLSFlagsWithPrefix(prefix string, servername string, http bool) map[string]string {
	flags := map[string]string{
		"-" + prefix + ".tls-cert-path":   filepath.Join(e2e.ContainerSharedDir, clientCertFile),
		"-" + prefix + ".tls-key-path":    filepath.Join(e2e.ContainerSharedDir, clientKeyFile),
		"-" + prefix + ".tls-ca-path":     filepath.Join(e2e.ContainerSharedDir, caCertFile),
		"-" + prefix + ".tls-server-name": servername,
	}

	if !http {
		flags["-"+prefix+".tls-enabled"] = "true"
	}

	return flags
}

// generateHistogramFunc defines what kind of native histograms to generate: float/integer, counter/gauge
type generateHistogramFunc func(tsMillis int64, value int) prompb.Histogram

func GenerateHistogramSeries(name string, ts time.Time, additionalLabels ...prompb.Label) (series []prompb.TimeSeries, vector model.Vector, matrix model.Matrix) {
	return generateHistogramSeriesWrapper(func(tsMillis int64, value int) prompb.Histogram {
		return remote.HistogramToHistogramProto(tsMillis, generateTestHistogram(value))
	}, name, ts, additionalLabels...)
}

func GenerateFloatHistogramSeries(name string, ts time.Time, additionalLabels ...prompb.Label) (series []prompb.TimeSeries, vector model.Vector, matrix model.Matrix) {
	return generateHistogramSeriesWrapper(func(tsMillis int64, value int) prompb.Histogram {
		return remote.FloatHistogramToHistogramProto(tsMillis, generateTestFloatHistogram(value))
	}, name, ts, additionalLabels...)
}

func GenerateGaugeHistogramSeries(name string, ts time.Time, additionalLabels ...prompb.Label) (series []prompb.TimeSeries, vector model.Vector, matrix model.Matrix) {
	return generateHistogramSeriesWrapper(func(tsMillis int64, value int) prompb.Histogram {
		return remote.HistogramToHistogramProto(tsMillis, generateTestGaugeHistogram(value))
	}, name, ts, additionalLabels...)
}

func GenerateGaugeFloatHistogramSeries(name string, ts time.Time, additionalLabels ...prompb.Label) (series []prompb.TimeSeries, vector model.Vector, matrix model.Matrix) {
	return generateHistogramSeriesWrapper(func(tsMillis int64, value int) prompb.Histogram {
		return remote.FloatHistogramToHistogramProto(tsMillis, generateTestGaugeFloatHistogram(value))
	}, name, ts, additionalLabels...)
}

func generateHistogramSeriesWrapper(generateHistogram generateHistogramFunc, name string, ts time.Time, additionalLabels ...prompb.Label) (series []prompb.TimeSeries, vector model.Vector, matrix model.Matrix) {
	tsMillis := e2e.TimeToMilliseconds(ts)

	value := rand.Intn(1000)

	lbls := append(
		[]prompb.Label{
			{Name: labels.MetricName, Value: name},
		},
		additionalLabels...,
	)

	// Generate the series
	series = append(series, prompb.TimeSeries{
		Labels: lbls,
		Exemplars: []prompb.Exemplar{
			{Value: float64(value), Timestamp: tsMillis, Labels: []prompb.Label{
				{Name: "trace_id", Value: "1234"},
			}},
		},
		Histograms: []prompb.Histogram{generateHistogram(tsMillis, value)},
	})

	// Generate the expected vector and matrix when querying it
	metric := model.Metric{}
	metric[labels.MetricName] = model.LabelValue(name)
	for _, lbl := range additionalLabels {
		metric[model.LabelName(lbl.Name)] = model.LabelValue(lbl.Value)
	}

	vector = append(vector, &model.Sample{
		Metric:    metric,
		Timestamp: model.Time(tsMillis),
		Histogram: generateTestSampleHistogram(value),
	})

	matrix = append(matrix, &model.SampleStream{
		Metric: metric,
		Histograms: []model.SampleHistogramPair{
			{
				Timestamp: model.Time(tsMillis),
				Histogram: generateTestSampleHistogram(value),
			},
		},
	})

	return
}

func GenerateNHistogramSeries(nSeries, nExemplars int, name func() string, ts time.Time, additionalLabels func() []prompb.Label) (series []prompb.TimeSeries, vector model.Vector) {
	tsMillis := e2e.TimeToMilliseconds(ts)

	// Generate the series
	for i := 0; i < nSeries; i++ {
		lbls := []prompb.Label{
			{Name: labels.MetricName, Value: name()},
		}
		if additionalLabels != nil {
			lbls = append(lbls, additionalLabels()...)
		}

		exemplars := []prompb.Exemplar{}
		if i < nExemplars {
			exemplars = []prompb.Exemplar{
				{Value: float64(i), Timestamp: tsMillis, Labels: []prompb.Label{{Name: "trace_id", Value: "1234"}}},
			}
		}

		series = append(series, prompb.TimeSeries{
			Labels:     lbls,
			Histograms: []prompb.Histogram{remote.HistogramToHistogramProto(tsMillis, generateTestHistogram(i))},
			Exemplars:  exemplars,
		})
	}

	// Generate the expected vector when querying it
	for i := 0; i < nSeries; i++ {
		metric := model.Metric{}
		for _, lbl := range series[i].Labels {
			metric[model.LabelName(lbl.Name)] = model.LabelValue(lbl.Value)
		}

		vector = append(vector, &model.Sample{
			Metric:    metric,
			Timestamp: model.Time(tsMillis),
			Histogram: generateTestSampleHistogram(i),
		})
	}
	return
}
