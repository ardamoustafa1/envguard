// Package alerts posts operational notifications to Slack.
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// Notify sends a message to the configured Slack incoming webhook.
func Notify(ctx context.Context, text string) error {
	url := os.Getenv("SLACK_WEBHOOK_URL")
	if url == "" {
		return fmt.Errorf("SLACK_WEBHOOK_URL is not set")
	}

	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack: unexpected status %d", resp.StatusCode)
	}
	return nil
}
