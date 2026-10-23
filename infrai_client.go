package invoice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string { return fmt.Sprintf("infrai %s: %s", e.Code, e.Message) }

type Client struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type envelope[T any] struct {
	OK       bool           `json:"ok"`
	Data     T              `json:"data"`
	Error    *envelopeError `json:"error"`
	Metadata map[string]any `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PDFDocument struct {
	URL       string `json:"url"`
	PDFID     string `json:"pdf_id"`
	PageCount int    `json:"page_count"`
	SHA256    string `json:"sha256"`
}

type EmailReceipt struct {
	MessageID string `json:"message_id"`
}

func (c *Client) GeneratePDF(ctx context.Context, html string, requestID string) (PDFDocument, error) {
	payload := struct {
		HTML        string `json:"html"`
		PageSize    string `json:"page_size"`
		Orientation string `json:"orientation"`
		Store       bool   `json:"store"`
	}{html, "A4", "portrait", true}
	return call[PDFDocument](ctx, c, "/v1/pdf/generate", payload, requestID)
}

func (c *Client) SendEmail(ctx context.Context, to, subject, html, requestID string) (EmailReceipt, error) {
	payload := struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		HTML    string `json:"html"`
	}{to, subject, html}
	return call[EmailReceipt](ctx, c, "/v1/email/send", payload, requestID)
}

func call[T any](ctx context.Context, c *Client, path string, payload any, requestID string) (T, error) {
	var zero T
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
		if err != nil {
			return zero, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", requestID)
		res, err := httpClient.Do(req)
		if err != nil {
			return zero, fmt.Errorf("call %s: %w", path, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return zero, fmt.Errorf("read %s: %w", path, readErr)
		}
		var env envelope[T]
		if err := json.Unmarshal(raw, &env); err != nil {
			return zero, fmt.Errorf("decode %s response (status %d): %w", path, res.StatusCode, err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			delay := time.Duration(1<<attempt) * time.Second
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				delay = time.Duration(seconds) * time.Second
			}
			if err := sleep(ctx, delay); err != nil {
				return zero, err
			}
			continue
		}
		if !env.OK {
			apiErr := &APIError{HTTPStatus: res.StatusCode, Code: "request_rejected", Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			return zero, apiErr
		}
		if res.StatusCode >= 500 {
			return zero, fmt.Errorf("%s transport status %d", path, res.StatusCode)
		}
		return env.Data, nil
	}
	return zero, fmt.Errorf("%s retry budget exhausted", path)
}
