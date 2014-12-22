package server

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"tftp/filestore"
	"time"
)

func fakeData(size int) []byte {
	data := make([]byte, size)

	for i := 0; i < size; i++ {
		data[i] = byte(i % 16)
	}

	return data
}

type fakeConn struct {
	packets [][]byte
	t       *testing.T
}

func (self *fakeConn) Read(b []byte) (int, error) {
	if len(self.packets) == 0 {
		self.t.Error("Got an unexpected read (no more packets)", b)
		return 0, errors.New("Failed")
	}

	packet := self.packets[0]
	copy(b, packet)

	self.packets = self.packets[1:]

	return len(packet), nil
}

func (self *fakeConn) Write(b []byte) (int, error) {
	if len(self.packets) == 0 {
		self.t.Error("Got an unexpected write (no more packets)", b)
		return 0, errors.New("Failed")
	}

	expected := self.packets[0]
	self.packets = self.packets[1:]

	if len(b) != len(expected) {
		self.t.Errorf("Received write packet with incorrect length: %v, expected %v", len(b), len(expected))
	}

	return len(b), nil
}

func (self *fakeConn) Close() error {
	return nil
}

func (self *fakeConn) LocalAddr() net.Addr {
	var addr net.Addr
	return addr
}

func (self *fakeConn) RemoteAddr() net.Addr {
	var addr net.Addr
	return addr
}

func (self *fakeConn) SetDeadline(t time.Time) error {
	return nil
}

