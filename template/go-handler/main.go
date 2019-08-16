package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/machinebox/graphql"

	handler "github.com/lavrahq/cli-function-go-sdk"
	"github.com/lavrahq/cli-function-go/template/go-handler/function"
)

// GetSecret retrieves a secret from OpenFaaS
func GetSecret(secretName string) (secretBytes []byte, err error) {
	// read from the openfaas secrets folder
	secretBytes, err = ioutil.ReadFile("/var/openfaas/secrets/" + secretName)
	if err != nil {
		// read from the original location for backwards compatibility with openfaas <= 0.8.2
		secretBytes, err = ioutil.ReadFile("/run/secrets/" + secretName)
	}

	return secretBytes, err
}

type header struct {
	http.Header
	rt http.RoundTripper
}

func withHeader(rt http.RoundTripper) header {
	if rt == nil {
		rt = http.DefaultTransport
	}

	return header{Header: make(http.Header), rt: rt}
}

func (h header) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.Header {
		req.Header[k] = v
	}

	return h.rt.RoundTrip(req)
}

func makeRequestHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var input []byte
		var graphqlClient *graphql.Client

		httpClient := http.DefaultClient

		accessKeySecret, _ := os.LookupEnv("HASURA_ACCESS_KEY_SECRET")
		hasuraAccessKey, _ := GetSecret(accessKeySecret)
		rt := withHeader(httpClient.Transport)
		rt.Set("X-Hasura-Access-Key", string(hasuraAccessKey))
		httpClient.Transport = rt

		if gqlHost, gqlHostExists := os.LookupEnv("GRAPHQL_HOST"); gqlHostExists {
			graphqlClient = graphql.NewClient(gqlHost, graphql.WithHTTPClient(httpClient))
		}

		if r.Body != nil {
			defer r.Body.Close()

			bodyBytes, bodyErr := ioutil.ReadAll(r.Body)

			if bodyErr != nil {
				log.Printf("Error reading body from request.")
			}

			input = bodyBytes
		}

		req := handler.Request{
			Body:        input,
			Header:      r.Header,
			Method:      r.Method,
			QueryString: r.URL.RawQuery,
		}

		context := handler.Context{
			GraphqlClient: graphqlClient,
			GetSecret:     GetSecret,
			GetEnv:        os.LookupEnv,
		}

		result, resultErr := function.Handle(req, context)

		if result.Header != nil {
			for k, v := range result.Header {
				w.Header()[k] = v
			}
		}

		if resultErr != nil {
			log.Print(resultErr)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			if result.StatusCode == 0 {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(result.StatusCode)
			}
		}

		w.Write(result.Body)
	}
}

func parseIntOrDurationValue(val string, fallback time.Duration) time.Duration {
	if len(val) > 0 {
		parsedVal, parseErr := strconv.Atoi(val)
		if parseErr == nil && parsedVal >= 0 {
			return time.Duration(parsedVal) * time.Second
		}
	}

	duration, durationErr := time.ParseDuration(val)
	if durationErr != nil {
		return fallback
	}
	return duration
}

func main() {
	readTimeout := parseIntOrDurationValue(os.Getenv("read_timeout"), 10*time.Second)
	writeTimeout := parseIntOrDurationValue(os.Getenv("write_timeout"), 10*time.Second)

	s := &http.Server{
		Addr:           fmt.Sprintf(":%d", 8082),
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: 1 << 20, // Max header of 1MB
	}

	http.HandleFunc("/", makeRequestHandler())
	log.Fatal(s.ListenAndServe())
}
