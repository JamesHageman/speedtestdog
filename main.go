package main

import (
	"log"
	"time"

	stdn "github.com/traetox/speedtest/speedtestdotnet"
)

const (
	duration = 1
)

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

func main() {
	sc, err := MakeSpeedTestConfig()
	if err != nil {
		log.Fatal("error building speedtestConfig: ", err)
	}

	downloads := make(chan Speed)
	uploads := make(chan Speed)

	go func() {
		for {
			s, err := sc.Download()
			if err != nil {
				log.Println("Error fetching download speed:", err)
			} else {
				downloads <- s
			}
			time.Sleep(time.Second * 10)
		}
	}()

	go func() {
		for {
			s, err := sc.Upload()
			if err != nil {
				log.Println("Error fetching upload speed:", err)
			} else {
				uploads <- s
			}
			time.Sleep(time.Second * 10)
		}
	}()

	for {
		select {
		case d := <-downloads:
			log.Println("Download: ", d)
		case u := <-uploads:
			log.Println("Upload: ", u)
		}
	}
}
