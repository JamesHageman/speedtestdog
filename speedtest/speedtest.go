package speedtest

import (
	"fmt"
	"log"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	stdn "github.com/traetox/speedtest/speedtestdotnet"
)

const (
	duration = 1
)

// Speed A bandwidth speed in bits/sec
type Speed uint64

// Client A speedtest client
type Client struct {
	server *stdn.Testserver
	err    error
}

// Result the result of running a speed test
type Result struct {
	DownloadSpeed Speed
	UploadSpeed   Speed
	Ping          time.Duration
	Err           error

	dog *statsd.Client
}

func closestAvailableServer(cfg *stdn.Config) (*stdn.Testserver, error) {
	var err error
	for _, s := range cfg.Servers[:5] {
		if _, err = s.MedianPing(1); err != nil {
			log.Println("failed to connect to %s, trying another. Error: %s", s.Host, err)
			continue
		}
		return &s, nil
	}

	return nil, fmt.Errorf("no available servers: %s", err)
}

func NewClient() (*Client, error) {
	log.Println("Fetching speedtest.net configuration...")
	cfg, err := stdn.GetConfig()
	if err != nil {
		return nil, err
	}

	log.Println("Finding the closest server...")
	server, err := closestAvailableServer(cfg)
	if err != nil {
		return nil, err
	}

	return &Client{server: server}, nil
}

func (s Speed) String() string {
	return stdn.HumanSpeed(uint64(s))
}

func (sc *Client) SpeedTest() *Result {
	sc.err = nil
	d := sc.download()
	u := sc.upload()
	p := sc.ping()

	return &Result{DownloadSpeed: d, UploadSpeed: u, Ping: p, Err: sc.err}
}

func (c *Client) Host() string {
	return c.server.Host
}

func (c *Client) Location() string {
	return c.server.Name
}

func (sc *Client) download() Speed {
	if sc.err != nil {
		return 0
	}
	s, err := sc.server.Downstream(duration)
	if err != nil {
		sc.err = fmt.Errorf("Error getting download: %s", err)
	}
	return Speed(s)
}

func (sc *Client) upload() Speed {
	if sc.err != nil {
		return 0
	}
	s, err := sc.server.Upstream(duration)
	if err != nil {
		sc.err = fmt.Errorf("Error getting upload: %s", err)
	}
	return Speed(s)
}

func (sc *Client) ping() time.Duration {
	if sc.err != nil {
		return 0
	}
	t, err := sc.server.MedianPing(3)
	if err != nil {
		sc.err = fmt.Errorf("Error getting ping: %s", err)
	}
	return t
}

func (result *Result) String() string {
	if result.Err != nil {
		return fmt.Sprintf("Failed Speedtest: %s", result.Err)
	}

	return fmt.Sprintf(
		"Download:\t%s\tUpload:\t%s\tPing:\t%s",
		result.DownloadSpeed,
		result.UploadSpeed,
		result.Ping,
	)
}

type Reporter struct {
	Client *statsd.Client

	err error
}

func (r *Reporter) Report(result *Result) error {
	r.err = nil

	r.histogram("download", float64(result.DownloadSpeed))
	r.histogram("upload", float64(result.UploadSpeed))
	r.histogram("ping", float64(result.Ping))

	return r.err
}

func (r *Reporter) histogram(name string, value float64) {
	if r.err != nil {
		return
	}

	r.err = r.Client.Histogram(name, value, nil, 1)
}
