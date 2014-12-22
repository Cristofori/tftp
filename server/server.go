package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"tftp/filestore"
	"time"
)

const BlockSize = 512
const Timeout = time.Second * 5

type ErrorCode int

const (
	NotDefinedError        = ErrorCode(0)
	FileNotFoundError      = ErrorCode(1)
	AccessViolationError   = ErrorCode(2)
	DiskFullError          = ErrorCode(3)
	IllegalOperationError  = ErrorCode(4)
	UnknownTransferIdError = ErrorCode(5)
	FileAlreadyExistsError = ErrorCode(6)
	NoSuchUserError        = ErrorCode(7)
)

type OpCode uint16

const (
	ReadRequestOp  = OpCode(1)
	WriteRequestOp = OpCode(2)
	DataOp         = OpCode(3)
	AckOp          = OpCode(4)
	ErrorOp        = OpCode(5)
)

type requestPacket struct {
	opcode   OpCode
	filename string
	mode     string
}

func getOpCode(packet []byte) OpCode {
	return OpCode(binary.BigEndian.Uint16(packet[:2]))
}

func parseRequest(packet []byte) requestPacket {
	opcode := getOpCode(packet)
	filename := ""

	index := -1

	for i, val := range packet[2:] {
		if val == 0 {
			index = i + 2
			filename = string(packet[2:index])
			break
		}
	}

	mode := string(packet[index+1:])

	_, cleanFile := filepath.Split(filename)

	return requestPacket{
		opcode:   opcode,
		filename: cleanFile,
		mode:     mode,
	}
}

func parseAck(packet []byte, expectedBlockNumber uint16) bool {
	if len(packet) != 4 {
		return false
	}

	opcode := getOpCode(packet)
	if opcode != AckOp {
		return false
	}

	blockNumber := binary.BigEndian.Uint16(packet[2:4])

	return blockNumber == expectedBlockNumber
}

func parseData(packet []byte) ([]byte, uint16, error) {
	opcode := getOpCode(packet)

	if opcode != DataOp {
		return []byte{}, 0, errors.New(fmt.Sprintf("Incorrect opcode for data packet: %v", opcode))
	}

	blockNumber := binary.BigEndian.Uint16(packet[2:4])
	data := packet[4:]

	return data, blockNumber, nil
}

func ackPacket(blockNumber uint16) []byte {
	packet := make([]byte, 4)

	binary.BigEndian.PutUint16(packet, uint16(AckOp))
	binary.BigEndian.PutUint16(packet[2:], blockNumber)

	return packet
}

func dataPacket(data []byte, blockNumber uint16) []byte {
	packet := make([]byte, 4)

	binary.BigEndian.PutUint16(packet, uint16(DataOp))
	binary.BigEndian.PutUint16(packet[2:], blockNumber)

	packet = append(packet, data...)

	return packet
}

func errorPacket(code ErrorCode, message string) []byte {
	packet := make([]byte, 4)

	binary.BigEndian.PutUint16(packet, uint16(ErrorOp))
	binary.BigEndian.PutUint16(packet[2:], uint16(code))

	packet = append(packet, []byte(message)...)
	packet = append(packet, 0)

	return packet
}

func handleReadRequest(filename string, conn net.Conn) {
	defer conn.Close()

	file, found := filestore.Get(filename)

	if !found {
		conn.Write(errorPacket(FileNotFoundError, fmt.Sprintf("File not found: %s", filename)))
		return
	}

	blockNumber := uint16(1)
	done := false

	buffer := make([]byte, 1024)

	for {
		index := int(BlockSize * (blockNumber - 1))

		length := BlockSize

		if len(file)-index < BlockSize {
			length = len(file) - int(index)
			done = true
		}

		attempts := 1
		packetToSend := dataPacket(file[index:index+length], blockNumber)

		for {
			if attempts >= 5 {
				conn.Write(errorPacket(NotDefinedError, fmt.Sprintf("Failed to get ACK for data block %v after 5 attempts", blockNumber)))
				return
			}

			conn.Write(packetToSend)

			conn.SetReadDeadline(time.Now().Add(Timeout))
			bytesRead, err := conn.Read(buffer)

			if err == nil && parseAck(buffer[:bytesRead], blockNumber) {
				break // Success
			}

			// Timed out waiting for ACK, couldn't parse ACK packet, or wrong block number
			attempts++
		}

		blockNumber++

		if done {
			fmt.Println(fmt.Sprintf("Successfully sent file: %s", filename))
			break
		}
	}
}

func handleWriteRequest(filename string, conn net.Conn) {
	defer conn.Close()

	if filestore.Exists(filename) {
		conn.Write(errorPacket(FileAlreadyExistsError, fmt.Sprintf("File already exists: %s", filename)))
		return
	}

	file := []byte{}
	blockNumber := uint16(0)

	for {
		buffer := make([]byte, 1024)
		attempts := 1

		var packet []byte

		for {
			if attempts >= 5 {
				conn.Write(errorPacket(NotDefinedError, fmt.Sprintf("Failed to get data block #%v after 5 attempts", blockNumber)))
				return
			}

			conn.Write(ackPacket(blockNumber))
			conn.SetReadDeadline(time.Now().Add(Timeout))

			bytesRead, err := conn.Read(buffer)

			if err == nil {
				packet = buffer[:bytesRead]
				break
			}

			// Probably timed out, try again
			attempts++
		}

		blockNumber++
		data, block, err := parseData(packet)

		if err != nil {
			conn.Write(errorPacket(NotDefinedError, "Unable to parse data packet"))
			return
		}

		if block != blockNumber {
			conn.Write(errorPacket(NotDefinedError, fmt.Sprintf("Expected block #%v, but got #%v instead", blockNumber, block)))
			return
		}

		file = append(file, data...)

		if len(packet) < (4 + BlockSize) {
			success := filestore.Create(filename, file)

			if success {
				conn.Write(ackPacket(blockNumber))
				fmt.Println(fmt.Sprintf("Successfully wrote file: %s, %v bytes", filename, len(file)))
			} else {
				conn.Write(errorPacket(FileAlreadyExistsError, fmt.Sprintf("File already exists: %s", filename)))
			}

			return
		}
	}
}

func Run() {
	filestore.Init()

	laddr, _ := net.ResolveUDPAddr("udp", ":69")
	conn, err := net.ListenUDP("udp", laddr)

	if err != nil {
		panic(err)
	}

	for {
		buffer := make([]byte, 1024)
		bytesRead, addr, err := conn.ReadFrom(buffer)

		if err != nil {
			panic(err)
		}

		request := parseRequest(buffer[:bytesRead])

		raddr, _ := net.ResolveUDPAddr("udp", addr.String())
		laddr, _ := net.ResolveUDPAddr("udp", ":0")

		conn, _ := net.DialUDP("udp", laddr, raddr)

		switch request.opcode {
		case ReadRequestOp:
			go handleReadRequest(request.filename, conn)
		case WriteRequestOp:
			go handleWriteRequest(request.filename, conn)
		default:
			conn.Write(errorPacket(IllegalOperationError, ""))
		}
	}
}
