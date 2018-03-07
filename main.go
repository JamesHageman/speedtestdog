package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	stdn "github.com/traetox/speedtest/speedtestdotnet"
	wifiname "github.com/yelinaung/wifi-name"
)

const (
	duration  = 1
	pollDelay = 30 * time.Second
)

var (
	statsdAddress = os.Getenv("STATSD_ADDR")
	wifiName      = wifiname.WifiName()
)

type speed uint64
type producer func() (float64, error)

type speedTestResult struct {
	DownloadSpeed speed
	UploadSpeed   speed
	Ping          time.Duration
	Err           error

	dog *statsd.Client
}

type speedtestConfig struct {
	server *stdn.Testserver
	err    error
}

func init() {
	if statsdAddress == "" {
		log.Fatal("STATSD_ADDR not given")
	}
}

func (s speed) String() string {
	return stdn.HumanSpeed(uint64(s))
}

func (sc *speedtestConfig) SpeedTest() *speedTestResult {
	sc.err = nil
	d := sc.download()
	u := sc.upload()
	p := sc.ping()

	return &speedTestResult{DownloadSpeed: d, UploadSpeed: u, Ping: p, Err: sc.err}
}

func (sc *speedtestConfig) download() speed {
	if sc.err != nil {
		return 0
	}
	s, err := sc.server.Downstream(duration)
	if err != nil {
		sc.err = fmt.Errorf("Error getting download: %s", err)
	}
	return speed(s)
}

func (sc *speedtestConfig) upload() speed {
	if sc.err != nil {
		return 0
	}
	s, err := sc.server.Upstream(duration)
	if err != nil {
		sc.err = fmt.Errorf("Error getting upload: %s", err)
	}
	return speed(s)
}

func (sc *speedtestConfig) ping() time.Duration {
	if sc.err != nil {
		return 0
	}
	t, err := sc.server.MedianPing(3)
	if err != nil {
		sc.err = fmt.Errorf("Error getting ping: %s", err)
	}
	return t
}

func (result *speedTestResult) histogram(name string, value float64) {
	if result.Err != nil {
		return
	}

	result.Err = result.dog.Histogram(name, value, nil, 1)
}

func (result *speedTestResult) Report(dog *statsd.Client) error {
	result.dog = dog
	result.Err = nil

	result.histogram("download", float64(result.DownloadSpeed))
	result.histogram("upload", float64(result.UploadSpeed))
	result.histogram("ping", float64(result.Ping))

	return result.Err
}

func (result *speedTestResult) String() string {
	if result.Err != nil {
		return fmt.Sprintf("Failed speedtest: %s", result.Err)
	}

	return fmt.Sprintf(
		"Download:\t%s\tUpload:\t%s\tPing:\t%s",
		result.DownloadSpeed,
		result.UploadSpeed,
		result.Ping,
	)
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

func makeSpeedTestConfig() (*speedtestConfig, error) {
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

	return &speedtestConfig{server: server}, nil
}

func main() {
	sc, err := makeSpeedTestConfig()
	if err != nil {
		log.Fatal("error building speedtestConfig: ", err)
	}
	dog, err := statsd.New(statsdAddress)
	if err != nil {
		log.Fatal(err)
	}

	dog.Namespace = "speedtest."
	dog.Tags = append(dog.Tags,
		"speedtest.server:"+sc.server.Host,
		"speedtest.wifi_name:"+wifiName,
	)

	log.Print("Monitoring network ", wifiName)
	log.Print("Polling server ", sc.server.Host, " in ", sc.server.Name)

	err = dog.Incr("boot", nil, 1)
	if err != nil {
		log.Fatalln(err)
	}

	results := make(chan *speedTestResult)

	go func() {
		results <- sc.SpeedTest()
		ticks := time.NewTicker(pollDelay).C
		for range ticks {
			results <- sc.SpeedTest()
		}
	}()

	for result := range results {
		if result.Err != nil {
			log.Fatalln(result)
		}

		log.Println(result)
		err := result.Report(dog)
		if err != nil {
			log.Fatalln("DataDog error:", err)
		}
	}
}
