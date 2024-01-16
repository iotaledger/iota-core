package collector

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Collector is responsible for creation and collection of metrics for the prometheus.
type Collector struct {
	Registry    *prometheus.Registry
	collections map[string]*Collection
}

// New creates an instance of Manager and creates a new prometheus registry for the protocol metrics collection.
func New() *Collector {
	return &Collector{
		Registry:    prometheus.NewRegistry(),
		collections: make(map[string]*Collection),
	}
}

func (c *Collector) RegisterCollection(coll *Collection) {
	c.collections[coll.CollectionName] = coll
	for _, m := range coll.metrics {
		c.Registry.MustRegister(m.promMetric)
		if m.initValueFunc != nil {
			metricValue, labelValues := m.initValueFunc()
			m.update(metricValue, labelValues...)
		}
		if m.initFunc != nil {
			m.initFunc()
		}
	}
}

// Collect collects all metrics from the registered collections.
func (c *Collector) Collect() {
	for _, collection := range c.collections {
		for _, metric := range collection.metrics {
			metric.collect()
		}
	}
}

// Update updates the value of the existing metric defined by the subsystem and metricName.
// Note that the label values must be passed in the same order as they were defined in the metric, and must match the
// number of labels defined in the metric.
func (c *Collector) Update(subsystem string, metricName string, metricValue float64, labelValues ...string) {
	m := c.getMetric(subsystem, metricName)
	if m != nil {
		m.update(metricValue, labelValues...)
	}
}

// Increment increments the value of the existing metric defined by the subsystem and metricName.
// Note that the label values must be passed in the same order as they were defined in the metric, and must match the
// number of labels defined in the metric.
func (c *Collector) Increment(subsystem string, metricName string, labels ...string) {
	m := c.getMetric(subsystem, metricName)
	if m != nil {
		m.increment(labels...)
	}
}

// DeleteLabels deletes the metric with the given labels values.
func (c *Collector) DeleteLabels(subsystem string, metricName string, labelValues map[string]string) {
	m := c.getMetric(subsystem, metricName)
	if m != nil {
		m.deleteLabels(labelValues)
	}
}

// ResetMetric resets the metric with the given name.
func (c *Collector) ResetMetric(namespace string, metricName string) {
	m := c.getMetric(namespace, metricName)
	if m != nil {
		m.reset()
	}
}

func (c *Collector) Shutdown() {
	for _, collection := range c.collections {
		for _, metric := range collection.metrics {
			metric.shutdown()
		}
	}
}

func (c *Collector) getMetric(subsystem string, metricName string) *Metric {
	col := c.getCollection(subsystem)
	if col != nil {
		return col.GetMetric(metricName)
	}

	return nil
}

func (c *Collector) getCollection(subsystem string) *Collection {
	if collection, exists := c.collections[subsystem]; exists {
		return collection
	}

	return nil
}
