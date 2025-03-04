// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package newrelicexporter

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/pdata"
	tracetranslator "go.opentelemetry.io/collector/translator/trace"
)

const (
	unitAttrKey               = "unit"
	descriptionAttrKey        = "description"
	collectorNameKey          = "collector.name"
	collectorVersionKey       = "collector.version"
	instrumentationNameKey    = "instrumentation.name"
	instrumentationVersionKey = "instrumentation.version"
	statusCodeKey             = "otel.status_code"
	statusDescriptionKey      = "otel.status_description"
	spanKindKey               = "span.kind"
	spanIDKey                 = "span.id"
	traceIDKey                = "trace.id"
	logSeverityTextKey        = "log.level"
	logSeverityNumKey         = "log.levelNum"
)

type transformer struct {
	OverrideAttributes map[string]interface{}
	details            *exportMetadata
}

func newTransformer(buildInfo *component.BuildInfo, details *exportMetadata) *transformer {
	overrideAttributes := make(map[string]interface{})
	if buildInfo != nil {
		overrideAttributes[collectorNameKey] = buildInfo.Command
		if buildInfo.Version != "" {
			overrideAttributes[collectorVersionKey] = buildInfo.Version
		}
	}

	return &transformer{OverrideAttributes: overrideAttributes, details: details}
}

func (t *transformer) CommonAttributes(resource pdata.Resource, lib pdata.InstrumentationLibrary) map[string]interface{} {
	resourceAttrs := resource.Attributes()
	commonAttrs := tracetranslator.AttributeMapToMap(resourceAttrs)
	t.TrackAttributes(attributeLocationResource, resourceAttrs)

	if n := lib.Name(); n != "" {
		commonAttrs[instrumentationNameKey] = n
		if v := lib.Version(); v != "" {
			commonAttrs[instrumentationVersionKey] = v
		}
	}

	for k, v := range t.OverrideAttributes {
		commonAttrs[k] = v
	}

	return commonAttrs
}

var (
	errInvalidSpanID  = errors.New("SpanID is invalid")
	errInvalidTraceID = errors.New("TraceID is invalid")
)

func (t *transformer) Span(span pdata.Span) (telemetry.Span, error) {
	startTime := span.StartTimestamp().AsTime()
	sp := telemetry.Span{
		// HexString validates the IDs, it will be an empty string if invalid.
		ID:         span.SpanID().HexString(),
		TraceID:    span.TraceID().HexString(),
		ParentID:   span.ParentSpanID().HexString(),
		Name:       span.Name(),
		Timestamp:  startTime,
		Duration:   span.EndTimestamp().AsTime().Sub(startTime),
		Attributes: t.SpanAttributes(span),
		Events:     t.SpanEvents(span),
	}

	spanMetadataKey := spanStatsKey{
		hasEvents: sp.Events != nil,
		hasLinks:  span.Links().Len() > 0,
	}
	t.details.spanMetadataCount[spanMetadataKey]++

	if sp.ID == "" {
		return sp, errInvalidSpanID
	}
	if sp.TraceID == "" {
		return sp, errInvalidTraceID
	}

	return sp, nil
}

func (t *transformer) Log(log pdata.LogRecord) (telemetry.Log, error) {
	var message string

	if bodyString := log.Body().StringVal(); bodyString != "" {
		message = bodyString
	} else {
		message = log.Name()
	}

	logAttrs := log.Attributes()
	attrs := make(map[string]interface{}, logAttrs.Len()+5)

	for k, v := range tracetranslator.AttributeMapToMap(logAttrs) {
		// Only include attribute if not an override attribute
		if _, isOverrideKey := t.OverrideAttributes[k]; !isOverrideKey {
			attrs[k] = v
		}
	}
	t.TrackAttributes(attributeLocationLog, logAttrs)

	attrs["name"] = log.Name()
	if !log.TraceID().IsEmpty() {
		attrs[traceIDKey] = log.TraceID().HexString()
	}

	if !log.SpanID().IsEmpty() {
		attrs[spanIDKey] = log.SpanID().HexString()
	}

	if log.SeverityText() != "" {
		attrs[logSeverityTextKey] = log.SeverityText()
	}

	if log.SeverityNumber() != 0 {
		attrs[logSeverityNumKey] = int32(log.SeverityNumber())
	}

	return telemetry.Log{
		Message:    message,
		Timestamp:  log.Timestamp().AsTime(),
		Attributes: attrs,
	}, nil
}

