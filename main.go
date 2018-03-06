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
			log.Println("failed to connect to %s, trying another: %s", s.Host, err)
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

func generate(c chan<- float64, e chan<- error, f producer) {
	n, err := f()
	if err != nil {
		e <- err
	} else {
		c <- n
	}
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

	downloads := make(chan float64)
	uploads := make(chan float64)
	ping := make(chan float64)
	errCh := make(chan error)

	go func() {
		for {
			generate(ping, errCh, func() (float64, error) {
				duration, err := sc.Ping()
				return float64(duration), err
			})

			generate(downloads, errCh, func() (float64, error) {
				speed, err := sc.Download()
				return float64(speed), err
			})

			generate(uploads, errCh, func() (float64, error) {
				speed, err := sc.Upload()
				return float64(speed), err
			})

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
		var err error
		select {
		case d := <-downloads:
			log.Println("Download:\t", speed(d))
			err = dog.Histogram("download", d, nil, 1)
		case u := <-uploads:
			log.Println("Upload:\t", speed(u))
			err = dog.Histogram("upload", u, nil, 1)
		case p := <-ping:
			log.Println("Ping:\t", time.Duration(p))
			err = dog.Histogram("ping", p, nil, 1)
		case produceErr := <-errCh:
			log.Fatalln("Error generating metric:", produceErr)
		}
		if err != nil {
			log.Fatalln("DataDog error:", err)
		}
	}
}
