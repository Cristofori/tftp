package filestore

import (
	"fmt"
	"testing"
)

func fakeFile(size int) []byte {
	file := make([]byte, size)

	for i := 0; i < size; i++ {
		file[i] = byte(i % 16)
	}

	return file
}

func Test_Init(t *testing.T) {
	Init()

	if files == nil {
		t.Error("Init did not initialize the files map")
	}
}

func Test_Create(t *testing.T) {
	Init()

	data := fakeFile(1024)
	success := Create("filename", data)

	if success == false {
		t.Error("Failed to create new file")
	}
}

func Test_Exists(t *testing.T) {
	Init()

	name := "some_file"

	if Exists(name) {
		t.Error(fmt.Sprintf("%s should not exist yet"))
	}

	Create(name, fakeFile(1234))

	if !Exists(name) {
		t.Error(fmt.Sprintf("%s should exist now"))
	}
}

func Test_Get(t *testing.T) {
	Init()

	name := "a_file"

	file := fakeFile(9999)

	Create(name, file)
	retrievedFile, found := Get(name)

	if found == false {
		t.Error(fmt.Sprintf("Get() failed to find file %s", name))
	}

	if len(file) != len(retrievedFile) {
		t.Error(fmt.Sprintf("Files were not the same length, %v vs %v", len(file), len(retrievedFile)))
	}

	for i, val := range file {
		if val != retrievedFile[i] {
			t.Error(fmt.Sprintf("Files were not the same. Byte %v differs, %v vs %v", i, val, retrievedFile[i]))
		}
	}
}
