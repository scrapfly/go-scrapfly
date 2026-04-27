package scrapfly

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ScheduleRecurrence struct {
	Cron     string       `json:"cron,omitempty"`
	Interval int          `json:"interval,omitempty"`
	Unit     string       `json:"unit,omitempty"`
	Days     []string     `json:"days,omitempty"`
	Ends     *ScheduleEnd `json:"ends,omitempty"`
}

type ScheduleEnd struct {
	Type  string  `json:"type"` // "date" | "count"
	Date  *string `json:"date,omitempty"`
	Count *int    `json:"count,omitempty"`
}

type CreateScheduleRequest struct {
	WebhookName      string              `json:"webhook_name"`
	Recurrence       *ScheduleRecurrence `json:"recurrence,omitempty"`
	ScheduledDate    string              `json:"scheduled_date,omitempty"`
	AllowConcurrency bool                `json:"allow_concurrency"`
	RetryOnFailure   bool                `json:"retry_on_failure"`
	MaxRetries       int                 `json:"max_retries,omitempty"`
	Notes            string              `json:"notes,omitempty"`
}

type UpdateScheduleRequest struct {
	Recurrence       *ScheduleRecurrence    `json:"recurrence,omitempty"`
	ScheduledDate    *string                `json:"scheduled_date,omitempty"`
	AllowConcurrency *bool                  `json:"allow_concurrency,omitempty"`
	RetryOnFailure   *bool                  `json:"retry_on_failure,omitempty"`
	MaxRetries       *int                   `json:"max_retries,omitempty"`
	Notes            *string                `json:"notes,omitempty"`
	ScrapeConfig     map[string]interface{} `json:"scrape_config,omitempty"`
	ScreenshotConfig map[string]interface{} `json:"screenshot_config,omitempty"`
	CrawlerConfig    map[string]interface{} `json:"crawler_config,omitempty"`
}

