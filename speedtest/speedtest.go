package speedtest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/pkg/errors"
	stdn "github.com/traetox/speedtest/speedtestdotnet"
)

const (
	duration = 1
)

// Speed is bandwidth speed in bits/sec
type Speed uint64

type Config struct {
	ServerBlacklist []string `json:"serverBlacklist,omitempty"`
}

// Client is the object used to connect to a speedtest server and run speed tests.
type Client struct {
	server *stdn.Testserver
	config *Config
	err    error
}

// Result the result of running a speed test. It includes an Err field which will
// be non-nil if the test failed.
type Result struct {
	DownloadSpeed Speed
	UploadSpeed   Speed
	Ping          time.Duration
	Err           error
}

func ReadConfig(r io.Reader) (*Config, error) {
	var config Config
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&config); err != nil {
		return nil, errors.Wrap(err, "Failed to parse config")
	}
	if config.ServerBlacklist == nil {
		config.ServerBlacklist = []string{}
	}
	return &config, nil
}

func closestAvailableServer(cfg *stdn.Config, serverBlacklist []string) (*stdn.Testserver, error) {
	blacklist := make(map[string]struct{})

	for _, s := range serverBlacklist {
		blacklist[s] = struct{}{}
	}

	for _, s := range cfg.Servers {
		if _, ok := blacklist[s.Host]; ok {
			// server is blacklisted, skip.
			continue
		}

		if _, err := s.MedianPing(1); err != nil {
			log.Println("failed to connect to %s, trying another. Error: %s", s.Host, err)
			continue
		}
		return &s, nil
	}

	return nil, fmt.Errorf("no available servers")
}

// NewClient creates a speedtest.Client, or an error if it could not find a server.
func NewClient(config *Config) (*Client, error) {
	log.Println("Fetching speedtest.net configuration...")
	stdnConfig, err := stdn.GetConfig()
	if err != nil {
		return nil, err
	}

	log.Println("Finding the closest server...")
	server, err := closestAvailableServer(stdnConfig, config.ServerBlacklist)
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