func (self *fakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (self *fakeConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func Test_ackPacket(t *testing.T) {
	block := uint16(1)
	packet := ackPacket(block)

	opcode := binary.BigEndian.Uint16(packet[:2])
	if OpCode(opcode) != AckOp {
		t.Errorf("ACK packet created with incorrect opcode: %#v", opcode)
	}

	blockNumber := binary.BigEndian.Uint16(packet[2:])

	if blockNumber != block {
		t.Errorf("ACK packet created with incorrect block number: %#v", blockNumber)
	}
}

func Test_parseAck(t *testing.T) {
	block := uint16(2)
	packet := ackPacket(block)

	success := parseAck(packet, block)

	if !success {
		t.Error("Failed to parse ACK packet")
	}
}

func Test_dataPacket(t *testing.T) {
	fakeData := fakeData(BlockSize)
	dataPacket := dataPacket(fakeData, 1)

	if len(dataPacket) != BlockSize+4 {
		t.Errorf("Made data packet of wrong size: %v", len(dataPacket))
	}

	opcode := OpCode(binary.BigEndian.Uint16(dataPacket[:2]))

	if opcode != DataOp {
		t.Errorf("Data packet made with incorrect op code: %#v", opcode)
	}

	blockNumber := binary.BigEndian.Uint16(dataPacket[2:4])

	if blockNumber != 1 {
		t.Errorf("Data packet made with incorrect block number: %#v", blockNumber)
	}

	packetData := dataPacket[4:]

	if len(packetData) != len(fakeData) {
		t.Errorf("Data packet made with wrong data length: %v, expected %v", len(packetData), len(fakeData))
	}

	for i := 0; i < len(packetData); i++ {
		if packetData[i] != fakeData[i] {
			t.Errorf("Data in data packet differs from given data at byte %v. Got %v, expected %v", i, packetData[i], fakeData[i])
		}
	}
}

func Test_parseData(t *testing.T) {
	fakeData := fakeData(498)
	dataPacket := dataPacket(fakeData, 2)

	parsedData, blockNumber, err := parseData(dataPacket)

	if err != nil {
		t.Errorf("Errored out while parsing data packet")
	}

	if blockNumber != 2 {
		t.Errorf("Parsed incorrected block number from data packet: %#v", blockNumber)
	}

	if len(parsedData) != len(fakeData) {
		t.Errorf("Data packet parsed wrong data length: %v, expected %v", len(parsedData), len(fakeData))
	}

	for i := 0; i < len(parsedData); i++ {
		if parsedData[i] != fakeData[i] {
			t.Errorf("Data in parsed data packet differs from given data at byte %v. Got %v, expected %v", i, parsedData[i], fakeData[i])
		}
	}
}

func makeRequestPacket(op OpCode, filename, mode string) []byte {
	packet := make([]byte, 2)
	binary.BigEndian.PutUint16(packet, uint16(op))

	packet = append(packet, []byte(filename)...)
	packet = append(packet, 0)
	packet = append(packet, []byte(mode)...)

	return packet
}

func parseErrorPacket(packet []byte) (ErrorCode, string) {
	code := ErrorCode(binary.BigEndian.Uint16(packet[2:4]))
	message := packet[2 : len(packet)-1]
	return code, string(message)
}

func Test_parseRequest(t *testing.T) {
	filename := "this_is_a_filename"
	mode := "octet"

	packet := makeRequestPacket(ReadRequestOp, filename, mode)

	parsed := parseRequest(packet)

	if parsed.opcode != ReadRequestOp {
		t.Error("Failed to parse request packet op")
	}

	if parsed.filename != filename {
		t.Error("Failed to parse request packet filename")
	}

	if parsed.mode != mode {
		t.Errorf("Failed to parse mode. Expected: '%#v', got: '%#v'", mode, parsed.mode)
	}
}

func Test_handleWriteRequest(t *testing.T) {
	var conn fakeConn
	conn.t = t

	filestore.Init()

	file := fakeData(500)
	conn.packets = [][]byte{
		ackPacket(0),
		dataPacket(file, 1),
		ackPacket(1),
	}
	handleWriteRequest("file1", &conn)

	if len(conn.packets) != 0 {
		t.Errorf("Did not complete transacion, %v packets still left", len(conn.packets))
	}

	file = fakeData(512)
	conn.packets = [][]byte{
		ackPacket(0),
		dataPacket(file, 1),
		ackPacket(1),
		dataPacket([]byte{}, 2),
		ackPacket(2),
	}
	handleWriteRequest("file2", &conn)

	if len(conn.packets) != 0 {
		t.Errorf("Did not complete transacion, %v packets still left", len(conn.packets))
	}

	file = fakeData(1024)
	conn.packets = [][]byte{
		ackPacket(0),
		dataPacket(file[:512], 1),
		ackPacket(1),
		dataPacket(file[512:1024], 2),
		ackPacket(2),
		dataPacket([]byte{}, 3),
		ackPacket(3),
	}
	handleWriteRequest("file3", &conn)

	if len(conn.packets) != 0 {
		t.Errorf("Did not complete transacion, %v packets still left", len(conn.packets))
	}
}

func Test_handleReadRequest(t *testing.T) {
	var conn fakeConn
	conn.t = t

	filestore.Init()

	expectedFile := fakeData(500)
	filestore.Create("file1", expectedFile)

	conn.packets = [][]byte{
		dataPacket(expectedFile, 1),
		ackPacket(1),
	}
	handleReadRequest("file1", &conn)

	if len(conn.packets) != 0 {
		t.Errorf("Did not complete transacion, %v packets still left", len(conn.packets))
	}

	expectedFile = fakeData(512)
	filestore.Create("file2", expectedFile)

	conn.packets = [][]byte{
		dataPacket(expectedFile, 1),
		ackPacket(1),
		dataPacket([]byte{}, 2),
		ackPacket(2),
	}
	handleReadRequest("file2", &conn)

	if len(conn.packets) != 0 {
		t.Errorf("Did not complete transacion, %v packets still left", len(conn.packets))
	}

	expectedFile = fakeData(1024)
	filestore.Create("file3", expectedFile)

	conn.packets = [][]byte{
		dataPacket(expectedFile[:512], 1),
		ackPacket(1),
		dataPacket(expectedFile[512:1024], 2),
		ackPacket(2),
		dataPacket([]byte{}, 3),
		ackPacket(3),
	}
	handleReadRequest("file3", &conn)

	if len(conn.packets) != 0 {
		t.Errorf("Did not complete transacion, %v packets still left", len(conn.packets))
	}
}
