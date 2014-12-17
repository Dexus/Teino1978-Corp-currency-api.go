package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"runtime"
	"time"
)

// these structs reflect the eurofxref xml data structure
type envelop struct {
	Subject string `xml:"subject"`
	Sender  string `xml:"Sender>name"`
	Cubes   []cube `xml:"Cube>Cube"`
}
type cube struct {
	Date      string     `xml:"time,attr"`
	Exchanges []exchange `xml:"Cube"`
}
type exchange struct {
	Currency string  `xml:"currency,attr" json:"currency"`
	Rate     float32 `xml:"rate,attr" json:"rate"`
}

// EUR is not present because all exchange rates are a reference to the EUR
var desiredCurrencies = map[string]struct{}{
	"USD": struct{}{},
	"JPY": struct{}{},
	"BGN": struct{}{},
	"CZK": struct{}{},
	"DKK": struct{}{},
	"GBP": struct{}{},
	"HUF": struct{}{},
	"LTL": struct{}{},
	"PLN": struct{}{},
	"RON": struct{}{},
	"SEK": struct{}{},
	"CHF": struct{}{},
	"NOK": struct{}{},
	"HRK": struct{}{},
	"RUB": struct{}{},
	"TRY": struct{}{},
	"AUD": struct{}{},
	"BRL": struct{}{},
	"CAD": struct{}{},
	"CNY": struct{}{},
	"HKD": struct{}{},
	"IDR": struct{}{},
	"ILS": struct{}{},
	"INR": struct{}{},
	"KRW": struct{}{},
	"MXN": struct{}{},
	"MYR": struct{}{},
	"NZD": struct{}{},
	"PHP": struct{}{},
	"SGD": struct{}{},
	"THB": struct{}{},
	"ZAR": struct{}{},
}

// last 90 days are available at http://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml
var eurHistURL = "http://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist.xml"
var exchangeRates = map[string][]exchange{}

func downloadExchangeRates() (io.Reader, error) {
	resp, err := http.Get(eurHistURL)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request returned %v", resp.Status)
	}

	return resp.Body, nil
}

func filterExchangeRates(c cube) []exchange {
	var rates []exchange
	for _, ex := range c.Exchanges {
		if _, ok := desiredCurrencies[ex.Currency]; ok {
			rates = append(rates, ex)
		}
	}
	return rates
}

func updateExchangeRates(data io.Reader) error {
	var e envelop
	decoder := xml.NewDecoder(data)
	if err := decoder.Decode(&e); err != nil {
		return err
	}

	for _, c := range e.Cubes {
		if _, ok := exchangeRates[c.Date]; !ok {
			exchangeRates[c.Date] = filterExchangeRates(c)
		}
	}

	return nil
}

func updateExchangeRatesCache() {
	if reader, err := downloadExchangeRates(); err != nil {
		fmt.Printf("Unable to download exchange rates. Is the URL correct?")
	} else {
		if err := updateExchangeRates(reader); err != nil {
			fmt.Printf("Failed to update exchange rates: %v", err)
		}
	}
}

func exchangeRatesByCurrency(rates []exchange) map[string]float32 {
	var mappedByCurrency = make(map[string]float32)
	for _, rate := range rates {
		mappedByCurrency[rate.Currency] = rate.Rate
	}
	return mappedByCurrency
}

// accept strings like /1986-09-03 and /1986-09-03/USD
var routingRegexp = regexp.MustCompile(`/(\d{4}-\d{2}-\d{2})/?([A-Za-z]{3})?`)

func newCurrencyExchangeServer() http.Handler {
	r := http.NewServeMux()

	r.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		if !routingRegexp.MatchString(req.URL.Path) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		parts := routingRegexp.FindAllStringSubmatch(req.URL.Path, -1)[0]
		requestedDate := parts[1]
		requestedCurrency := parts[2]

		enc := json.NewEncoder(w)
		if _, ok := exchangeRates[requestedDate]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var exs = exchangeRates[requestedDate]
		if requestedCurrency == "" {
			enc.Encode(exchangeRatesByCurrency(exs))
		} else {
			for _, rate := range exs {
				if rate.Currency == parts[2] {
					enc.Encode(rate)
					return
				}
			}

			w.WriteHeader(http.StatusNotFound)
		}
	})

	return http.Handler(r)
}

func updateExchangeRatesPeriodically() {
	for {
		time.Sleep(1 * time.Hour)

		updateExchangeRatesCache()
	}
}

func init() {
	updateExchangeRatesCache()
}

func printMemoryUsage() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	fmt.Printf("total memory usage: %2.3f MB\n", float32(memStats.Alloc)/1024./1024.)
}

func main() {
	go updateExchangeRatesPeriodically()

	printMemoryUsage()
	log.Printf("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", newCurrencyExchangeServer()))
}
