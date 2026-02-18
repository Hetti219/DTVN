package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Consensus metrics
	ConsensusRoundsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "consensus_rounds_total",
		Help: "Total number of consensus rounds",
	})

	ConsensusSuccessTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "consensus_success_total",
		Help: "Total number of successful consensus rounds",
	})

	ConsensusFailureTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "consensus_failure_total",
		Help: "Total number of failed consensus rounds",
	})

	ConsensusLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "consensus_latency_seconds",
		Help:    "Consensus round latency in seconds",
		Buckets: prometheus.DefBuckets,
	})

	// Gossip metrics
	GossipMessagesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gossip_messages_sent_total",
		Help: "Total number of gossip messages sent",
	})

	GossipMessagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gossip_messages_received_total",
		Help: "Total number of gossip messages received",
	})

	GossipCacheSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gossip_cache_size",
		Help: "Current size of the gossip message cache",
	})

	GossipCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gossip_cache_hits_total",
		Help: "Total number of gossip cache hits",
	})

	GossipCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gossip_cache_misses_total",
		Help: "Total number of gossip cache misses",
	})

	// Network metrics
	PeerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "peer_count",
		Help: "Current number of connected peers",
	})

	NetworkMessagesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "network_messages_sent_total",
		Help: "Total number of network messages sent",
	})

	NetworkMessagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "network_messages_received_total",
		Help: "Total number of network messages received",
	})

	NetworkBytesTransmitted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "network_bytes_transmitted_total",
		Help: "Total number of bytes transmitted",
	})

	// Ticket metrics
	TicketsValidated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tickets_validated_total",
		Help: "Total number of tickets validated",
	})

	TicketsConsumed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tickets_consumed_total",
		Help: "Total number of tickets consumed",
	})

	TicketsDisputed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tickets_disputed_total",
		Help: "Total number of tickets disputed",
	})

	TicketStateGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ticket_state_count",
			Help: "Number of tickets in each state",
		},
		[]string{"state"},
	)

	// Storage metrics
	StorageOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "storage_operations_total",
			Help: "Total number of storage operations",
		},
		[]string{"operation"},
	)

	StorageLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "storage_latency_seconds",
			Help:    "Storage operation latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// API metrics
	APIRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "api_requests_total",
			Help: "Total number of API requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	APILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "api_latency_seconds",
			Help:    "API request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	WebSocketConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "websocket_connections",
		Help: "Current number of WebSocket connections",
	})
)

// RegisterMetrics registers all metrics with Prometheus.
// Uses Register instead of MustRegister so that calling this function more than
// once (e.g. in tests) does not panic on AlreadyRegisteredError.
func RegisterMetrics() {
	collectors := []prometheus.Collector{
		// Consensus
		ConsensusRoundsTotal,
		ConsensusSuccessTotal,
		ConsensusFailureTotal,
		ConsensusLatency,
		// Gossip
		GossipMessagesSent,
		GossipMessagesReceived,
		GossipCacheSize,
		GossipCacheHits,
		GossipCacheMisses,
		// Network
		PeerCount,
		NetworkMessagesSent,
		NetworkMessagesReceived,
		NetworkBytesTransmitted,
		// Tickets
		TicketsValidated,
		TicketsConsumed,
		TicketsDisputed,
		TicketStateGauge,
		// Storage
		StorageOperations,
		StorageLatency,
		// API
		APIRequests,
		APILatency,
		WebSocketConnections,
	}
	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				fmt.Printf("metrics: failed to register collector: %v\n", err)
			}
		}
	}
}

// Handler returns the Prometheus HTTP handler
func Handler() http.Handler {
	return promhttp.Handler()
}

// StartMetricsServer starts a metrics server on the specified address
func StartMetricsServer(addr string) error {
	http.Handle("/metrics", Handler())
	return http.ListenAndServe(addr, nil)
}
