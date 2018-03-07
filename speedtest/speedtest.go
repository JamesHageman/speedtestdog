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

// Speed is bandwidth speed in bits/sec
type Speed uint64

// Client is the object used to connect to a speedtest server and run speed tests.
type Client struct {
	server *stdn.Testserver
	err    error
}

// Result the result of running a speed test. It includes an Err field which will
// be non-nil if the test failed.
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

// NewClient creates a speedtest.Client, or an error if it could not find a server.
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

// SpeedTest runs a speedtest calculating download, upload and ping in sequence.
func (c *Client) SpeedTest() *Result {
	c.err = nil
	d := c.download()
	u := c.upload()
	p := c.ping()

	return &Result{DownloadSpeed: d, UploadSpeed: u, Ping: p, Err: c.err}
}

// Host returns the address of the speedtest server.
func (c *Client) Host() string {
	return c.server.Host
}

// Location returns the location of the speedtest server.
func (c *Client) Location() string {
	return c.server.Name
}

func (c *Client) download() Speed {
	if c.err != nil {
		return 0
	}
	s, err := c.server.Downstream(duration)
	if err != nil {
		c.err = fmt.Errorf("Error getting download: %s", err)
	}
	return Speed(s)
}

func (c *Client) upload() Speed {
	if c.err != nil {
		return 0
	}
	s, err := c.server.Upstream(duration)
	if err != nil {
		c.err = fmt.Errorf("Error getting upload: %s", err)
	}
	return Speed(s)
}

func (c *Client) ping() time.Duration {
	if c.err != nil {
		return 0
	}
	t, err := c.server.MedianPing(3)
	if err != nil {
		c.err = fmt.Errorf("Error getting ping: %s", err)
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

// Reporter will report your speedtest to a DataDog statsd.Client.
type Reporter struct {
	Client *statsd.Client

	err error
}

// Report sends the results from result to r.Client
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
