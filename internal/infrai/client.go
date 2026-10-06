package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	key     string
	http    *http.Client
	sleep   func(context.Context, time.Duration) error
}

type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope[T any] struct {
	OK       bool            `json:"ok"`
	Data     T               `json:"data"`
	Error    *Error          `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type DNSRecord struct {
	RecordType string `json:"record_type"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	TTL        int    `json:"ttl,omitempty"`
	Priority   *int   `json:"priority,omitempty"`
}

type DomainResult struct {
	ZoneID string `json:"zone_id"`
}

type Verification struct {
	Status     string      `json:"status"`
	DNSRecords []DNSRecord `json:"dns_records"`
}

type SendResult struct {
	MessageID string `json:"message_id"`
}

type SendInput struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	From    string `json:"from"`
}

func New(baseURL, key string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    &http.Client{Timeout: 15 * time.Second},
		sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

func (c *Client) AddDomain(ctx context.Context, domain, operationID string) (DomainResult, error) {
	return request[DomainResult](ctx, c, http.MethodPost, "/v1/dns/domain/add", map[string]string{"domain": domain}, operationID)
}

func (c *Client) UpsertRecord(ctx context.Context, zoneID string, record DNSRecord, operationID string) error {
	body := struct {
		ZoneID     string `json:"zone_id"`
		RecordType string `json:"record_type"`
		Name       string `json:"name"`
		Content    string `json:"content"`
		TTL        int    `json:"ttl,omitempty"`
		Priority   *int   `json:"priority,omitempty"`
	}{zoneID, record.RecordType, record.Name, record.Content, record.TTL, record.Priority}
	_, err := request[json.RawMessage](ctx, c, http.MethodPut, "/v1/dns/record/upsert", body, operationID)
	return err
}

func (c *Client) RotateDKIM(ctx context.Context, domain, operationID string) (Verification, error) {
	path := "/v1/email/domain/rotate_dkim/" + url.PathEscape(domain)
	return request[Verification](ctx, c, http.MethodPost, path, nil, operationID)
}

func (c *Client) VerifyEmailDomain(ctx context.Context, domain, operationID string) (Verification, error) {
	return request[Verification](ctx, c, http.MethodPost, "/v1/email/domain/verify", map[string]string{"domain": domain}, operationID)
}

func (c *Client) Send(ctx context.Context, input SendInput, operationID string) (SendResult, error) {
	return request[SendResult](ctx, c, http.MethodPost, "/v1/email/send", input, operationID)
}

func request[T any](ctx context.Context, c *Client, method, path string, body any, operationID string) (T, error) {
	var zero T
	payload, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return zero, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		if operationID != "" {
			req.Header.Set("Idempotency-Key", operationID)
		}

		res, err := c.http.Do(req)
		if err != nil {
			return zero, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			return zero, readErr
		}

		var env envelope[T]
		decodeErr := json.Unmarshal(raw, &env)
		if decodeErr == nil && !env.OK && env.Error != nil {
			env.Error.HTTPStatus = res.StatusCode
			if res.StatusCode != http.StatusTooManyRequests {
				return zero, env.Error
			}
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return zero, err
			}
			continue
		}
		if decodeErr != nil {
			return zero, fmt.Errorf("decode Infrai response: %w", decodeErr)
		}
		if !env.OK {
			if env.Error != nil {
				return zero, env.Error
			}
			return zero, errors.New("Infrai request was not accepted")
		}
		if res.StatusCode >= http.StatusInternalServerError {
			return zero, fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
	return zero, errors.New("Infrai retry budget exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}
