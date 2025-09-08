package metrics

// MetricSource represents different types of metric collection sources
type MetricSource string

const (
	SOURCE_REAL_OPEN5GS    MetricSource = "real_open5gs"
	SOURCE_CONTAINER_STATS MetricSource = "container_stats"
	SOURCE_HEALTH_CHECK    MetricSource = "health_check"
)

// MetricTarget represents a target for Prometheus scraping
type MetricTarget struct {
	JobName     string            `json:"job_name"`
	Target      string            `json:"target"`
	Labels      map[string]string `json:"labels"`
	Source      MetricSource      `json:"source"`
	ScrapeePath string            `json:"scrape_path"`
	Interval    string            `json:"interval"`
	ComponentID string            `json:"component_id"`
}

// MetricsRegistry holds the registry of all available metrics
type MetricsRegistry struct {
	Targets map[string]*MetricTarget `json:"targets"`
}
