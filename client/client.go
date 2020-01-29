package main

import (
	"bytes"
	"encoding/json"
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

func makeQuery(host string, queryString string, channel chan string) {
	// TCP connection
	port := "5002"
	hostPort := string(host + ":" + port)
	log.Println("Establishing TCP connection with " + hostPort)
	conn, err := net.Dial("tcp", hostPort)
	if err != nil {
		log.Println("Failed to connect to " + hostPort)
		log.Println(err.Error())
		channel <- "fail"
		return
	}
	defer conn.Close()

	start := time.Now()

	// Send request
	if _, err = conn.Write([]byte(queryString + "\n")); err != nil {
		log.Println("Failed to send request to " + hostPort)
		log.Println(err.Error())
		channel <- "fail"
		return
	}

	// Receive response
	var resp GrepResponse
	decoder := json.NewDecoder(conn)
	err = decoder.Decode(&resp)

	end := time.Now()

	// Find domain name
	ip := string(bytes.Trim([]byte(resp.Ip), "\n"))
	command := exec.Command("/usr/bin/dig", "+short", "-x", ip)
	dn, err := command.Output()
	if err != nil {
		log.Println("DNS failed for server " + ip)
		log.Println(err.Error())
		channel <- "fail"
		return
	}

	// Print and save results to file
	name := string(bytes.Trim(dn, ".cs.illinois.edu\n"))
	total := string(bytes.Trim([]byte(resp.Total), "\n"))
	log.Println("Log of " + name + ": " + total + " line(s) found\n" + "query time", end.Sub(start))

	f, err := os.OpenFile("grep.out", os.O_APPEND|os.O_WRONLY, 0600)
	defer f.Close()
	if err != nil {
		log.Println("Failed to save grep results")
		log.Println(err.Error())
		channel <- "fail"
		return
	}
	if _, err = f.WriteString(name + "\n" + resp.Content + "\n" + total + " line(s) found\n\n"); err != nil {
		log.Println("Failed to save grep results")
		log.Println(err.Error())
		channel <- "fail"
	}

	// Send message
	channel <- "ok"
}

var servers []string = []string{
	"fa19-cs425-g32-01.cs.illinois.edu",
	"fa19-cs425-g32-02.cs.illinois.edu",
	"fa19-cs425-g32-03.cs.illinois.edu",
	"fa19-cs425-g32-04.cs.illinois.edu",
	"fa19-cs425-g32-05.cs.illinois.edu",
	"fa19-cs425-g32-06.cs.illinois.edu",
	"fa19-cs425-g32-07.cs.illinois.edu",
	"fa19-cs425-g32-08.cs.illinois.edu",
	"fa19-cs425-g32-09.cs.illinois.edu",
	"fa19-cs425-g32-10.cs.illinois.edu",
}

func main() {
	// check arguments
	if len(os.Args) != 2 {
		log.Fatal("Invalid arguments")
	}

	// Construct query
	pattern := os.Args[1]
	queryStruct := GrepRequest{Query: pattern}
	var queryString []byte
	queryString, err := json.Marshal(queryStruct)
	if err != nil {
		log.Fatal("Query construction failed\n" + err.Error())
	}
	log.Println(string(queryString))

	// Create or clear output file
	if _, err := os.Stat("grep.out"); os.IsNotExist(err) {
		if _, err = os.Create("grep.out"); err != nil {
			log.Fatal("Failed to create output file")
		}
	} else if err = os.Truncate("grep.out", 0); err != nil {
		log.Fatal("Failed to clear output file")
	}

	// Find localhost IP
	addrs, _ := net.InterfaceAddrs()
	var curIP string
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			curIP = ipnet.IP.String()
			log.Println("Localhost IP: " + curIP)
		}
	}

	// Find remote host IPs
	var localname string
	var hosts []string
	for _, server := range servers {
		command := exec.Command("/usr/bin/dig", "+short", server)
		host, err := command.Output()
		if err != nil {
			log.Println("DNS failed for server " + server)
			log.Println(err.Error())
		} else if ip := string(bytes.Trim(host, "\n")); ip != curIP {
			hosts = append(hosts, ip)
			log.Println("Adding " + ip + " to host list")
		} else {
			localname = server
		}
	}

	// Query all remote hosts
	channel := make(chan string)
	for _, host := range hosts {
		go makeQuery(host, string(queryString), channel)
	}

	start := time.Now()

	// Local query
	fname := "../logfile.log"
	cmd_content := exec.Command("/usr/bin/grep", "-n", "-E", pattern, fname)
	cmd_count := exec.Command("/usr/bin/grep", "-c", "-E", pattern, fname)
	res_content, _ := cmd_content.CombinedOutput()
	res_count, _ := cmd_count.CombinedOutput()

	end := time.Now()

	// Print and save results of local query
	localname = string(bytes.Trim([]byte(localname), ".cs.illinois.edu"))
	total := string(bytes.Trim(res_count, "\n"))
	log.Println("Log of " + localname + ": " + total + " line(s) found\n" + "query time", end.Sub(start))

	if f, err := os.OpenFile("grep.out", os.O_APPEND|os.O_WRONLY, 0600); err != nil {
		log.Println("Failed to save grep results")
		log.Println(err.Error())
	} else if _, err = f.WriteString(curIP + "\n" + string(res_content) + "\n" + total + " line(s) found\n\n"); err != nil {
		log.Println("Failed to save grep results")
		log.Println(err.Error())
	}

	// Sync go routines
	for i := 0; i < len(hosts); i++ {
		msg := <-channel
		if msg != "ok" && msg != "fail" {
			log.Fatal("Synchronization failed")
		}
	}
}
