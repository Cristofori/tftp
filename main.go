package main

import (
	"fmt"
	"tftp/server"
)

func main() {
	fmt.Println("Starting tftp server")
	server.Run()
}
