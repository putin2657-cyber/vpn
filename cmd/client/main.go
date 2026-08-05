package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	server = flag.String("server", "localhost:8443", "server address")
	token  = flag.String("token", "secret-token", "pre-shared token")
)

func main() {
	flag.Parse()
	conf := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.Dial("tcp", *server, conf)
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	// send token
	_, err = conn.Write([]byte(*token + "\n"))
	if err != nil {
		log.Fatalf("write token: %v", err)
	}
	resp := make([]byte, 64)
	n, _ := conn.Read(resp)
	fmt.Printf("server: %s", string(resp[:n]))

	// simple interactive: send lines to server
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := sc.Text() + "\n"
		conn.Write([]byte(line))
		// read echo
		n, err := conn.Read(resp)
		if err != nil {
			log.Printf("read: %v", err)
			break
		}
		fmt.Print(string(resp[:n]))
	}
}
