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

func init() {
	if statsdAddress == "" {
		log.Fatal("STATSD_ADDR not given")
	}
}

func (s speed) String() string {
	return stdn.HumanSpeed(uint64(s))
}

type speedtestConfig struct {
	server *stdn.Testserver
}

func (sc *speedtestConfig) Download() (speed, error) {
	s, err := sc.server.Downstream(duration)
	return speed(s), err
}

func (sc *speedtestConfig) Upload() (speed, error) {
	s, err := sc.server.Upstream(duration)
	return speed(s), err
}

func (sc *speedtestConfig) Ping() (time.Duration, error) {
	return sc.server.MedianPing(3)
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

	downloads := make(chan speed)
	uploads := make(chan speed)
	ping := make(chan time.Duration)
	errCh := make(chan error)

	go func() {
		for {
			if duration, err := sc.Ping(); err != nil {
				errCh <- fmt.Errorf("Error getting ping: %s", err)
			} else {
				ping <- duration
			}

			if speed, err := sc.Download(); err != nil {
				errCh <- fmt.Errorf("Error getting download: %s", err)
			} else {
				downloads <- speed
			}

			if speed, err := sc.Upload(); err != nil {
				errCh <- fmt.Errorf("Error getting upload: %s", err)
			} else {
				uploads <- speed
			}

			time.Sleep(pollDelay)
		}
	}()

	log.Print("Monitoring network ", wifiName)
	log.Print("Polling server ", sc.server.Host, " in ", sc.server.Name)
	err = dog.Incr("boot", nil, 1)
	if err != nil {
		log.Fatalln(err)
	}

	for {
		var ddErr error
		select {
		case d := <-downloads:
			log.Println("Download:\t", d)
			ddErr = dog.Histogram("download", float64(d), nil, 1)
		case u := <-uploads:
			log.Println("Upload:\t", u)
			ddErr = dog.Histogram("upload", float64(u), nil, 1)
		case p := <-ping:
			log.Println("Ping:\t", p)
			ddErr = dog.Histogram("ping", float64(p), nil, 1)
		case err := <-errCh:
			log.Fatalln(err)
		}
		if ddErr != nil {
			log.Fatalln("DataDog error:", ddErr)
		}
	}
}
