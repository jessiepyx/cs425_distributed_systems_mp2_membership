package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var introducer = "fa19-cs425-g32-01.cs.illinois.edu"
var introducerIP string
var localIP string
var localID uint32

type Member struct {
	Id uint32
	Ip string
}

var memlist []Member
var neighbor []int
var neighborHB map[uint32]bool
var udpAddrs []net.UDPAddr
var udpHBAddrs []net.UDPAddr

var mu sync.Mutex

func getNeighbor() {
	//fmt.Println("get neighbor")
	var current int
	for idx, monitor := range memlist {
		if monitor.Ip == localIP {
			current = idx
			localID = monitor.Id
			break
		}
	}
	var prev1, prev2, next1, next2 int
	neighborHB = make(map[uint32]bool, 0)
	n := len(memlist)
	//fmt.Println(n, "members")
	neighbor = make([]int, 0)
	if n > 1 {
		prev1 = (current + n - 1) % n
		neighbor = append(neighbor, prev1)
		neighborHB[memlist[prev1].Id] = true
	}
	if n > 2 {
		prev2 = (prev1 + n - 1) % n
		neighbor = append(neighbor, prev2)
		neighborHB[memlist[prev2].Id] = true
	}
	if n > 3 {
		next1 = (current + 1) % n
		neighbor = append(neighbor, next1)
		neighborHB[memlist[next1].Id] = true
	}
	if n > 4 {
		next2 = (next1 + 1) % n
		neighbor = append(neighbor, next2)
		neighborHB[memlist[next2].Id] = true
	}

	// get udp addresses
	udpAddrs = make([]net.UDPAddr, 0)
	udpHBAddrs = make([]net.UDPAddr, 0)
	for _, j := range neighbor {
		port := 5001
		portHB := 5003
		udpAddr := net.UDPAddr{IP: net.ParseIP(memlist[j].Ip), Port: port}
		udpHBAddr := net.UDPAddr{IP: net.ParseIP(memlist[j].Ip), Port: portHB}
		udpAddrs = append(udpAddrs, udpAddr)
		udpHBAddrs = append(udpHBAddrs, udpHBAddr)
	}
}

var channel = make(chan string)

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32() % 1000000007
}

func updateList(id uint32, ip string) {
	//fmt.Println("update list")
	memlist = append(memlist, Member{Id: id, Ip: ip})
	log.Println(ip, "joined the group")
}

func launchFailureDetector() {
	t := time.NewTimer(time.Second * 2)
	defer t.Stop()
	for {
		<-t.C
		mu.Lock()
		for _, j := range neighbor {

			if j < len(memlist) {
				if neighborHB[memlist[j].Id] == false {
					fmt.Println(memlist[j].Ip, "failed")
					log.Println(memlist[j].Ip, "failed")

					failhost := memlist[j].Ip
					// delete from memlist
					memlist = append(memlist[:j], memlist[j+1:]...)
					// update neighbors
					getNeighbor()
					//send message to other members
					for _, memb := range memlist {
						if memb.Ip != localIP {
							launchSender(memb.Ip, "1"+failhost)
						}
					}
				} else {
					neighborHB[memlist[j].Id] = false
				}
			}
		}
		mu.Unlock()
		t.Reset(time.Second * 2)
	}
}

func launchHeartbeater() {
	t := time.NewTimer(time.Millisecond * 1000)
	defer t.Stop()

	for {
		<-t.C
		for _, addr := range udpHBAddrs {
			var seed = time.Now().UnixNano()
			src := rand.NewSource(seed)
			rnd := rand.New(src)
			a := rnd.Float64()
			if a > 0 {
				conn, err := net.DialUDP("udp4", nil, &addr)
				defer conn.Close()
				if err != nil {
					fmt.Println("[heartbeat]", err, addr)
					return
				}
				buffer := []byte("0")

				_, err = conn.Write(buffer)

				if err != nil {
					fmt.Println(err)
					return
				}
				err = conn.Close()
				if err != nil {
					fmt.Println(err)
					return
				}
			}
		}
		//fmt.Println("Heartbeat...")
		t.Reset(time.Millisecond * 1000)
	}
}

func printList() {
	fmt.Println(memlist)
}

func handleFailure(failhost string) {
	mu.Lock()
	for i, memb := range memlist {
		if memb.Ip == failhost {
			// delete from memlist
			memlist = append(memlist[:i], memlist[i+1:]...)
			getNeighbor()
			log.Println(failhost, "failed")
			break
		}
	}
	mu.Unlock()
}

