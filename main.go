package main

import (
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	stdn "github.com/traetox/speedtest/speedtestdotnet"
)

const (
	duration  = 1
	pollDelay = 10 * time.Second
)

var (
	statsdAddress = os.Getenv("STATSD_ADDR")
)

func init() {
	if statsdAddress == "" {
		log.Fatal("STATSD_ADDR not given")
	}
}

type Speed uint64

func (s Speed) String() string {
	return stdn.HumanSpeed(uint64(s))
}

type speedtestConfig struct {
	server *stdn.Testserver
}

func (sc *speedtestConfig) Download() (Speed, error) {
	s, err := sc.server.Downstream(duration)
	return Speed(s), err
}

func (sc *speedtestConfig) Upload() (Speed, error) {
	s, err := sc.server.Upstream(duration)
	return Speed(s), err
}

func makeSpeedTestConfig() (*speedtestConfig, error) {
	cfg, err := stdn.GetConfig()
	if err != nil {
		return nil, err
	}
	server := cfg.Servers[0]

	sc := new(speedtestConfig)
	sc.server = &server
	return sc, nil
}

func (sc *speedtestConfig) pollDownloads(c chan<- Speed) {
	for {
		s, err := sc.Download()
		if err != nil {
			log.Println("Error fetching download speed:", err)
		} else {
			c <- s
		}
		time.Sleep(pollDelay)
	}
}

func (sc *speedtestConfig) pollUploads(c chan<- Speed) {
	for {
		s, err := sc.Upload()
		if err != nil {
			log.Println("Error fetching upload speed:", err)
		} else {
			c <- s
		}
		time.Sleep(pollDelay)
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

	downloads := make(chan Speed)
	uploads := make(chan Speed)

	go sc.pollDownloads(downloads)
	go sc.pollUploads(uploads)

	for {
		var err error
		select {
		case d := <-downloads:
			log.Println("Download:\t", d)
			err = dog.Gauge("download", float64(d), nil, 1)
		case u := <-uploads:
			log.Println("Upload:\t", u)
			err = dog.Gauge("upload", float64(u), nil, 1)
		}
		if err != nil {
			log.Println("DataDog error:", err)
		}
	}
}
