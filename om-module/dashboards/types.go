package dashboards

import "time"

// Grafana Dashboard Structures (from your existing code)
type Dashboard struct {
	ID                   *int        `json:"id,omitempty"`
	Title                string      `json:"title"`
	Description          string      `json:"description"`
	Tags                 []string    `json:"tags"`
	Style                string      `json:"style"`
	Timezone             string      `json:"timezone"`
	Editable             bool        `json:"editable"`
	GraphTooltip         int         `json:"graphTooltip"`
	Time                 TimeRange   `json:"time"`
	Timepicker           TimePicker  `json:"timepicker"`
	Templating           Templating  `json:"templating"`
	Annotations          Annotations `json:"annotations"`
	Refresh              string      `json:"refresh"`
	SchemaVersion        int         `json:"schemaVersion"`
	Version              int         `json:"version"`
	Panels               []Panel     `json:"panels"`
	FiscalYearStartMonth int         `json:"fiscalYearStartMonth"`
}

type TimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type TimePicker struct {
	RefreshIntervals []string `json:"refresh_intervals"`
}

type Templating struct {
	List []any `json:"list"`
}

type Annotations struct {
	List []any `json:"list"`
}

type Panel struct {
	ID          int           `json:"id"`
	Title       string        `json:"title"`
	Type        string        `json:"type"`
	GridPos     GridPos       `json:"gridPos"`
	Targets     []Target      `json:"targets"`
	Options     *PanelOptions `json:"options,omitempty"`
	FieldConfig *FieldConfig  `json:"fieldConfig,omitempty"`
	Description string        `json:"description,omitempty"`
	Datasource  *Datasource   `json:"datasource,omitempty"`
}

type GridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

type Target struct {
	Expr         string      `json:"expr"`
	Interval     string      `json:"interval,omitempty"`
	LegendFormat string      `json:"legendFormat,omitempty"`
	RefID        string      `json:"refId"`
	Datasource   *Datasource `json:"datasource,omitempty"`
	QueryType    string      `json:"queryType,omitempty"`
	MaxLines     int         `json:"maxLines,omitempty"`
}

type Datasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

type PanelOptions struct {
	ReduceOptions *ReduceOptions   `json:"reduceOptions,omitempty"`
	Orientation   string           `json:"orientation,omitempty"`
	TextMode      string           `json:"textMode,omitempty"`
	ColorMode     string           `json:"colorMode,omitempty"`
	GraphMode     string           `json:"graphMode,omitempty"`
	JustifyMode   string           `json:"justifyMode,omitempty"`
	DisplayMode   string           `json:"displayMode,omitempty"`
	Content       string           `json:"content,omitempty"`
	Mode          string           `json:"mode,omitempty"`
	Tooltip       *TooltipOptions  `json:"tooltip,omitempty"`
	Legend        *LegendOptions   `json:"legend,omitempty"`
	Min           *float64         `json:"min,omitempty"`
	Max           *float64         `json:"max,omitempty"`
	Thresholds    *ThresholdConfig `json:"thresholds,omitempty"`
}

type TooltipOptions struct {
	Mode string `json:"mode"`
}

type LegendOptions struct {
	DisplayMode string   `json:"displayMode"`
	Values      []string `json:"values"`
}

type ReduceOptions struct {
	Values bool     `json:"values"`
	Calcs  []string `json:"calcs"`
	Fields string   `json:"fields"`
}

type FieldConfig struct {
	Defaults  *FieldDefaults `json:"defaults,omitempty"`
	Overrides []any          `json:"overrides,omitempty"`
}

type FieldDefaults struct {
	Color      *ColorConfig     `json:"color,omitempty"`
	Custom     *CustomConfig    `json:"custom,omitempty"`
	Mappings   []any            `json:"mappings,omitempty"`
	Thresholds *ThresholdConfig `json:"thresholds,omitempty"`
	Unit       string           `json:"unit,omitempty"`
	Min        *float64         `json:"min,omitempty"`
	Max        *float64         `json:"max,omitempty"`
}

type ColorConfig struct {
	Mode string `json:"mode"`
}

type CustomConfig struct {
	DrawStyle         string          `json:"drawStyle,omitempty"`
	LineInterpolation string          `json:"lineInterpolation,omitempty"`
	LineWidth         int             `json:"lineWidth,omitempty"`
	FillOpacity       int             `json:"fillOpacity,omitempty"`
	GradientMode      string          `json:"gradientMode,omitempty"`
	SpanNulls         bool            `json:"spanNulls,omitempty"`
	PointSize         int             `json:"pointSize,omitempty"`
	Stacking          *StackingConfig `json:"stacking,omitempty"`
}

type StackingConfig struct {
	Group string `json:"group"`
	Mode  string `json:"mode"`
}

type ThresholdConfig struct {
	Mode  string          `json:"mode"`
	Steps []ThresholdStep `json:"steps"`
}

type ThresholdStep struct {
	Color string   `json:"color"`
	Value *float64 `json:"value"`
}

// Template Variable for dashboards
type TemplateVariable struct {
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	Type       string   `json:"type"`
	Query      string   `json:"query,omitempty"`
	Datasource string   `json:"datasource,omitempty"`
	Options    []string `json:"options,omitempty"`
	Multi      bool     `json:"multi"`
}

// MetricInfo holds discovered metric information
type MetricInfo struct {
	Name      string
	Type      string // gauge, counter, histogram
	Help      string
	Component string
	Category  string // "session", "connection", "system", etc.
}

// ComponentMetrics holds all metrics for a component
type ComponentMetrics struct {
	ComponentName string
	Metrics       []MetricInfo
	IsAvailable   bool
	LastUpdated   time.Time
}
