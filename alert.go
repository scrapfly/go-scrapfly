package scrapfly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AlertState is the lifecycle state of an alert definition. Mirrors the
// AlertState enum in scrapfly-api/pkg/alert/model.go.
type AlertState string

const (
	AlertStateOK         AlertState = "ok"
	AlertStatePending    AlertState = "pending"
	AlertStateTriggered  AlertState = "triggered"
	AlertStateRecovering AlertState = "recovering"
	AlertStateNoData     AlertState = "no_data"
	AlertStateSnoozed    AlertState = "snoozed"
)

// AlertComparator is the threshold comparison operator. Mirrors the
// Comparator enum in scrapfly-api/pkg/alert/model.go. The API rejects
// any other string with ERR::ALERT::INVALID_THRESHOLD.
type AlertComparator string

const (
	AlertComparatorGt  AlertComparator = "gt"
	AlertComparatorLt  AlertComparator = "lt"
	AlertComparatorGte AlertComparator = "gte"
	AlertComparatorLte AlertComparator = "lte"
	AlertComparatorEq  AlertComparator = "eq"
	AlertComparatorNeq AlertComparator = "neq"
)

// AlertNoDataPolicy controls what to do when the underlying ClickHouse
// rollup returns no rows in the evaluation window. Mirrors NoDataPolicy
// in the API.
type AlertNoDataPolicy string

const (
	AlertNoDataOK        AlertNoDataPolicy = "ok"
	AlertNoDataTriggered AlertNoDataPolicy = "triggered"
	AlertNoDataIgnore    AlertNoDataPolicy = "ignore"
)

// AlertNotifyKind is the delivery channel for an alert notification.
// Mirrors NotifyKind in the API.
type AlertNotifyKind string

const (
	AlertChannelEmail   AlertNotifyKind = "email"
	AlertChannelWebhook AlertNotifyKind = "webhook"
	AlertChannelInApp   AlertNotifyKind = "inapp"
)

// AlertNotifyChannel is a single configured delivery target on an alert.
// Target is the email address, webhook URL, or empty string for inapp.
// Opts carries channel-specific settings (e.g. custom webhook headers).
type AlertNotifyChannel struct {
	Kind   AlertNotifyKind   `json:"kind"`
	Target string            `json:"target"`
	Opts   map[string]string `json:"opts,omitempty"`
}