type Schedule struct {
	ID                  string                 `json:"id"`
	Kind                string                 `json:"kind"`
	Status              string                 `json:"status"`
	NextScheduledDate   *string                `json:"next_scheduled_date,omitempty"`
	ScheduledDate       *string                `json:"scheduled_date,omitempty"`
	Recurrence          *ScheduleRecurrence    `json:"recurrence,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	Notes               *string                `json:"notes,omitempty"`
	CreatedBy           *string                `json:"created_by,omitempty"`
	CreatedAt           string                 `json:"created_at"`
	UpdatedAt           string                 `json:"updated_at"`
	CancelledAt         *string                `json:"cancelled_at,omitempty"`
	AllowConcurrency    bool                   `json:"allow_concurrency"`
	RetryOnFailure      bool                   `json:"retry_on_failure"`
	MaxRetries          int                    `json:"max_retries"`
	WebhookUUID         *string                `json:"webhook_uuid,omitempty"`
	UserUUID            *string                `json:"user_uuid,omitempty"`
	ConsecutiveFailures int                    `json:"consecutive_failures,omitempty"`
}

type ListSchedulesOptions struct {
	Status string // "ACTIVE" | "PAUSED" | "CANCELLED"
	Kind   string // "api.scrape" | "api.screenshot" | "api.crawler" (cross-kind list only)
}

func (c *Client) CreateScrapeSchedule(scrapeConfig map[string]interface{}, req *CreateScheduleRequest) (*Schedule, error) {
	return c.createSchedule("/scrape/schedules", "scrape_config", scrapeConfig, req)
}

func (c *Client) CreateScreenshotSchedule(screenshotConfig map[string]interface{}, req *CreateScheduleRequest) (*Schedule, error) {
	return c.createSchedule("/screenshot/schedules", "screenshot_config", screenshotConfig, req)
}

func (c *Client) CreateCrawlerSchedule(crawlerConfig map[string]interface{}, req *CreateScheduleRequest) (*Schedule, error) {
	return c.createSchedule("/crawl/schedules", "crawler_config", crawlerConfig, req)
}

func (c *Client) createSchedule(path, configKey string, config map[string]interface{}, req *CreateScheduleRequest) (*Schedule, error) {
	if req == nil {
		req = &CreateScheduleRequest{}
	}
	body := map[string]interface{}{
		configKey:           config,
		"webhook_name":      req.WebhookName,
		"allow_concurrency": req.AllowConcurrency,
		"retry_on_failure":  req.RetryOnFailure,
	}
	if req.Recurrence != nil {
		body["recurrence"] = req.Recurrence
	}
	if req.ScheduledDate != "" {
		body["scheduled_date"] = req.ScheduledDate
	}
	if req.MaxRetries > 0 {
		body["max_retries"] = req.MaxRetries
	}
	if req.Notes != "" {
		body["notes"] = req.Notes
	}
	var out Schedule
	if err := c.scheduleRequest("POST", path, "", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetSchedule(id string) (*Schedule, error) {
	var out Schedule
	if err := c.scheduleRequest("GET", "/schedules/"+url.PathEscape(id), "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ListScrapeSchedules(opts *ListSchedulesOptions) ([]Schedule, error) {
	return c.listSchedules("/scrape/schedules", opts)
}

func (c *Client) ListScreenshotSchedules(opts *ListSchedulesOptions) ([]Schedule, error) {
	return c.listSchedules("/screenshot/schedules", opts)
}

func (c *Client) ListCrawlerSchedules(opts *ListSchedulesOptions) ([]Schedule, error) {
	return c.listSchedules("/crawl/schedules", opts)
}

func (c *Client) ListSchedules(opts *ListSchedulesOptions) ([]Schedule, error) {
	return c.listSchedules("/schedules", opts)
}

func (c *Client) listSchedules(path string, opts *ListSchedulesOptions) ([]Schedule, error) {
	q := url.Values{}
	if opts != nil {
		if opts.Status != "" {
			q.Set("status", opts.Status)
		}
		if opts.Kind != "" {
			q.Set("kind", opts.Kind)
		}
	}
	var out []Schedule
	if err := c.scheduleRequest("GET", path, q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) UpdateSchedule(id string, req *UpdateScheduleRequest) (*Schedule, error) {
	if req == nil {
		return nil, fmt.Errorf("update request is required")
	}
	var out Schedule
	if err := c.scheduleRequest("PATCH", "/schedules/"+url.PathEscape(id), "", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) CancelSchedule(id string) error {
	return c.scheduleRequest("DELETE", "/schedules/"+url.PathEscape(id), "", nil, nil)
}

func (c *Client) PauseSchedule(id string) (*Schedule, error) {
	var out Schedule
	if err := c.scheduleRequest("POST", "/schedules/"+url.PathEscape(id)+"/pause", "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ResumeSchedule(id string) (*Schedule, error) {
	var out Schedule
	if err := c.scheduleRequest("POST", "/schedules/"+url.PathEscape(id)+"/resume", "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ExecuteSchedule(id string) (*Schedule, error) {
	var out Schedule
	if err := c.scheduleRequest("POST", "/schedules/"+url.PathEscape(id)+"/execute", "", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) scheduleRequest(method, path, extraQuery string, body interface{}, out interface{}) error {
	endpointURL, err := url.Parse(c.host + path)
	if err != nil {
		return err
	}
	q := endpointURL.Query()
	q.Set("key", c.key)
	if extraQuery != "" {
		extra, _ := url.ParseQuery(extraQuery)
		for k, vs := range extra {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
	}
	endpointURL.RawQuery = q.Encode()

	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal schedule request: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, endpointURL.String(), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", sdkUserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read schedule response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return c.handleScheduleErrorResponse(resp.StatusCode, bodyBytes)
	}
	if resp.StatusCode == http.StatusNoContent || len(bodyBytes) == 0 || out == nil {
		return nil
	}
	if err := json.Unmarshal(bodyBytes, out); err != nil {
		return fmt.Errorf("decode schedule response: %w (body: %s)", err, truncateBody(string(bodyBytes), 200))
	}
	return nil
}

func (c *Client) handleScheduleErrorResponse(status int, body []byte) error {
	var env struct {
		Error   string      `json:"error"`
		Message string      `json:"message"`
		Reason  string      `json:"reason"`
		Details interface{} `json:"details"`
	}
	_ = json.Unmarshal(body, &env)
	code := env.Error
	if code == "" {
		code = "ERR::SCHEDULER::BACKEND_ERROR"
	}
	msg := env.Message
	if env.Reason != "" {
		msg = env.Message + " (" + env.Reason + ")"
	}
	if msg == "" {
		msg = strings.TrimSpace(string(body))
	}
	hint := ""
	if env.Details != nil {
		if buf, mErr := json.Marshal(env.Details); mErr == nil {
			hint = string(buf)
		}
	}
	return &APIError{
		Code:           code,
		Message:        msg,
		HTTPStatusCode: status,
		Hint:           hint,
	}
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
