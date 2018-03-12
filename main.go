package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/JamesHageman/speedtestdog/speedtest"
	"github.com/pkg/errors"
	wifiname "github.com/yelinaung/wifi-name"
)

func die(err error) {
	if err != nil {
		log.Fatalf("[ERROR] %+v", err)
	}
}

func buildConfig(configFileName string) *speedtest.Config {
	var config *speedtest.Config
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
	configFileName := flag.String("configFile", "speedtestdog.json", "the speedtest configuration json file")
	statsdAddress := flag.String("statsdAddress", "localhost:8125", "the address of the DataDog agent")
	wifiName := flag.String("wifiName", wifiname.WifiName(), "the name of your network")
	pollDelay := flag.Duration("poll", 30*time.Second, "The wait time between successive speed tests")
	flag.Parse()

	config := buildConfig(*configFileName)
	log.Printf("Config: %#v", *config)

	sc, err := speedtest.NewClient(config)
	die(err)

	dog, err := statsd.New(*statsdAddress)
	die(err)

	dog.Namespace = "speedtest."
	dog.Tags = append(dog.Tags,
		"speedtest.server:"+sc.Host(),
		"speedtest.wifi_name:"+*wifiName,
	)

	log.Print("Monitoring network ", *wifiName)
	log.Print("Polling server ", sc.Host(), " in ", sc.Location(), " every ", pollDelay, ".")

	err = dog.Incr("boot", nil, 1)
	die(err)

	results := make(chan *speedtest.Result)
	reporter := &speedtest.Reporter{Client: dog}

	go func() {
		results <- sc.SpeedTest()
		ticks := time.NewTicker(*pollDelay).C
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
