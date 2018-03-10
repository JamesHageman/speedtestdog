package speedtest

import (
	"fmt"
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
	var d Speed
	var u Speed
	var p time.Duration
	var err error

	err = Try(
		WrapError(c.download(&d)),
		WrapError(c.upload(&u)),
		WrapError(c.ping(&p)),
	)

	return &Result{DownloadSpeed: d, UploadSpeed: u, Ping: p, Err: err}
}

// Host returns the address of the speedtest server.
func (c *Client) Host() string {
	return c.server.Host
}

// Location returns the location of the speedtest server.
func (c *Client) Location() string {
	return c.server.Name
}

func (c *Client) download(result *Speed) error {
	s, err := c.server.Downstream(duration)
	*result = Speed(s)
	return errors.Wrap(err, "Error getting download speed")
}

func (c *Client) upload(result *Speed) error {
	s, err := c.server.Upstream(duration)
	*result = Speed(s)
	return errors.Wrap(err, "Error getting upload speed")
}

func (c *Client) ping(result *time.Duration) error {
	t, err := c.server.MedianPing(3)
	*result = t
	return errors.Wrap(err, "Error getting ping")
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
	return Try(
		r.histogram("download", float64(result.DownloadSpeed)),
		r.histogram("upload", float64(result.UploadSpeed)),
		r.histogram("ping", float64(result.Ping)),
	)
}

func (r *Reporter) histogram(name string, value float64) func() error {
	return func() error {
		return r.Client.Histogram(name, value, nil, 1)
	}
}

type ErrFunc func() error

func Try(funcs ...ErrFunc) error {
	var err error

	for _, f := range funcs {
		err = f()
		if err != nil {
			return err
		}
	}

	return err
}

func WrapError(err error) ErrFunc {
	return func() error {
		return err
	}
}
