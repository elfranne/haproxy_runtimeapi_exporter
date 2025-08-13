package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	socket  = flag.String("socket", "/var/run/haproxy.sock", "Haproxy socket to send commands to, default: /var/run/haproxy.sock")
	address = flag.String("address", "127.0.0.1:10001", "Address to listen to, default: 127.0.0.1:10001")
)

func main() {
	flag.Parse()

	// ensure the socket works
	command := "help"
	_, err := runCMD(command)
	if err != nil {
		fmt.Printf("Failed to access Haproxy socket: %v\n", err)
		os.Exit(2)
	}

	http.HandleFunc("/tables", handleTables)
	if err := http.ListenAndServe(*address, nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}

func parseTables(data []byte) []string {
	re := regexp.MustCompile(`# table: (.*?),`)
	matching := re.FindAllStringSubmatch(string(data[:]), -1)
	tables := make([]string, len(matching))
	for i, match := range matching {
		tables[i] = match[1]
	}
	return tables
}

func parseTable(data []byte, table string) ([]string, error) {
	var output []string
	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		re := regexp.MustCompile(`key=([A-Za-z0-9_.]+).*http_req_rate\(\d+\)=(\d+)`)
		matching := re.FindStringSubmatch(line)
		if matching == nil {
			return nil, fmt.Errorf("failed to parse")
		}
		if len(matching) != 3 {
			return nil, fmt.Errorf("failed to match")
		}
		rate, _ := strconv.ParseInt(matching[2], 10, 64)
		output = append(output, fmt.Sprintf("http_req_rate{table=\"%s\",key=\"%s\"} %d", table, matching[1], rate))
	}
	return output, nil
}

func runCMD(command string) ([]byte, error) {
	conn, err := net.Dial("unix", *socket)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte(command + "\n")); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(conn)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = conn.Close()
	}()

	return data, nil
}

func handleTables(w http.ResponseWriter, r *http.Request) {
	command := "show table"
	data, err := runCMD(command)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	tables := parseTables(data)
	for _, table := range tables {
		data, err := runCMD(command + " " + table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
		parsed, err := parseTable(data, table)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		}
		for _, result := range parsed {
			if _, err := fmt.Fprintf(w, "%s\n", result); err != nil {
				http.Error(w, "Unable to write response", http.StatusInternalServerError)
			}
		}
	}
}
