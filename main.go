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

type Speed uint64
type producer func() (float64, error)

func init() {
	if statsdAddress == "" {
		log.Fatal("STATSD_ADDR not given")
	}
}

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

func (sc *speedtestConfig) Ping() (time.Duration, error) {
	return sc.server.MedianPing(10)
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

func poll(c chan<- float64, e chan<- error, f producer) {
	for {
		n, err := f()
		if err != nil {
			e <- err
		} else {
			c <- n
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

	downloads := make(chan float64)
	uploads := make(chan float64)
	ping := make(chan float64)
	errCh := make(chan error)

	go poll(downloads, errCh, func() (float64, error) {
		speed, err := sc.Download()
		return float64(speed), err
	})

	go poll(uploads, errCh, func() (float64, error) {
		speed, err := sc.Upload()
		return float64(speed), err
	})

	go poll(ping, errCh, func() (float64, error) {
		duration, err := sc.Ping()
		return float64(duration), err
	})

	log.Println("Starting...")

	for {
		var err error
		select {
		case d := <-downloads:
			log.Println("Download:\t", Speed(d))
			err = dog.Gauge("download", d, nil, 1)
		case u := <-uploads:
			log.Println("Upload:\t", Speed(u))
			err = dog.Gauge("upload", u, nil, 1)
		case p := <-ping:
			log.Println("Ping:\t", time.Duration(p))
			err = dog.Gauge("ping", p, nil, 1)
		case produceErr := <-errCh:
			log.Fatalln("Error producing metric:", produceErr)
		}
		if err != nil {
			log.Fatal("DataDog error:", err)
		}
	}
}
