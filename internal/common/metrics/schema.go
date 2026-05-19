package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	eventSchemaConsumerVersions = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "paygate_event_schema_consumer_version_active",
		Help: "Active schema version markers observed by consumer group and topic",
	}, []string{"consumer_group", "topic", "schema_subject", "schema_version"})
	deprecatedSchemaAlerts = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "paygate_event_schema_deprecated_version_overdue",
		Help: "Deprecated schema version cutovers that remain overdue per consumer",
	}, []string{"schema_subject", "from_version", "to_version", "consumer_name"})
	registerSchemaMetricsOnce sync.Once
)

func RecordConsumerSchemaVersion(consumerGroup, topic, schemaSubject, schemaVersion string) {
	registerSchemaMetricsOnce.Do(func() {
		prometheus.MustRegister(eventSchemaConsumerVersions)
		prometheus.MustRegister(deprecatedSchemaAlerts)
	})
	eventSchemaConsumerVersions.WithLabelValues(consumerGroup, topic, schemaSubject, schemaVersion).Set(1)
}

func RecordDeprecatedSchemaAlert(schemaSubject, fromVersion, toVersion, consumerName string) {
	registerSchemaMetricsOnce.Do(func() {
		prometheus.MustRegister(eventSchemaConsumerVersions)
		prometheus.MustRegister(deprecatedSchemaAlerts)
	})
	deprecatedSchemaAlerts.WithLabelValues(schemaSubject, fromVersion, toVersion, consumerName).Set(1)
}

func ResetDeprecatedSchemaAlerts() {
	registerSchemaMetricsOnce.Do(func() {
		prometheus.MustRegister(eventSchemaConsumerVersions)
		prometheus.MustRegister(deprecatedSchemaAlerts)
	})
	deprecatedSchemaAlerts.Reset()
}
