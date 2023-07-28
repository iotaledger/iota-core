package collector

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/iotaledger/hive.go/runtime/options"
)

type MetricType uint8

const (
	// Gauge is a metric that represents a single numerical value that can arbitrarily go up and down.
	// during metric Update the collected value is set, thus previous value is overwritten.
	Gauge MetricType = iota
	// Counter is a cumulative metric that represents a single numerical value that only ever goes up.
	// during metric Update the collected value is added to its current value.
	Counter
)

// Metric is a single metric that will be registered to prometheus registry and collected with WithCollectFunc callback.
// Metric can be collected periodically based on metric collection rate of prometheus or WithUpdateOnEvent callback
// can be provided, so the Metric will keep its internal representation of metrics value,
// and WithCollectFunc will use it instead requesting data directly form other components.
type Metric struct {
	Name          string
	Type          MetricType
	Namespace     string
	help          string
	labels        []string
	collectFunc   func() (value float64, labelValues []string)
	initValueFunc func() (value float64, labelValues []string)
	initFunc      func()

	promMetric   prometheus.Collector
	resetEnabled bool // if enabled metric will be reset before each collectFunction call

	once sync.Once
}

// NewMetric creates a new metric with given name and options.
func NewMetric(name string, opts ...options.Option[Metric]) *Metric {
	m := options.Apply(&Metric{
		Name:        name,
		collectFunc: func() (value float64, labelValues []string) { return 0, nil },
	}, opts)

	return m
}

func (m *Metric) initPromMetric() {
	m.once.Do(func() {
		switch m.Type {
		case Gauge:
			if len(m.labels) > 0 {
				m.promMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
					Name:      m.Name,
					Namespace: m.Namespace,
					Help:      m.help,
				}, m.labels)

				return
			}
			m.promMetric = prometheus.NewGauge(prometheus.GaugeOpts{
				Name:      m.Name,
				Namespace: m.Namespace,
				Help:      m.help,
			})
		case Counter:
			if len(m.labels) > 0 {
				m.promMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
					Name:      m.Name,
					Namespace: m.Namespace,
					Help:      m.help,
				}, m.labels)

				return
			}
			m.promMetric = prometheus.NewCounter(prometheus.CounterOpts{
				Name:      m.Name,
				Namespace: m.Namespace,
				Help:      m.help,
			})
		}
	})
}

func (m *Metric) collect() {
	if m.resetEnabled {
		m.Reset()
	}
	value, labelValues := m.collectFunc()
	m.update(value, labelValues...)
}

func (m *Metric) update(metricValue float64, labelValues ...string) {
	if len(labelValues) != len(m.labels) {
		fmt.Println("Warning! Nothing updated, label values and labels length mismatch when updating metric", m.Name)
		return
	}
	m.metricUpdate(metricValue, labelValues...)
}

func (m *Metric) metricUpdate(value float64, labelValues ...string) {
	switch m.promMetric.(type) {
	case prometheus.Gauge:
		m.promMetric.(prometheus.Gauge).Set(value)
	case *prometheus.GaugeVec:
		m.promMetric.(*prometheus.GaugeVec).WithLabelValues(labelValues...).Set(value)
	case prometheus.Counter:
		m.promMetric.(prometheus.Counter).Add(value)
	case *prometheus.CounterVec:
		m.promMetric.(*prometheus.CounterVec).WithLabelValues(labelValues...).Add(value)
	}
}

func (m *Metric) increment(labelValues ...string) {
	if len(labelValues) != len(m.labels) {
		fmt.Println("Warning! Nothing updated, label values and labels length mismatch when updating metric", m.Name)
		return
	}
	m.metricIncrement(labelValues...)
}

func (m *Metric) metricIncrement(labelValues ...string) {
	switch m.promMetric.(type) {
	case prometheus.Gauge:
		m.promMetric.(prometheus.Gauge).Inc()
	case prometheus.GaugeVec:
		m.promMetric.(*prometheus.GaugeVec).WithLabelValues(labelValues...).Inc()
	case prometheus.Counter:
		m.promMetric.(prometheus.Counter).Inc()
	case prometheus.CounterVec:
		m.promMetric.(*prometheus.CounterVec).WithLabelValues(labelValues...).Inc()
	}
}

func (m *Metric) Reset() {
	switch m.promMetric.(type) {
	case prometheus.Gauge:
		m.promMetric.(prometheus.Gauge).Set(0)
	case prometheus.GaugeVec:
		m.promMetric.(*prometheus.GaugeVec).Reset()
	case prometheus.Counter:
		m.promMetric = prometheus.NewCounter(prometheus.CounterOpts{
			Name:      m.Name,
			Namespace: m.Namespace,
			Help:      m.help,
		})
	case prometheus.CounterVec:
		m.promMetric.(*prometheus.CounterVec).Reset()
	}
}

// DeleteLabels deletes the metric value matching the provided labels.
func (m *Metric) DeleteLabels(labels map[string]string) {
	// We can only reset labels if we initialized this metric to have labels in the first place.
	if len(m.labels) == len(labels) {
		switch m.Type {
		case Gauge:
			m.promMetric.(*prometheus.GaugeVec).Delete(labels)
		case Counter:
			m.promMetric.(*prometheus.CounterVec).Delete(labels)
		}
	}
}

// WithType sets the metric type: Gauge, GaugeVec, Counter, CounterVec.
func WithType(t MetricType) options.Option[Metric] {
	return func(m *Metric) {
		m.Type = t
	}
}

// WithHelp sets the help text for the metric.
func WithHelp(help string) options.Option[Metric] {
	return func(m *Metric) {
		m.help = help
	}
}

// WithLabels allows to define labels for the metric, they will need to be passed in the same order to the Update.
func WithLabels(labels ...string) options.Option[Metric] {
	return func(m *Metric) {
		m.labels = labels
	}
}

// WithResetBeforeCollecting  if enabled there will be a reset call on metric before each collectFunction call.
func WithResetBeforeCollecting(resetEnabled bool) options.Option[Metric] {
	return func(m *Metric) {
		m.resetEnabled = resetEnabled
	}
}

// WithCollectFunc allows to define a function that will be called each time when prometheus will scrap the data.
// Should be used when metric value can be read at any time and we don't need to attach to an event.
func WithCollectFunc(collectFunc func() (metricValue float64, labelValues []string)) options.Option[Metric] {
	return func(m *Metric) {
		m.collectFunc = collectFunc
	}
}

// WithInitValueFunc allows to set function that sets an initial value for a metric.
func WithInitValueFunc(initValueFunc func() (metricValue float64, labelValues []string)) options.Option[Metric] {
	return func(m *Metric) {
		m.initValueFunc = initValueFunc
	}
}

// WithInitFunc allows to define a function that will be called once when metric is created. Should be used instead of WithCollectFunc
// when metric value needs to be collected on event. With this type of collection we need to make sure that we call one
// of update methods of collector e.g.: Increment, Update.
func WithInitFunc(initFunc func()) options.Option[Metric] {
	return func(m *Metric) {
		m.initFunc = initFunc
	}
}