// Alert is the full row mirror of alert_definition. Pointer fields map
// to nullable SQL columns — nil means "never set" for last_notified_at,
// last_evaluated_at, last_metric_value, snoozed_until, schedule_uuid.
//
// HmacKey is the per-alert HMAC-SHA256 signing secret used to authenticate
// webhook deliveries. Auto-generated on Create; read it from this struct
// to configure your webhook verifier.
type Alert struct {
	UUID        string `json:"alert_uuid"`
	ProjectUUID string `json:"project_uuid"`
	ProjectName string `json:"project_name"`
	UserUUID    string `json:"user_uuid"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	HmacKey     string `json:"hmac_key"`

	MetricID         string          `json:"metric_id"`
	MetricDimensions json.RawMessage `json:"metric_dimensions"`

	Comparator         AlertComparator `json:"comparator"`
	Threshold          float64         `json:"threshold"`
	SustainedMinutes   int             `json:"sustained_minutes"`
	RecoveryMinutes    int             `json:"recovery_minutes"`
	EvaluationWindowM  int             `json:"evaluation_window_m"`
	EvalCadenceSeconds int             `json:"eval_cadence_seconds"`

	NotifyChannels  []AlertNotifyChannel `json:"notify_channels"`
	RenotifyMinutes int                  `json:"renotify_minutes"`
	NoDataPolicy    AlertNoDataPolicy    `json:"no_data_policy"`

	State              AlertState `json:"state"`
	StateSince         time.Time  `json:"state_since"`
	LastNotifiedAt     *time.Time `json:"last_notified_at,omitempty"`
	LastEvaluatedAt    *time.Time `json:"last_evaluated_at,omitempty"`
	LastMetricValue    *float64   `json:"last_metric_value,omitempty"`
	SnoozedUntil       *time.Time `json:"snoozed_until,omitempty"`
	NotificationsToday int        `json:"notifications_today"`

	ScheduleUUID *string `json:"schedule_uuid,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AlertMetricFamily is one entry in the metric registry. The dashboard's
// create wizard renders the metric picker from this list; SDK callers
// use it to discover legal MetricID values and their allowed dimensions
// before issuing CreateAlert.
type AlertMetricFamily struct {
	ID                      string            `json:"id"`
	DisplayName             string            `json:"display_name"`
	Description             string            `json:"description"`
	Unit                    string            `json:"unit"`
	Product                 string            `json:"product"`
	SourceTable             string            `json:"source_table"`
	Expression              string            `json:"expression"`
	BucketGrainMinutes      int               `json:"bucket_grain_minutes"`
	AllowedDimensions       []string          `json:"allowed_dimensions"`
	TagDimensions           map[string]string `json:"tag_dimensions,omitempty"`
	DefaultSustainedMinutes int               `json:"default_sustained_minutes"`
	TimeColumn              string            `json:"time_column"`
	ProjectColumn           string            `json:"project_column"`
}

// AlertSeriesPoint is a single (time, value) pair in the metric series
// returned by GetAlertSeries / PreviewAlert.
type AlertSeriesPoint struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// AlertTransitionMarker is a state-change event overlaid on the graph.
type AlertTransitionMarker struct {
	T    time.Time  `json:"t"`
	From AlertState `json:"from"`
	To   AlertState `json:"to"`
}

// AlertSeriesResponse is the payload returned by GetAlertSeries —
// everything a chart needs to plot the metric with the threshold band
// and state-change transitions overlaid.
type AlertSeriesResponse struct {
	Points        []AlertSeriesPoint      `json:"points"`
	Threshold     float64                 `json:"threshold"`
	Comparator    AlertComparator         `json:"comparator"`
	Transitions   []AlertTransitionMarker `json:"transitions"`
	Unit          string                  `json:"unit"`
	MetricID      string                  `json:"metric_id"`
	BucketMinutes int                     `json:"bucket_minutes"`
}

// AlertPreviewFiredMarker identifies a TRIGGERED window discovered during
// a historical preview replay. ResolvedAt is nil when the rule was still
// firing at the end of the preview range.
type AlertPreviewFiredMarker struct {
	FiredAt    time.Time  `json:"fired_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	PeakValue  float64    `json:"peak_value"`
}

// AlertPreviewResponse is the payload returned by PreviewAlert.
// FiredCount is len(Markers) — kept as an explicit field so callers can
// surface the headline number without iterating.
type AlertPreviewResponse struct {
	Points        []AlertSeriesPoint        `json:"points"`
	Threshold     float64                   `json:"threshold"`
	Comparator    AlertComparator           `json:"comparator"`
	Unit          string                    `json:"unit"`
	BucketMinutes int                       `json:"bucket_minutes"`
	FiredCount    int                       `json:"fired_count"`
	Markers       []AlertPreviewFiredMarker `json:"markers"`
}

// AlertListOptions narrows ListAlerts. Empty fields are omitted from the
// query string — pass an empty struct to get all alerts for the caller.
type AlertListOptions struct {
	ProjectUUID string
	State       AlertState
	MetricID    string
}

// AlertCreateRequest is the body for CreateAlert. ProjectUUID/ProjectName
// are optional — when empty the API substitutes the caller's currently-
// selected project (the dashboard's project switcher controls this).
//
// Server-side defaults applied when zero values are sent:
//   - EvalCadenceSeconds:  300
//   - SustainedMinutes:    metric family default (or 5)
//   - EvaluationWindowM:   SustainedMinutes
//   - NoDataPolicy:        ignore
//   - RenotifyMinutes:     60
//
// ValidateAlertCreate runs the same checks the API enforces — call it
// before sending the request to fail fast on the client.
type AlertCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ProjectUUID string `json:"project_uuid,omitempty"`
	ProjectName string `json:"project_name,omitempty"`

	MetricID         string          `json:"metric_id"`
	MetricDimensions json.RawMessage `json:"metric_dimensions,omitempty"`

	Comparator         AlertComparator `json:"comparator"`
	Threshold          float64         `json:"threshold"`
	SustainedMinutes   int             `json:"sustained_minutes,omitempty"`
	RecoveryMinutes    int             `json:"recovery_minutes,omitempty"`
	EvaluationWindowM  int             `json:"evaluation_window_m,omitempty"`
	EvalCadenceSeconds int             `json:"eval_cadence_seconds,omitempty"`

	NotifyChannels  []AlertNotifyChannel `json:"notify_channels"`
	RenotifyMinutes int                  `json:"renotify_minutes,omitempty"`
	NoDataPolicy    AlertNoDataPolicy    `json:"no_data_policy,omitempty"`
}

// AlertUpdateRequest is the body for UpdateAlert. All fields are pointers
// so the caller can distinguish "omit" from "set to zero" — only non-nil
// fields are applied server-side. Each successful update auto-snoozes the
// alert for SustainedMinutes so a tuning iteration doesn't refire on the
// next eval tick.
type AlertUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`

	Comparator        *AlertComparator `json:"comparator,omitempty"`
	Threshold         *float64         `json:"threshold,omitempty"`
	SustainedMinutes  *int             `json:"sustained_minutes,omitempty"`
	RecoveryMinutes   *int             `json:"recovery_minutes,omitempty"`
	EvaluationWindowM *int             `json:"evaluation_window_m,omitempty"`

	NotifyChannels  []AlertNotifyChannel `json:"notify_channels,omitempty"`
	RenotifyMinutes *int                 `json:"renotify_minutes,omitempty"`
	NoDataPolicy    *AlertNoDataPolicy   `json:"no_data_policy,omitempty"`
}

// AlertPreviewRequest replays an unsaved rule against historical metric
// data without persisting anything. Use it to answer "how many times
// would this rule have fired in the last 24h?" before hitting Create.
type AlertPreviewRequest struct {
	MetricID         string          `json:"metric_id"`
	MetricDimensions json.RawMessage `json:"metric_dimensions,omitempty"`
	ProjectUUID      string          `json:"project_uuid,omitempty"`
	ProjectName      string          `json:"project_name,omitempty"`
	Comparator       AlertComparator `json:"comparator"`
	Threshold        float64         `json:"threshold"`
	SustainedMinutes int             `json:"sustained_minutes,omitempty"`
	// EvaluationWindowM matches the live evaluator's window; defaults to
	// SustainedMinutes server-side when zero.
	EvaluationWindowM int               `json:"evaluation_window_m,omitempty"`
	RangeMinutes      int               `json:"range_minutes,omitempty"` // default 1440 (24h)
	NoDataPolicy      AlertNoDataPolicy `json:"no_data_policy,omitempty"`
}

// AlertSnoozeRequest is the body for SnoozeAlert. Set Minutes for a
// time-bound snooze, OR UntilResolved=true to mute indefinitely until
// the next OK transition. Exactly one MUST be provided.
type AlertSnoozeRequest struct {
	Minutes       int  `json:"minutes,omitempty"`
	UntilResolved bool `json:"until_resolved,omitempty"`
}

// AlertTestResult is the response from TestAlert — a synthetic notification
// fired on every configured channel without touching alert state.
type AlertTestResult struct {
	OK        bool   `json:"ok"`
	AlertUUID string `json:"alert_uuid"`
	Channels  int    `json:"channels"`
	EventID   string `json:"event_id"`
}

// AlertCountActiveResult is the response from CountActiveAlerts — the
// count of alerts currently in actively-firing states (triggered,
// pending, recovering). Backs the sidebar badge in the dashboard.
type AlertCountActiveResult struct {
	Count int `json:"count"`
}

// AlertDeleteResult is the response from DeleteAlert.
type AlertDeleteResult struct {
	Deleted string `json:"deleted"`
}

// ValidateAlertCreate runs the same checks the API performs server-side
// (scrapfly-api/pkg/alert/validate.go). Use it to fail fast before the
// network round-trip. It does NOT validate that the MetricID exists in
// the registry — for that, call ListAlertMetricFamilies first.
//
// Returns nil on success or an error whose Error() string is the same
// shape the API would have returned (ERR::ALERT::* prefix + reason).
func ValidateAlertCreate(req AlertCreateRequest) error {
	if req.Threshold != req.Threshold || // NaN
		req.Threshold > 1.797e308 || req.Threshold < -1.797e308 {
		return fmt.Errorf("ERR::ALERT::INVALID_THRESHOLD: threshold must be a finite number")
	}
	if req.SustainedMinutes != 0 && (req.SustainedMinutes < 1 || req.SustainedMinutes > 1440) {
		return fmt.Errorf("ERR::ALERT::INVALID_SUSTAINED_WINDOW: sustained_minutes must be between 1 and 1440, got %d", req.SustainedMinutes)
	}
	switch req.Comparator {
	case AlertComparatorGt, AlertComparatorLt, AlertComparatorGte,
		AlertComparatorLte, AlertComparatorEq, AlertComparatorNeq:
	default:
		return fmt.Errorf("ERR::ALERT::INVALID_THRESHOLD: comparator %q is not valid (allowed: gt, lt, gte, lte, eq, neq)", req.Comparator)
	}
	if len(req.NotifyChannels) == 0 {
		return fmt.Errorf("ERR::ALERT::CHANNEL_UNREACHABLE: at least one notify_channel is required")
	}
	for i, ch := range req.NotifyChannels {
		switch ch.Kind {
		case AlertChannelEmail, AlertChannelWebhook, AlertChannelInApp:
		default:
			return fmt.Errorf("ERR::ALERT::CHANNEL_UNREACHABLE: notify_channels[%d] has unknown kind %q", i, ch.Kind)
		}
	}
	return nil
}

// ── Read endpoints ─────────────────────────────────────────────────
//
// Auth: session OR ?key= (api-key). All read methods work today with the
// SDK's ?key= auth.

// ListAlerts returns every alert definition owned by the caller. Filters
// in opts narrow the result server-side — pass an empty AlertListOptions{}
// to get all rows.
//
// Example:
//
//	alerts, err := client.ListAlerts(scrapfly.AlertListOptions{
//	    State: scrapfly.AlertStateTriggered,
//	})
func (c *Client) ListAlerts(opts AlertListOptions) ([]Alert, error) {
	params := url.Values{}
	params.Set("key", c.key)
	if opts.ProjectUUID != "" {
		params.Set("project_uuid", opts.ProjectUUID)
	}
	if opts.State != "" {
		params.Set("state", string(opts.State))
	}
	if opts.MetricID != "" {
		params.Set("metric_id", opts.MetricID)
	}
	var out []Alert
	if err := c.alertGetJSON("/alert", params, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CountActiveAlerts returns the count of alerts currently in actively-
// firing states (triggered, pending, recovering) for the caller. Cheap
// — designed for sub-millisecond badge rendering in the dashboard.
// projectUUID is optional; empty string means "all projects".
func (c *Client) CountActiveAlerts(projectUUID string) (*AlertCountActiveResult, error) {
	params := url.Values{}
	params.Set("key", c.key)
	if projectUUID != "" {
		params.Set("project_uuid", projectUUID)
	}
	var out AlertCountActiveResult
	if err := c.alertGetJSON("/alert/count-active", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAlert fetches one alert definition by UUID.
//
// Returns an *APIError with HTTPStatusCode 404 (Code ERR::ALERT::NOT_FOUND)
// for both "doesn't exist" and "not yours" — the API deliberately collapses
// these to prevent enumeration. Don't try to distinguish them.
func (c *Client) GetAlert(alertUUID string) (*Alert, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: GetAlert: alertUUID is required")
	}
	params := url.Values{}
	params.Set("key", c.key)
	var out Alert
	if err := c.alertGetJSON("/alert/"+url.PathEscape(alertUUID), params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAlertMetricFamilies returns the full metric registry (~19 entries
// at the time of writing). Use it to discover legal MetricID values
// before issuing CreateAlert, and to discover each metric's allowed
// dimensions and native bucket grain.
func (c *Client) ListAlertMetricFamilies() ([]AlertMetricFamily, error) {
	params := url.Values{}
	params.Set("key", c.key)
	var out []AlertMetricFamily
	if err := c.alertGetJSON("/alert/metric-families", params, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetAlertSeries returns the time-series metric data + state-change
// transitions for an alert. rangeMinutes is the lookback window (default
// 240 = 4h; max 7*24*60 = 10080 = 7 days). Values outside that bound are
// silently clamped to the default by the API.
func (c *Client) GetAlertSeries(alertUUID string, rangeMinutes int) (*AlertSeriesResponse, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: GetAlertSeries: alertUUID is required")
	}
	params := url.Values{}
	params.Set("key", c.key)
	if rangeMinutes > 0 {
		params.Set("range_minutes", strconv.Itoa(rangeMinutes))
	}
	var out AlertSeriesResponse
	if err := c.alertGetJSON("/alert/"+url.PathEscape(alertUUID)+"/series", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Write endpoints ────────────────────────────────────────────────
//
// Auth: session OR api-key, same as the read endpoints. The shared
// AuthMiddleware reads the key from ?key=, Authorization: Bearer, or
// X-API-Key, so an api-key-only client (this SDK's default) works on
// every endpoint below.

// PreviewAlert replays an unsaved rule against historical data and
// returns "would-have-fired" markers — without creating the alert.
// Useful for tuning thresholds before committing.
func (c *Client) PreviewAlert(req AlertPreviewRequest) (*AlertPreviewResponse, error) {
	var out AlertPreviewResponse
	if err := c.alertPostJSON("/alert/preview", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateAlert persists a new alert definition. The returned Alert carries
// the server-assigned UUID, the auto-generated HMAC signing key, and the
// resolved server-side defaults for any zero-value fields in the request.
//
// Calling ValidateAlertCreate(req) first is recommended — it runs the
// same checks server-side, so a successful local validation avoids one
// network round-trip per bad payload.
func (c *Client) CreateAlert(req AlertCreateRequest) (*Alert, error) {
	var out Alert
	if err := c.alertPostJSON("/alert", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateAlert patches an existing alert. All AlertUpdateRequest fields
// are pointers — only non-nil fields are applied. The alert is auto-
// snoozed for SustainedMinutes after a successful update, preventing
// notification spam during threshold tuning.
func (c *Client) UpdateAlert(alertUUID string, req AlertUpdateRequest) (*Alert, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: UpdateAlert: alertUUID is required")
	}
	var out Alert
	if err := c.alertDoJSON(http.MethodPut, "/alert/"+url.PathEscape(alertUUID), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAlert removes an alert definition. Returns {"deleted": "<uuid>"}.
// Idempotent on the happy path; calling twice for the same UUID returns
// 404 (ERR::ALERT::NOT_FOUND) on the second call.
func (c *Client) DeleteAlert(alertUUID string) (*AlertDeleteResult, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: DeleteAlert: alertUUID is required")
	}
	var out AlertDeleteResult
	if err := c.alertDoJSON(http.MethodDelete, "/alert/"+url.PathEscape(alertUUID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SnoozeAlert mutes notifications. Provide either Minutes>0 OR
// UntilResolved=true (not both) — the API rejects neither-or-both with
// ERR::ALERT::INVALID_SUSTAINED_WINDOW.
func (c *Client) SnoozeAlert(alertUUID string, req AlertSnoozeRequest) (*Alert, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: SnoozeAlert: alertUUID is required")
	}
	if req.Minutes <= 0 && !req.UntilResolved {
		return nil, fmt.Errorf("scrapfly: SnoozeAlert: provide minutes>0 or until_resolved=true")
	}
	var out Alert
	if err := c.alertPostJSON("/alert/"+url.PathEscape(alertUUID)+"/snooze", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UnsnoozeAlert lifts any active snooze, resuming normal notification
// delivery on the next evaluator tick.
func (c *Client) UnsnoozeAlert(alertUUID string) (*Alert, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: UnsnoozeAlert: alertUUID is required")
	}
	var out Alert
	if err := c.alertPostJSON("/alert/"+url.PathEscape(alertUUID)+"/unsnooze", struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TestAlert fires a one-shot synthetic notification on every configured
// channel WITHOUT touching alert state or the state machine. Useful for
// verifying webhook URLs and email addresses without waiting for a real
// transition.
//
// Dedup: two test fires within the same UTC minute share the same dedup
// key — the second is suppressed and the response carries the same
// EventID. This is intentional (matches the dashboard's UI rate-limit).
func (c *Client) TestAlert(alertUUID string) (*AlertTestResult, error) {
	if alertUUID == "" {
		return nil, fmt.Errorf("scrapfly: TestAlert: alertUUID is required")
	}
	var out AlertTestResult
	if err := c.alertPostJSON("/alert/"+url.PathEscape(alertUUID)+"/test", struct{}{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── Internal helpers ───────────────────────────────────────────────

// alertGetJSON issues a GET with the supplied querystring and decodes
// a JSON body into out. Wraps non-2xx via the shared error mapper so
// callers get *APIError just like every other SDK method.
func (c *Client) alertGetJSON(path string, params url.Values, out any) error {
	u, _ := url.Parse(c.host + path)
	u.RawQuery = params.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept", "application/json")
	return c.alertExec(req, out)
}

// alertPostJSON is a thin POST wrapper used by all alert mutating endpoints.
func (c *Client) alertPostJSON(path string, body, out any) error {
	return c.alertDoJSON(http.MethodPost, path, body, out)
}

// alertDoJSON issues a request with a JSON body and decodes a JSON
// response. body may be nil for verb-only calls (e.g. DELETE).
func (c *Client) alertDoJSON(method, path string, body, out any) error {
	u, _ := url.Parse(c.host + path)
	params := url.Values{}
	params.Set("key", c.key)
	u.RawQuery = params.Encode()

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("scrapfly: encode alert request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, u.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.alertExec(req, out)
}

// alertExec runs the request and decodes the response. On non-2xx it
// delegates to handleAPIErrorResponse (shared with monitoring + scrape),
// keeping error shapes consistent across the whole SDK.
func (c *Client) alertExec(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("scrapfly: read alert response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.handleAPIErrorResponse(resp, bodyBytes)
	}
	if out == nil || len(bodyBytes) == 0 {
		return nil
	}
	if err := json.Unmarshal(bodyBytes, out); err != nil {
		return fmt.Errorf("scrapfly: decode alert response: %w", err)
	}
	return nil
}
