package main

import (
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	speedtest "github.com/JamesHageman/speedtestdog/speedtest"
	"github.com/pkg/errors"
	wifiname "github.com/yelinaung/wifi-name"
)

const (
	pollDelay = 30 * time.Second
)

var (
	statsdAddress = os.Getenv("STATSD_ADDR")
	wifiName      = wifiname.WifiName()
)

func init() {
	if statsdAddress == "" {
		log.Fatal("STATSD_ADDR not given")
	}
}

func die(err error) {
	if err != nil {
		log.Fatalf("[ERROR] %+v", err)
	}
}

func buildConfig() *speedtest.Config {
	var config *speedtest.Config
	configFileName := "speedtestdog.json"
	configFile, err := os.Open(configFileName)
	if err == nil {
		log.Println("Reading config from", configFileName)
		config, err = speedtest.ReadConfig(configFile)
		die(err)
	} else {
		log.Println("Using default configuration")
		config = &speedtest.Config{ServerBlacklist: []string{}}
	}

	return config
}

func main() {
	config := buildConfig()
	log.Printf("Config: %#v", *config)

	sc, err := speedtest.NewClient(config)
	die(err)

	dog, err := statsd.New(statsdAddress)
	die(err)

	dog.Namespace = "speedtest."
	dog.Tags = append(dog.Tags,
		"speedtest.server:"+sc.Host(),
		"speedtest.wifi_name:"+wifiName,
	)

	log.Print("Monitoring network ", wifiName)
	log.Print("Polling server ", sc.Host(), " in ", sc.Location())

	err = dog.Incr("boot", nil, 1)
	die(err)

	results := make(chan *speedtest.Result)
	reporter := &speedtest.Reporter{Client: dog}

	go func() {
		results <- sc.SpeedTest()
		ticks := time.NewTicker(pollDelay).C
		for range ticks {
			results <- sc.SpeedTest()
		}
	}()

	for result := range results {
		die(result.Err)
		log.Println(result)
		err := reporter.Report(result)
		die(errors.Wrap(err, "DataDog error"))
	}
}