func (t *transformer) SpanAttributes(span pdata.Span) map[string]interface{} {
	spanAttrs := span.Attributes()
	length := spanAttrs.Len()

	var hasStatusCode, hasStatusDesc bool
	s := span.Status()
	if s.Code() != pdata.StatusCodeUnset {
		hasStatusCode = true
		length++
		if s.Message() != "" {
			hasStatusDesc = true
			length++
		}
	}

	validSpanKind := span.Kind() != pdata.SpanKindUNSPECIFIED
	if validSpanKind {
		length++
	}

	attrs := make(map[string]interface{}, length)

	if hasStatusCode {
		code := strings.TrimPrefix(span.Status().Code().String(), "STATUS_CODE_")
		attrs[statusCodeKey] = code
	}
	if hasStatusDesc {
		attrs[statusDescriptionKey] = span.Status().Message()
	}

	// Add span kind if it is set
	if validSpanKind {
		kind := strings.TrimPrefix(span.Kind().String(), "SPAN_KIND_")
		attrs[spanKindKey] = strings.ToLower(kind)
	}

	for k, v := range tracetranslator.AttributeMapToMap(spanAttrs) {
		// Only include attribute if not an override attribute
		if _, isOverrideKey := t.OverrideAttributes[k]; !isOverrideKey {
			attrs[k] = v
		}
	}
	t.TrackAttributes(attributeLocationSpan, spanAttrs)

	return attrs
}

// SpanEvents transforms the recorded events of span into New Relic tracing events.
func (t *transformer) SpanEvents(span pdata.Span) []telemetry.Event {
	length := span.Events().Len()
	if length == 0 {
		return nil
	}

	events := make([]telemetry.Event, length)

	for i := 0; i < length; i++ {
		event := span.Events().At(i)
		eventAttrs := event.Attributes()
		events[i] = telemetry.Event{
			EventType:  event.Name(),
			Timestamp:  event.Timestamp().AsTime(),
			Attributes: tracetranslator.AttributeMapToMap(eventAttrs),
		}
		t.TrackAttributes(attributeLocationSpanEvent, eventAttrs)
	}
	return events
}

type errUnsupportedMetricType struct {
	metricType    string
	metricName    string
	numDataPoints int
}

func (e errUnsupportedMetricType) Error() string {
	return fmt.Sprintf("unsupported metric %v (%v)", e.metricName, e.metricType)
}

