package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var (
	socket = flag.String("socket", "/var/run/haproxy.sock", "Haproxy socket to send commands to, default: /var/run/haproxy.sock")
	address = flag.String("address", "127.0.0.1:10001", "Address to listen to, default: 127.0.0.1:10001")
)

func main() {
	flag.Parse()
	http.HandleFunc("/tables", handleTables)

	if err := http.ListenAndServe(*address, nil); err != nil {
		panic(err)
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
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" == true {
			continue
		}
		re := regexp.MustCompile(`key=([A-Za-z0-9_.]+).*http_req_rate\(\d+\)=(\d+)`)
		matching := re.FindStringSubmatch(line)
		if matching == nil {
			return nil, fmt.Errorf("failed to parse\n")
		}
		if len(matching) != 3 {
			return nil, fmt.Errorf("failed to match\n")
		}
		rate, _ := strconv.ParseInt(matching[2], 10, 64)
		output = append(output, fmt.Sprintf("http_req_rate{table=\"%s\",key=\"%s\"} %d", table, matching[1], rate))
	}
	return output, nil
}

func runCMD(command string) (result []byte) {
	conn, err := net.Dial("unix", *socket)
	if err != nil {
		panic(err)
	}
	if _, err := conn.Write([]byte(command + "\n")); err != nil {
		panic(err)
	}
	data, err := io.ReadAll(conn)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	return data
}

func handleTables(w http.ResponseWriter, r *http.Request) {
	command := "show table"
	data := runCMD(command)
	tables := parseTables(data)
	for _, table := range tables {
		data := runCMD(command + " " + table)
		parsed, err := parseTable(data, table)
		if err != nil {
			fmt.Printf("%s", err)
		}
		for _, result := range parsed {
			fmt.Fprint(w, result+"\n")
		}
	}
}
