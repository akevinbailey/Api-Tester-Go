package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func printHelp() {
	fmt.Println("Usage:")
	fmt.Println("  api-tester [URL] [arguments]")
	fmt.Println("Required arguments:")
	fmt.Println("  [URL]                   - Server URL.")
	fmt.Println("Optional Arguments:")
	fmt.Println("  -totalCalls [value]     - Total number of calls across all threads. Default is 10000.")
	fmt.Println("  -numThreads [value]     - Number of threads. Default is 12.")
	fmt.Println("  -sleepTime [value]      - Sleep time in milliseconds between calls within a thread. Default is 0.")
	fmt.Println("  -requestTimeOut [value] - HTTP request timeout in milliseconds. Default is 10000.")
	fmt.Println("  -connectTimeOut [value] - HTTP request timeout in milliseconds. Default is 20000.")
	fmt.Println("  -reuseConnects          - Add the request 'Connection: keep-alive' header.")
	fmt.Println("  -keepConnectsOpen       - Force a new connection with every request (not advised).")
	fmt.Println("Help:")
	fmt.Println("  -? or --help - Display this help message.")
}

// Function to make the GET request and measure response time
func fetchData(wg *sync.WaitGroup, mu *sync.Mutex, httpClient *http.Client, responseTimes *[]float64, url string,
	sleepTime time.Duration, keepConnectsOpen bool, reuseConnects bool, threadID int, numCalls int) {
	defer wg.Done()
	status := ""

	// Create the request structure for the httpClient
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("Error:  Request creation failed for thread %2d: %v\n", threadID, err)
	}

	if reuseConnects {
		request.Header.Add("Connection", "keep-alive")
	} else {
		request.Header.Add("Connection", "close")
	}

	for i := 0; i < numCalls; i++ {
		startTime := time.Now()
		// Make the http or https call
		resp, err := httpClient.Do(request)
		endTime := time.Now()

		responseTime := endTime.Sub(startTime).Seconds() * 1000 // Use Seconds to get float value and convert to milliseconds

		if resp != nil {
			status = resp.Status
			if !keepConnectsOpen {
				// Must read the body.  Dumping it to null out.
				_, err = io.Copy(io.Discard, resp.Body)
				err = resp.Body.Close()
			}
		}

		mu.Lock()
		if err != nil {
			fmt.Printf("Thread %2d.%-6d - Request failed: %v - Response time: %.2f ms\n", threadID, i, err, responseTime)
		} else {
			fmt.Printf("Thread %2d.%-6d - Success: %s - Response time: %.2f ms\n", threadID, i, status, responseTime)
		}
		*responseTimes = append(*responseTimes, responseTime)
		mu.Unlock()

		time.Sleep(sleepTime)
	}
}

func main() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var responseTimes []float64

	// URL to call
	url := ""
	// Total number of calls to make
	totalCalls := 10000
	// Number of threads
	numThreads := 12
	// Sleep time between calls in a thead (milliseconds)
	sleepTime := 0 * time.Millisecond
	// HTTP request timeout (milliseconds)
	requestTimeOut := 10000 * time.Millisecond
	// HTTP connection timeout (milliseconds)
	connectTimeOut := requestTimeOut * 3
	// Reuse the HTTP connections
	reuseConnects := false
	// Leaves all the connection requests open
	keepConnectsOpen := false

	// Check if there are enough arguments
	if len(os.Args) < 2 {
		fmt.Println("Error: No command line argument provided.")
		printHelp()
		return
	}

	// Check for help flag
	for _, arg := range os.Args[1:] {
		if arg == "-?" || arg == "--help" {
			printHelp()
			return
		}
	}

	// Check if the URL has a valid prefix
	if strings.HasPrefix(os.Args[1], "http") {
		url = os.Args[1]
	} else {
		fmt.Printf("Error: \"%s\" is not a valid URL\n", url)
		printHelp()
		return
	}

	// Iterate through command line arguments
	var argErr error
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "-totalCalls" {
			i++
			totalCalls, argErr = strconv.Atoi(os.Args[i])
			if argErr != nil {
				fmt.Printf("Error: \"%s\" is not a valid integer.\n", os.Args[i])
				printHelp()
				return
			}
		} else if os.Args[i] == "-numThreads" {
			i++
			numThreads, argErr = strconv.Atoi(os.Args[i])
			if argErr != nil {
				fmt.Printf("Error: \"%s\" is not a valid integer.\n", os.Args[i])
				printHelp()
				return
			}
		} else if os.Args[i] == "-sleepTime" {
			i++
			sleepTime, argErr = time.ParseDuration(os.Args[i] + "ms")
			if argErr != nil {
				fmt.Printf("Error: \"%s\" is not a valid integer.\n", os.Args[i])
				printHelp()
				return
			}
		} else if os.Args[i] == "-requestTimeOut" {
			i++
			requestTimeOut, argErr = time.ParseDuration(os.Args[i] + "ms")
			if argErr != nil {
				fmt.Printf("Error: \"%s\" is not a valid integer.\n", os.Args[i])
				printHelp()
				return
			}
		} else if os.Args[i] == "-connectTimeOut" {
			i++
			connectTimeOut, argErr = time.ParseDuration(os.Args[i] + "ms")
			if argErr != nil {
				fmt.Printf("Error: \"%s\" is not a valid integer.\n", os.Args[i])
				printHelp()
				return
			}
		} else if os.Args[i] == "-reuseConnects" {
			reuseConnects = true
		} else if os.Args[i] == "-keepConnectsOpen" {
			keepConnectsOpen = true
		}
	}

	// Create an HTTP client
	tr := &http.Transport{
		MaxIdleConns:       numThreads * 10,
		IdleConnTimeout:    connectTimeOut,
		DisableCompression: true,
		DisableKeepAlives:  !reuseConnects,
	}
	if strings.HasPrefix(strings.ToLower(url), "https") {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{Transport: tr, Timeout: requestTimeOut}

	// Calculate the number of calls each goroutine should make
	callsPerGoroutine := totalCalls / numThreads
	remainderCalls := totalCalls % numThreads
	startTime := time.Now()
	// Create and start goroutines
	for i := 0; i < numThreads; i++ {
		numCalls := callsPerGoroutine
		if i < remainderCalls {
			numCalls++
		}
		wg.Add(1)
		go fetchData(&wg, &mu, client, &responseTimes, url, sleepTime, keepConnectsOpen, reuseConnects, i, numCalls)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	endTime := time.Now()

	// Calculate the total time for the test.  Use Seconds to get float value.
	totalTime := endTime.Sub(startTime).Seconds()

	// Calculate the average requests per second
	requestsPerSecond := float64(totalCalls) / totalTime

	// Calculate and print the average response time
	var totalResponseTime float64
	for _, rt := range responseTimes {
		totalResponseTime += rt
	}
	averageResponseTime := totalResponseTime / float64(len(responseTimes))

	fmt.Printf("Total test time: %.2f s\n", totalTime)
	fmt.Printf("Average response time: %.2f ms\n", averageResponseTime)
	fmt.Printf("Average requests per second: %.2f\n", requestsPerSecond)

	// Dump all the connection states
	client.CloseIdleConnections()

	fmt.Println("All threads have finished.")
}
