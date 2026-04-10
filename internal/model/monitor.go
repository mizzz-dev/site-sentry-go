package model

import "time"

type MonitorStatus string

const (
	StatusUp   MonitorStatus = "UP"
	StatusDown MonitorStatus = "DOWN"
)

type Monitor struct {
	ID                 int64          `json:"id"`
	Name               string         `json:"name"`
	URL                string         `json:"url"`
	IntervalSeconds    int            `json:"interval_seconds"`
	TimeoutSeconds     int            `json:"timeout_seconds"`
	IsEnabled          bool           `json:"is_enabled"`
	LastStatus         *MonitorStatus `json:"last_status,omitempty"`
	LastStatusCode     *int           `json:"last_status_code,omitempty"`
	LastResponseTimeMS *int64         `json:"last_response_time_ms,omitempty"`
	LastCheckedAt      *time.Time     `json:"last_checked_at,omitempty"`
	ConsecutiveFailure int            `json:"consecutive_failures"`
	LastErrorMessage   *string        `json:"last_error_message,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type CheckResult struct {
	ID             int64         `json:"id"`
	MonitorID      int64         `json:"monitor_id"`
	Status         MonitorStatus `json:"status"`
	StatusCode     *int          `json:"status_code,omitempty"`
	ResponseTimeMS int64         `json:"response_time_ms"`
	ErrorMessage   *string       `json:"error_message,omitempty"`
	CheckedAt      time.Time     `json:"checked_at"`
}

type MonitorInput struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	IsEnabled       bool   `json:"is_enabled"`
}
