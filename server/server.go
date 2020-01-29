package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"time"
)

type GrepRequest struct {
	Query string `json:"query"`
}

type GrepResponse struct {
	Total   string `json:"total"`
	Content string `json:"content"`
	Ip      string `json:"ip"`
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}
func handleClient(conn net.Conn) {
	// match and get result
	fname := "logfile.log"
	//input := bufio.NewReader(conn)

	// read query patter from input connection
	var in GrepRequest
	d := json.NewDecoder(conn)
	d.Decode(&in)
	q := in.Query
	log.Println("grep -n " + q)
	start := time.Now()
	//use regex to match pattern
	cmd_content := exec.Command("/usr/bin/grep", "-n","-E", q, fname)
	cmd_count := exec.Command("/usr/bin/grep", "-c","-E", q, fname)
	res_content, _ := cmd_content.CombinedOutput()
	//log.Println("result of query is" + string(res_content))
	res_count, _ := cmd_count.CombinedOutput()
	addrs, _ := net.InterfaceAddrs()
	elapsed1 := time.Since(start)
	log.Printf("REGEX MATCHING TOOK %s", elapsed1)
	var curIP string
	for _, address := range addrs {

		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				curIP = ipnet.IP.String()
			}
		}
	}

	m_content := GrepResponse{string(res_count), string(res_content), curIP}
	b, _ := json.Marshal(m_content)
	conn.Write(b)
	elapsed2 := time.Since(start)
	log.Printf("SEND BACK TOOK %s", elapsed2)
}

func main() {
	port := "5002"
	listener, err := net.Listen("tcp", ":"+port)
	checkError(err)
	log.Println("Waiting for connection")
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		log.Println("Handling request from client")
		go handleClient(conn)
	}
}