func (t *transformer) Metric(m pdata.Metric) ([]telemetry.Metric, error) {
	var output []telemetry.Metric
	baseAttributes := t.BaseMetricAttributes(m)

	dataType := m.DataType()
	k := metricStatsKey{MetricType: dataType}

	switch dataType {
	case pdata.MetricDataTypeIntGauge:
		t.details.metricMetadataCount[k]++
		// "StartTimestampUnixNano" is ignored for all data points.
		gauge := m.IntGauge()
		points := gauge.DataPoints()
		output = make([]telemetry.Metric, 0, points.Len())
		for l := 0; l < points.Len(); l++ {
			point := points.At(l)
			attributes := t.MetricAttributes(baseAttributes, point.LabelsMap())

			nrMetric := telemetry.Gauge{
				Name:       m.Name(),
				Attributes: attributes,
				Value:      float64(point.Value()),
				Timestamp:  point.Timestamp().AsTime(),
			}
			output = append(output, nrMetric)
		}
	case pdata.MetricDataTypeDoubleGauge:
		t.details.metricMetadataCount[k]++
		// "StartTimestampUnixNano" is ignored for all data points.
		gauge := m.DoubleGauge()
		points := gauge.DataPoints()
		output = make([]telemetry.Metric, 0, points.Len())
		for l := 0; l < points.Len(); l++ {
			point := points.At(l)
			attributes := t.MetricAttributes(baseAttributes, point.LabelsMap())

			nrMetric := telemetry.Gauge{
				Name:       m.Name(),
				Attributes: attributes,
				Value:      point.Value(),
				Timestamp:  point.Timestamp().AsTime(),
			}
			output = append(output, nrMetric)
		}
	case pdata.MetricDataTypeIntSum:
		// aggregation_temporality describes if the aggregator reports delta changes
		// since last report time, or cumulative changes since a fixed start time.
		sum := m.IntSum()
		temporality := sum.AggregationTemporality()
		k.MetricTemporality = temporality
		t.details.metricMetadataCount[k]++

		if temporality != pdata.AggregationTemporalityDelta {
			return nil, &errUnsupportedMetricType{metricType: k.MetricType.String(), metricName: m.Name(), numDataPoints: sum.DataPoints().Len()}
		}

		points := sum.DataPoints()
		output = make([]telemetry.Metric, 0, points.Len())
		for l := 0; l < points.Len(); l++ {
			point := points.At(l)
			attributes := t.MetricAttributes(baseAttributes, point.LabelsMap())

			nrMetric := telemetry.Count{
				Name:       m.Name(),
				Attributes: attributes,
				Value:      float64(point.Value()),
				Timestamp:  point.StartTimestamp().AsTime(),
				Interval:   time.Duration(point.Timestamp() - point.StartTimestamp()),
			}
			output = append(output, nrMetric)
		}
	case pdata.MetricDataTypeDoubleSum:
		sum := m.DoubleSum()
		temporality := sum.AggregationTemporality()
		k.MetricTemporality = temporality
		t.details.metricMetadataCount[k]++

		if temporality != pdata.AggregationTemporalityDelta {
			return nil, &errUnsupportedMetricType{metricType: k.MetricType.String(), metricName: m.Name(), numDataPoints: sum.DataPoints().Len()}
		}

		points := sum.DataPoints()
		output = make([]telemetry.Metric, 0, points.Len())
		for l := 0; l < points.Len(); l++ {
			point := points.At(l)
			attributes := t.MetricAttributes(baseAttributes, point.LabelsMap())

			nrMetric := telemetry.Count{
				Name:       m.Name(),
				Attributes: attributes,
				Value:      point.Value(),
				Timestamp:  point.StartTimestamp().AsTime(),
				Interval:   time.Duration(point.Timestamp() - point.StartTimestamp()),
			}
			output = append(output, nrMetric)
		}
	case pdata.MetricDataTypeIntHistogram:
		t.details.metricMetadataCount[k]++
		hist := m.IntHistogram()
		return nil, &errUnsupportedMetricType{metricType: k.MetricType.String(), metricName: m.Name(), numDataPoints: hist.DataPoints().Len()}
	case pdata.MetricDataTypeHistogram:
		t.details.metricMetadataCount[k]++
		hist := m.Histogram()
		return nil, &errUnsupportedMetricType{metricType: k.MetricType.String(), metricName: m.Name(), numDataPoints: hist.DataPoints().Len()}
	case pdata.MetricDataTypeSummary:
		t.details.metricMetadataCount[k]++
		summary := m.Summary()
		points := summary.DataPoints()
		output = make([]telemetry.Metric, 0, points.Len())
		name := m.Name()
		for l := 0; l < points.Len(); l++ {
			point := points.At(l)
			quantiles := point.QuantileValues()
			minQuantile := math.NaN()
			maxQuantile := math.NaN()

			if quantiles.Len() > 0 {
				quantileA := quantiles.At(0)
				if quantileA.Quantile() == 0 {
					minQuantile = quantileA.Value()
				}
				if quantiles.Len() > 1 {
					quantileB := quantiles.At(quantiles.Len() - 1)
					if quantileB.Quantile() == 1 {
						maxQuantile = quantileB.Value()
					}
				} else if quantileA.Quantile() == 1 {
					maxQuantile = quantileA.Value()
				}
			}

			attributes := t.MetricAttributes(baseAttributes, point.LabelsMap())
			nrMetric := telemetry.Summary{
				Name:       name,
				Attributes: attributes,
				Count:      float64(point.Count()),
				Sum:        point.Sum(),
				Min:        minQuantile,
				Max:        maxQuantile,
				Timestamp:  point.StartTimestamp().AsTime(),
				Interval:   time.Duration(point.Timestamp() - point.StartTimestamp()),
			}

			output = append(output, nrMetric)
		}
	default:
		t.details.metricMetadataCount[k]++
	}
	return output, nil
}

func (t *transformer) BaseMetricAttributes(metric pdata.Metric) map[string]interface{} {
	length := 0

	if metric.Unit() != "" {
		length++
	}

	if metric.Description() != "" {
		length++
	}

	attrs := make(map[string]interface{}, length)

	if metric.Unit() != "" {
		attrs[unitAttrKey] = metric.Unit()
	}

	if metric.Description() != "" {
		attrs[descriptionAttrKey] = metric.Description()
	}
	return attrs
}

func (t *transformer) MetricAttributes(baseAttributes map[string]interface{}, attrMap pdata.StringMap) map[string]interface{} {
	rawMap := make(map[string]interface{}, len(baseAttributes)+attrMap.Len())
	for k, v := range baseAttributes {
		rawMap[k] = v
	}
	attrMap.Range(func(k string, v string) bool {
		// Only include attribute if not an override attribute
		if _, isOverrideKey := t.OverrideAttributes[k]; !isOverrideKey {
			rawMap[k] = v
		}
		return true
	})

	return rawMap
}

func (t *transformer) TrackAttributes(location attributeLocation, attributeMap pdata.AttributeMap) {
	attributeMap.Range(func(k string, v pdata.AttributeValue) bool {
		statsKey := attributeStatsKey{location: location, attributeType: v.Type()}
		t.details.attributeMetadataCount[statsKey]++
		return true
	})
}
