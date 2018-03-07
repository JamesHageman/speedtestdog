package main

import (
	"log"
	"os"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	speedtest "github.com/JamesHageman/speedtestdog/speedtest"
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

func main() {
	sc, err := speedtest.NewClient()
	if err != nil {
		log.Fatal("Error building speedtestConfig: ", err)
	}
	dog, err := statsd.New(statsdAddress)
	if err != nil {
		log.Fatal(err)
	}

	dog.Namespace = "speedtest."
	dog.Tags = append(dog.Tags,
		"speedtest.server:"+sc.Host(),
		"speedtest.wifi_name:"+wifiName,
	)

	log.Print("Monitoring network ", wifiName)
	log.Print("Polling server ", sc.Host(), " in ", sc.Location())

	err = dog.Incr("boot", nil, 1)
	if err != nil {
		log.Fatalln(err)
	}

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
		if result.Err != nil {
			log.Fatalln(result)
		}

		log.Println(result)
		err := reporter.Report(result)
		if err != nil {
			log.Fatalln("DataDog error:", err)
		}
	}
}