func launchListener() {
	hostName := "0.0.0.0"
	PORT := "5001"
	service := hostName + ":" + PORT
	s, err := net.ResolveUDPAddr("udp4", service)
	connection, err := net.ListenUDP("udp4", s)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer connection.Close()
	for {
		buffer := make([]byte, 1024)
		n, addr, err := connection.ReadFromUDP(buffer)
		fmt.Println("-> ", string(buffer), n, addr)
		if err != nil {
			fmt.Println(err)
			return
		}
		if buffer[0] == '1' {
			// fail
			failhost := string(buffer[1:n])
			go handleFailure(failhost)
		} else if buffer[0] == '2' {
			// join
			mu.Lock()
			if localIP == introducerIP {
				//fmt.Println("I am introducer")
				// update memlist
				joinhost := string(buffer[1:n])
				timestamp := string(time.Now().Unix())
				updateList(hash(joinhost+timestamp), joinhost)

				// send memlist to new node
				tmplist := make([]string, 0)
				for _, memb := range memlist {
					membstr, err := json.Marshal(&memb)
					if err != nil {
						fmt.Println("Failed to marshal member")
					}
					tmplist = append(tmplist, string(membstr))
				}
				memliststr, err := json.Marshal(&tmplist)
				if err != nil {
					fmt.Println("Failed to marshal membership list")
				}
				launchSender(joinhost, "4"+string(memliststr))
				//fmt.Println("send:", string(memliststr))

				// send join message to all other members
				for _, memb := range memlist {
					if memb.Ip != localIP && memb.Ip != joinhost {
						launchSender(memb.Ip, string(buffer[0:n])+":"+timestamp)
					}
				}
			} else {
				s := strings.Split(string(buffer[1:n]), ":")
				joinhost := s[0]
				timestamp := s[1]
				id := hash(joinhost + timestamp)
				updateList(id, joinhost)
			}
			// update neighbors
			getNeighbor()
			mu.Unlock()
		} else if buffer[0] == '3' {
			// leave
			leavehost := string(buffer[1:n])
			mu.Lock()
			for i, memb := range memlist {
				if memb.Ip == leavehost {
					// delete from memlist
					memlist = append(memlist[:i], memlist[i+1:]...)
					getNeighbor()
					log.Println(leavehost, "left the group")
					break
				}
			}
			mu.Unlock()
		} else if buffer[0] == '4' {
			//fmt.Println("receive:", string(buffer[1:n]))
			// update memlist (new node)
			tmplist := make([]string, 0)
			err := json.Unmarshal(buffer[1:n], &tmplist)
			if err != nil {
				fmt.Println("Failed to unmarshal membership list")
			}
			mu.Lock()
			memlist = make([]Member, len(tmplist))
			for i, membstr := range tmplist {
				err := json.Unmarshal([]byte(membstr), &memlist[i])
				if err != nil {
					fmt.Println("Failed to unmarshal member")
				}
			}
			getNeighbor()
			mu.Unlock()
			log.Println("membership list initialized as", memlist)
		}
	}
}

func launchListenerHB() {
	hostName := "0.0.0.0"
	PORT := "5003"
	service := hostName + ":" + PORT
	s, err := net.ResolveUDPAddr("udp4", service)
	connection, err := net.ListenUDP("udp4", s)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer connection.Close()
	for {
		buffer := make([]byte, 1024)
		n, addr, err := connection.ReadFromUDP(buffer)
		fmt.Println("-> ", string(buffer), n, addr)
		if err != nil {
			fmt.Println(err)
			return
		}
		if buffer[0] == '0' {
			// heartbeat
			for _, j := range neighbor {
				if memlist[j].Ip == strings.Split(addr.String(), ":")[0] {
					neighborHB[memlist[j].Id] = true
				}
			}
		}
	}
}

func launchCommandProcessor() {
	fmt.Println("command processor ready")
	for {
		buf := bufio.NewReader(os.Stdin)
		sentence, err := buf.ReadBytes('\n')

		if err != nil {
			fmt.Println(err)
		} else {
			command := string(sentence)
			s := strings.Split(command, " ")
			cmd := string(bytes.Trim([]byte(s[0]), "\n"))
			fmt.Println("command: " + cmd)
			if cmd == "ls" {
				go printList()
			} else if cmd == "join" {
				fmt.Println("Joining")
				log.Println("joining")
				if localIP == introducerIP {
					// introducer join: add itself to memlist
					localID = hash(introducerIP + string(time.Now().Unix()))
					mu.Lock()
					updateList(localID, introducerIP)
					mu.Unlock()
				} else {
					// others join: send join request to introducer
					addr := localIP
					msg := "2" + addr
					launchSender(introducerIP, msg)
				}
				go printList()
			} else if cmd == "leave" {
				log.Println("leaving")
				addr := localIP
				msg := "3" + addr
				mu.Lock()
				for _, memb := range memlist {
					if memb.Ip != localIP {
						launchSender(memb.Ip, msg)
					}
				}
				// clear memlist
				memlist = make([]Member, 0)
				getNeighbor()
				mu.Unlock()
			} else if cmd == "id" {
				fmt.Println("Localhost ID:", localID)
			} else if cmd == "ip" {
				fmt.Println("Localhost IP:", localIP)
			}
		}
	}
}

func launchSender(ip string, message string) {
	//fmt.Println("launch sender " + "send->" + ip)
	port := 5001
	udpAddr := net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	//fmt.Println("udp addr: " + udpAddr.String())
	conn, err := net.DialUDP("udp4", nil, &udpAddr)
	if err != nil {
		fmt.Println("[send]", message[0], err, udpAddr)
		return
	}
	defer conn.Close()
	buffer := []byte(message)
	_, err = conn.Write(buffer)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func main() {
	// log
	f, err := os.OpenFile("logfile.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	// Find localhost IP
	addrs, _ := net.InterfaceAddrs()
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			localIP = ipnet.IP.String()
			fmt.Println("Localhost IP: " + localIP)
			log.Println("Localhost IP: " + localIP)
		}
	}

	// Find introducer IP
	command := exec.Command("/usr/bin/dig", "+short", introducer)
	host, err := command.Output()
	if err != nil {
		fmt.Println("DNS failed for introducer " + introducer)
		fmt.Println(err.Error())
	} else {
		introducerIP = string(bytes.Trim(host, "\n"))
	}

	go launchCommandProcessor()
	go launchListener()
	go launchListenerHB()
	go launchHeartbeater()
	go launchFailureDetector()

	// Wait for go routines
	<-channel
}
