package healthchecks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://hc-ping.com/"

type Check struct {
	ID      string
	BaseURL string
}

func NewCheck(id, baseURL string) *Check {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Check{ID: id, BaseURL: baseURL}
}

func (c *Check) Start() error {
	return c.sendPing(fmt.Sprintf("%s%s/start", c.BaseURL, c.ID), "")
}

func (c *Check) Ping(code int64, message string) error {
	return c.sendPing(fmt.Sprintf("%s%s/%d", c.BaseURL, c.ID, code), message)
}

func (c *Check) sendPing(url string, message string) error {
	r, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(message))
	if err != nil {
		return err
	}

	var attempts uint8
	var response *http.Response

	for attempts < 3 {
		if attempts != 0 {
			time.Sleep(time.Duration(math.Pow(2, float64(attempts-1))) * time.Second)
		}

		response, err = http.DefaultClient.Do(r)
		if err != nil {
			attempts++

			continue
		}

		break
	}

	if response != nil {
		defer response.Body.Close()

		b, err := io.ReadAll(response.Body)
		if err != nil {
			return err
		}
		body := string(b)

		switch body {
		case "OK":
			return nil

		case "OK (not found)":
			return fmt.Errorf("the server could not find a check with ID: %q", c.ID)

		case "OK (rate limited)":
			return errors.New("the server indicates the check was pinged too frequently (5+ times in one minute)")
		}

		return fmt.Errorf("the server returned an unknown response: %v", body)
	}

	return err
}
