/*
	A Simple Memcached Clone written in Go

	Author: Arun Babu (143050032)
*/

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// Port in which the server should listen to
const PORT = "9000"

//Should the server print debug messages while handling client
const DEBUG = false

//Value of the key-value pair to be stored in datastore
type value struct {
	val                        []byte
	numbytes, version, exptime int64
}

/* This bundle is sent into queue for write requests*/
type dataStoreWriteBundle struct {
	clientConn net.Conn
	command    []string
	data       string
	ack        chan bool // The handle client will wait on this channel for ack
}

//Write queue
var writeQueue chan dataStoreWriteBundle

//Data store as a map
var m map[string]value

func main() {

	debug("Starting server..")

	//Listen to TCP connection on specified port
	conn, err := net.Listen("tcp", ":"+PORT)
	if err != nil {
		debug("Error listening to port:" + err.Error())
		os.Exit(1)
	}

	// Close listener when application closes
	defer closeConn(conn)

	//Initialize datastore
	m = make(map[string]value)

	//Initialize write queue
	writeQueue = make(chan dataStoreWriteBundle)

	//Wake up write handler
	go dataStoreWriteHandler()

	debug("Server started..")

	for {
		//Wait for connections from clients
		client, err := conn.Accept()

		if err != nil {
			debug("Error accepting connection :" + err.Error())
			os.Exit(1)
		}

		//Handle each client in a seperate
		go handleClient(client)
	}

}

func closeConn(c net.Listener) {
	debug("Closing server..")
	c.Close()
}

func handleClient(clientConn net.Conn) {

	//Close connection when client is done
	defer clientConn.Close()

	//Input reader
	reader := bufio.NewReader(clientConn)

	// Server the client till he exits
	for {

		buf, err := reader.ReadString('\n')

		if err != nil {
			debug("Command Read Error:" + err.Error())
			WriteTCP(clientConn, "ERR_INTERNAL\r\n")
			return
		}
		// debug("Read Msg: |"+string(buf)+" |")

		command := strings.Split(strings.Trim(buf, "\n \r\000"), " ")
		switch command[0] {
		case "get":
			getValueMeta(clientConn, command, "value")
		case "getm":
			getValueMeta(clientConn, command, "meta")

		case "set", "cas":
			data, err := reader.ReadString('\n') // These commands have data line
			if err != nil {
				debug("Data Read Error:" + err.Error())
				WriteTCP(clientConn, "ERR_INTERNAL\r\n")
				return
			}

			ack := make(chan bool)
			//Send the bundle into write queue
			writeQueue <- dataStoreWriteBundle{clientConn, command, data, ack}
			<-ack // Wait for ack after operation
		case "delete":
			ack := make(chan bool)
			writeQueue <- dataStoreWriteBundle{clientConn, command, "", ack}
			<-ack // Wait for ack after operation

		default:
			WriteTCP(clientConn, "ERR_CMD_ERR\r\n")
		}

		if DEBUG { //Print map as debug
			fmt.Println(m)
		}
	}
}

func WriteTCP(clientConn net.Conn, data string) {
	//Write to TCP connection
	_, err := clientConn.Write([]byte(data))
	if err != nil {
		debug("Write Error:" + err.Error())
	}
}

//Make sure the writes are sequential
func dataStoreWriteHandler() {

	for {

		//Receive write bundle from queue and process sequentially
		writeBundle := <-writeQueue

		debug("Write received : " + writeBundle.command[0])
		switch writeBundle.command[0] {
		case "set":
			setValue(writeBundle.clientConn, writeBundle.command, writeBundle.data)
		case "cas":
			casValue(writeBundle.clientConn, writeBundle.command, writeBundle.data)
		case "delete":
			deleteValue(writeBundle.clientConn, writeBundle.command)
		case "expire":
			expiryHandler(writeBundle.command)
		}

		//Send ACK
		writeBundle.ack <- true
	}
}

func debug(msg string) {
	if DEBUG {
		fmt.Println(msg)
	}
}